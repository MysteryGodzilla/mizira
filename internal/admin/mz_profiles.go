// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package admin

import (
	"net/http"
	"strings"
)

// ProfileView is a person's or bot's profile: what is set about them rather than remembered.
type ProfileView struct {
	Subject        string    `json:"subject"`
	Bot            bool      `json:"bot"`
	RunBy          string    `json:"runBy"`
	Pronouns       string    `json:"pronouns"`
	AvatarPronouns string    `json:"avatarPronouns"`
	Aliases        []string  `json:"aliases"`
	Timezone       string    `json:"timezone"`
	Runs           []string  `json:"runs,omitempty"` // the bots whose profiles name this one as runner
	Look           *LookView `json:"look,omitempty"`
	By             string    `json:"by,omitempty"`
	Updated        int64     `json:"updated,omitempty"`
}

// LookView is how someone is drawn: appearance word for word, default hair and outfit, a reference
// picture, and variants that swap hair or outfit when named.
type LookView struct {
	Appearance string `json:"appearance"`
	Hair       string `json:"hair"`
	Outfit     string `json:"outfit"`
	// DefaultName is what a request calls the default hair and outfit; blank is "casual".
	DefaultName string            `json:"default_name"`
	Reference   string            `json:"reference"`
	Variants    []LookVariantView `json:"variants"`
}

// LookVariantView is one named variant of a look.
type LookVariantView struct {
	Name   string `json:"name"`
	Hair   string `json:"hair"`
	Outfit string `json:"outfit"`
}

func (s *Server) mzProfileRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /profiles", s.profiles)
	api.HandleFunc("PUT /profiles", s.setProfile)
	api.HandleFunc("PUT /profiles/look", s.setLook)
	api.HandleFunc("DELETE /profiles", s.deleteProfile)
}

func (s *Server) profiles(w http.ResponseWriter, r *http.Request) {
	n := r.URL.Query().Get("network")
	if !s.knownNetwork(w, n) {
		return
	}
	list, err := s.mz.Profiles(n)
	if err != nil {
		fail(w, http.StatusInternalServerError, "memory is unavailable")
		return
	}
	if list == nil {
		list = []ProfileView{}
	}
	respond(w, http.StatusOK, map[string]any{"profiles": list})
}

func (s *Server) setProfile(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Network string `json:"network"`
		ProfileView
	}
	if !decode(w, r, &in) || !s.knownNetwork(w, in.Network) {
		return
	}
	by := s.who(r)
	if err := s.mz.SetProfile(in.Network, in.ProfileView, by); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.log.Info("console_action", "action", "profile_set", "network", in.Network, "subject", in.Subject,
		"bot", in.Bot, "run_by", in.RunBy, "pronouns", in.Pronouns, "avatar_pronouns", in.AvatarPronouns,
		"aliases", strings.Join(in.Aliases, ","), "by", by)
	respond(w, http.StatusOK, map[string]any{"subject": in.Subject})
}

func (s *Server) setLook(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Network string   `json:"network"`
		Subject string   `json:"subject"`
		Look    LookView `json:"look"`
	}
	if !decode(w, r, &in) || !s.knownNetwork(w, in.Network) || !validNick(w, strings.TrimSpace(in.Subject)) {
		return
	}
	by := s.who(r)
	if err := s.mz.SetLook(in.Network, strings.TrimSpace(in.Subject), in.Look, by); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.log.Info("console_action", "action", "look_set", "network", in.Network, "subject", in.Subject,
		"appearance", in.Look.Appearance, "variants", len(in.Look.Variants), "by", by)
	respond(w, http.StatusOK, map[string]any{"subject": in.Subject})
}

func (s *Server) deleteProfile(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	n, subject := q.Get("network"), strings.TrimSpace(q.Get("subject"))
	if !s.knownNetwork(w, n) || !validNick(w, subject) {
		return
	}
	by := s.who(r)
	ok, err := s.mz.DeleteProfile(n, subject, by)
	switch {
	case err != nil:
		fail(w, http.StatusInternalServerError, "memory is unavailable")
	case !ok:
		fail(w, http.StatusNotFound, "no profile for "+subject)
	default:
		s.log.Info("console_action", "action", "profile_delete", "network", n, "subject", subject, "by", by)
		respond(w, http.StatusOK, map[string]any{"subject": subject})
	}
}
