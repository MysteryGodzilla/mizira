// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
)

// The Conversation and People pages: the channel's conversation (reset, recap), ignores, screened
// nicks and suspicion scores. Each change is the matching ~ command's own code (see Mizira).

// ConversationView is one network's channel conversation.
type ConversationView struct {
	Network    string     `json:"network"`
	Channel    string     `json:"channel"`
	Messages   int        `json:"messages"`
	Tokens     int        `json:"tokens"`
	MaxContext int        `json:"maxContext"`
	LastUsed   int64      `json:"lastUsed"`
	Persona    string     `json:"persona"`
	PersonaBy  string     `json:"personaBy"`
	Recap      string     `json:"recap"`
	Recent     []LineView `json:"recent"`
}

// LineView is one message of history, clipped.
type LineView struct {
	Role string `json:"role"`
	Text string `json:"text"`
	At   int64  `json:"at,omitempty"` // when it entered history; 0 for lines saved before times were kept
}

// ResetView is what a reset did.
type ResetView struct {
	PersonaCleared bool   `json:"personaCleared"`
	ModelRestored  string `json:"modelRestored"`
	Cancelled      int    `json:"cancelled"`
}

// FoldView is what a fold on request did.
type FoldView struct {
	Folded     int `json:"folded"`
	Kept       int `json:"kept"`
	RecapChars int `json:"recapChars"`
}

// FoldRefused is a fold that didn't run, with the reason to show.
type FoldRefused struct{ Reason string }

func (e FoldRefused) Error() string { return e.Reason }

type IgnoreView struct {
	Network string `json:"network"`
	Nick    string `json:"nick"`
	Until   int64  `json:"until"`
	Kind    string `json:"kind"` // admin, bot or flood
	By      string `json:"by"`
	Reason  string `json:"reason"`
}

type ScreenView struct {
	In  []string `json:"in"`  // screennicks: messages gated
	Out []string `json:"out"` // filternicks: replies checked
}

type ScoreView struct {
	Network string  `json:"network"`
	Key     string  `json:"key"` // a nick, or "nick tag" for a speaker relayed by a bot
	Score   float64 `json:"score"`
}

const (
	maxNick       = 64
	maxReason     = 200
	maxIgnoreMins = 30 * 24 * 60
)

func (s *Server) mzPeopleRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /conversation", s.conversations)
	api.HandleFunc("POST /conversation/reset", s.resetConversation)
	api.HandleFunc("DELETE /recap", s.clearRecap)
	api.HandleFunc("POST /conversation/fold", s.foldConversation)
	api.HandleFunc("GET /people", s.people)
	api.HandleFunc("POST /ignores", s.addIgnore)
	api.HandleFunc("DELETE /ignores", s.removeIgnore)
	api.HandleFunc("POST /screens", s.addScreen)
	api.HandleFunc("DELETE /screens", s.removeScreen)
	api.HandleFunc("DELETE /suspicion", s.clearSuspicion)
}

// decode reads a small JSON body into v.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(v); err != nil {
		fail(w, http.StatusBadRequest, "send a JSON body")
		return false
	}
	return true
}

// knownNetwork checks a network named in a body or query.
func (s *Server) knownNetwork(w http.ResponseWriter, n string) bool {
	if !slices.Contains(s.cfg.Networks, n) {
		fail(w, http.StatusNotFound, "no such network")
		return false
	}
	return true
}

// validNick accepts what IRC could have as a nick: one word, not too long.
func validNick(w http.ResponseWriter, nick string) bool {
	if nick == "" || len(nick) > maxNick || strings.ContainsAny(nick, " \t\r\n,!@*?") {
		fail(w, http.StatusBadRequest, "give one nick")
		return false
	}
	return true
}

func (s *Server) conversations(w http.ResponseWriter, _ *http.Request) {
	respond(w, http.StatusOK, map[string]any{"conversations": s.mz.Conversations()})
}

func (s *Server) resetConversation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Network string `json:"network"`
	}
	if !decode(w, r, &in) || !s.knownNetwork(w, in.Network) {
		return
	}
	by := s.who(r)
	out, err := s.mz.ResetConversation(in.Network, by)
	if err != nil {
		fail(w, http.StatusInternalServerError, "reset failed")
		return
	}
	s.log.Info("console_action", "action", "reset", "network", in.Network, "by", by,
		"persona_cleared", out.PersonaCleared, "model_restored", out.ModelRestored, "cancelled", out.Cancelled)
	respond(w, http.StatusOK, out)
}

func (s *Server) clearRecap(w http.ResponseWriter, r *http.Request) {
	n := r.URL.Query().Get("network")
	if !s.knownNetwork(w, n) {
		return
	}
	by := s.who(r)
	cleared, err := s.mz.ClearRecap(n, by)
	switch {
	case err != nil:
		fail(w, http.StatusInternalServerError, "the recap is unavailable")
	case !cleared:
		fail(w, http.StatusConflict, "there is no recap to clear")
	default:
		s.log.Info("console_action", "action", "recap_clear", "network", n, "by", by)
		respond(w, http.StatusOK, map[string]any{"cleared": true})
	}
}

// foldConversation can take a minute or more: the summary waits for the model like any reply.
func (s *Server) foldConversation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Network string `json:"network"`
	}
	if !decode(w, r, &in) || !s.knownNetwork(w, in.Network) {
		return
	}
	by := s.who(r)
	out, err := s.mz.FoldConversation(in.Network, by)
	var refused FoldRefused
	switch {
	case errors.As(err, &refused):
		fail(w, http.StatusConflict, refused.Reason)
	case err != nil:
		fail(w, http.StatusBadGateway, "the fold failed: the recap model call didn't work (see the log)")
	default:
		s.log.Info("console_action", "action", "recap_fold", "network", in.Network, "by", by,
			"folded", out.Folded, "kept", out.Kept)
		respond(w, http.StatusOK, out)
	}
}

func (s *Server) people(w http.ResponseWriter, _ *http.Request) {
	scores, threshold := s.mz.Suspicion()
	respond(w, http.StatusOK, map[string]any{"ignores": s.mz.Ignores(), "screened": s.mz.Screened(),
		"suspicion": scores, "quarantineAt": threshold})
}

func (s *Server) addIgnore(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Network string `json:"network"`
		Nick    string `json:"nick"`
		Minutes int    `json:"minutes"`
		Reason  string `json:"reason"`
	}
	if !decode(w, r, &in) || !s.knownNetwork(w, in.Network) || !validNick(w, in.Nick) {
		return
	}
	if in.Minutes < 1 || in.Minutes > maxIgnoreMins {
		fail(w, http.StatusBadRequest, "minutes must be 1 to 43200 (30 days)")
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if len(in.Reason) > maxReason {
		fail(w, http.StatusBadRequest, "the reason is too long")
		return
	}
	by := s.who(r)
	until, err := s.mz.Ignore(in.Network, in.Nick, time.Duration(in.Minutes)*time.Minute, in.Reason, by)
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	s.log.Info("console_action", "action", "ignore", "network", in.Network, "nick", in.Nick,
		"minutes", in.Minutes, "reason", in.Reason, "by", by)
	respond(w, http.StatusOK, map[string]any{"nick": in.Nick, "until": until.Unix()})
}

func (s *Server) removeIgnore(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	n, nick := q.Get("network"), q.Get("nick")
	if !s.knownNetwork(w, n) || !validNick(w, nick) {
		return
	}
	by := s.who(r)
	if !s.mz.Unignore(n, nick, by) {
		fail(w, http.StatusNotFound, nick+" wasn't ignored")
		return
	}
	s.log.Info("console_action", "action", "unignore", "network", n, "nick", nick, "by", by)
	respond(w, http.StatusOK, map[string]any{"nick": nick})
}

func (s *Server) addScreen(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Network string `json:"network"`
		Nick    string `json:"nick"`
	}
	if !decode(w, r, &in) || !s.knownNetwork(w, in.Network) || !validNick(w, in.Nick) {
		return
	}
	by := s.who(r)
	dropped, err := s.mz.Screen(in.Network, in.Nick, by)
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	s.log.Info("console_action", "action", "screen", "nick", in.Nick, "quarantined", dropped, "by", by)
	respond(w, http.StatusOK, map[string]any{"nick": in.Nick, "dropped": dropped})
}

func (s *Server) removeScreen(w http.ResponseWriter, r *http.Request) {
	nick := r.URL.Query().Get("nick")
	if !validNick(w, nick) {
		return
	}
	by := s.who(r)
	if !s.mz.Unscreen(nick, by) {
		fail(w, http.StatusNotFound, nick+" wasn't screened")
		return
	}
	s.log.Info("console_action", "action", "unscreen", "nick", nick, "by", by)
	respond(w, http.StatusOK, map[string]any{"nick": nick})
}

func (s *Server) clearSuspicion(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	n, key := q.Get("network"), strings.TrimSpace(q.Get("key"))
	if !s.knownNetwork(w, n) {
		return
	}
	if key == "" || len(key) > 2*maxNick {
		fail(w, http.StatusBadRequest, "give the score's key")
		return
	}
	by := s.who(r)
	if !s.mz.ClearSuspicion(n, key, by) {
		fail(w, http.StatusNotFound, "no score for "+key)
		return
	}
	s.log.Info("console_action", "action", "suspicion_clear", "network", n, "key", key, "by", by)
	respond(w, http.StatusOK, map[string]any{"key": key})
}
