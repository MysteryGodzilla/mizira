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
	Subject        string   `json:"subject"`
	Bot            bool     `json:"bot"`
	RunBy          string   `json:"runBy"`
	Pronouns       string   `json:"pronouns"`
	AvatarPronouns string   `json:"avatarPronouns"`
	Aliases        []string `json:"aliases"`
	Runs           []string `json:"runs,omitempty"` // the bots whose profiles name this one as runner
	By             string   `json:"by,omitempty"`
	Updated        int64    `json:"updated,omitempty"`
}

func (s *Server) mzProfileRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /profiles", s.profiles)
	api.HandleFunc("PUT /profiles", s.setProfile)
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
