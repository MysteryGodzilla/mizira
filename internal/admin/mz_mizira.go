// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"time"
)

// Mizira is what the console's own pages need from the bot, on top of upstream's services. The bot
// wires it (internal/bot) to the same code the ~ commands run, so the console and IRC can't disagree.
type Mizira interface {
	// RunState is "running", "paused" or "stopped".
	RunState() string
	// SetRunState does what ~pause, ~stop or ~resume does. changed is false when it was a no-op
	// (pausing a stopped bot, resuming a running one).
	SetRunState(state, by string) (previous string, changed bool, cancelled int)
	// Tools are the tools loaded right now.
	Tools() []string
	// Thinking reports whether the model is asked to reason before answering.
	Thinking() bool

	// The channel conversation on each network: ~reset and ~recap clear.
	Conversations() []ConversationView
	ResetConversation(network, by string) (ResetView, error)
	ClearRecap(network, by string) (bool, error)

	// ~ignore, ~screen and ~suspicion. A refusal's error is the reason, shown as is.
	Ignores() []IgnoreView
	Ignore(network, nick string, d time.Duration, reason, by string) (time.Time, error)
	Unignore(network, nick, by string) bool
	Screened() ScreenView
	Screen(network, nick, by string) (dropped int, err error)
	Unscreen(nick, by string) bool
	Suspicion() (scores []ScoreView, quarantineAt float64)
	ClearSuspicion(network, key, by string) bool

	// ~bots, ~set (credentials excluded) and the tool list.
	Bots() BotsView
	ChangeBots(add bool, kind, value, by string) (stored string, err error)
	Settings() []SettingView
	SetSetting(key, value, by string) (SettingChange, error)
	ResetSetting(key, by string) (SettingChange, error)
	ToolCatalog() []ToolView
	SwitchTool(spec string, on bool, by string) error
	ResetToolSwitches(by string) error

	// Memories. Edit and forget return ErrNoSuchMemory for an id not on the network.
	MemorySubjects(network string) (subjects []SubjectView, perSubject int, err error)
	Memories(network, subject, query string) ([]MemoryView, error)
	AddMemory(network, subject, fact, by string) (id int64, merged bool, err error)
	EditMemory(network string, id int64, fact, by string) error
	ForgetMemory(network string, id int64, by string) error
	// Compaction: a model-proposed shorter list (nothing changes), then the operator's reviewed list
	// replaces the memories the preview was based on, or ErrMemoriesChanged.
	CompactPreview(network, subject string) (facts []string, basedOn []int64, err error)
	CompactApply(network, subject string, basedOn []int64, facts []string, by string) (stored int, err error)

	// Self-notes, as ~selfnotes does. A note already decided returns ErrNotPending.
	SelfNotes(network, status string) ([]SelfNoteView, error)
	ApproveSelfNote(network string, id int64, text, by string) (memoryID int64, merged bool, err error)
	DenySelfNote(network string, id int64, by string) error

	// Safety events from the log file over the last days, newest first, at most limit.
	SafetyEvents(days, limit int) ([]SafetyEventView, error)
}

// WithMizira adds the console's own pages. Without it the server is upstream's page as it was.
func (s *Server) WithMizira(m Mizira) *Server {
	s.mz = m
	return s
}

// mzRoutes registers the console's own endpoints.
func (s *Server) mzRoutes(api *http.ServeMux) {
	if s.mz == nil {
		return
	}
	api.HandleFunc("GET /features", s.features)
	api.HandleFunc("GET /mizira/state", s.mizState)
	api.HandleFunc("PUT /mizira/state", s.setMizState)
	s.mzPeopleRoutes(api)
	s.mzSettingsRoutes(api)
	s.mzMemoryRoutes(api)
	s.mzSelfNoteRoutes(api)
	s.mzSafetyRoutes(api)
}

type feature struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Active bool   `json:"active"`
	// How to turn it on, shown on the hidden card when "show inactive" is on.
	How string `json:"how"`
}

// features says which optional parts of the bot are on, so the page can hide the cards for the rest.
func (s *Server) features(w http.ResponseWriter, _ *http.Request) {
	tools := s.mz.Tools()
	has := func(name string) bool { return slices.Contains(tools, name) }
	respond(w, http.StatusOK, map[string]any{"features": []feature{
		{ID: "work", Label: "Background work", Active: has("task__start"),
			How: "add the task__ tools to tools in config.yml"},
		{ID: "reminders", Label: "Reminders", Active: has("irc__remind"),
			How: "add irc__remind to tools in config.yml"},
		{ID: "gpu", Label: "GPU queue", Active: s.cfg.ComfyURL != "", How: "set COMFYUI_URL"},
		{ID: "radio", Label: "Radio", Active: s.cfg.RadioURL != "", How: "set RADIO_API_URL"},
		{ID: "thinking", Label: "Thinking", Active: s.mz.Thinking(), How: "set thinkingeffort above off"},
	}})
}

type mizState struct {
	State string `json:"state"`
}

func (s *Server) mizState(w http.ResponseWriter, _ *http.Request) {
	respond(w, http.StatusOK, mizState{State: s.mz.RunState()})
}

// setMizState is ~pause, ~stop and ~resume from the console.
func (s *Server) setMizState(w http.ResponseWriter, r *http.Request) {
	var in mizState
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&in); err != nil ||
		!slices.Contains([]string{"running", "paused", "stopped"}, in.State) {
		fail(w, http.StatusBadRequest, `send {"state": "running"}, "paused" or "stopped"`)
		return
	}
	by := s.who(r)
	previous, changed, cancelled := s.mz.SetRunState(in.State, by)
	if !changed {
		fail(w, http.StatusConflict, "already "+previous)
		return
	}
	s.log.Info("console_action", "action", "runstate", "from", previous, "to", in.State, "by", by,
		"cancelled", cancelled)
	respond(w, http.StatusOK, map[string]any{"state": in.State, "was": previous, "cancelled": cancelled})
}
