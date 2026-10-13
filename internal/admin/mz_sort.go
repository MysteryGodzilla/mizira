// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// SortView is one memory's sorting: the model's suggestion in a preview, the operator's choice in an
// apply.
type SortView struct {
	ID      int64    `json:"id"`
	Fact    string   `json:"fact,omitempty"`
	Subject string   `json:"subject,omitempty"` // the subject it has now
	Kind    string   `json:"kind"`
	About   string   `json:"about"` // the subject it should have
	Others  []string `json:"others"`
}

// ProfileHintView is a profile change suggested by one of the subject's memories.
type ProfileHintView struct {
	Field    string `json:"field"` // pronouns, avatarPronouns, alias or runBy
	Value    string `json:"value"`
	MemoryID int64  `json:"memoryId"`
	Fact     string `json:"fact"`
}

// maxSortItems bounds one apply.
const maxSortItems = 100

func (s *Server) mzSortRoutes(api *http.ServeMux) {
	api.HandleFunc("POST /memories/sort/preview", s.sortPreview)
	api.HandleFunc("POST /memories/sort/apply", s.sortApply)
	api.HandleFunc("GET /profiles/hints", s.profileHints)
}

// sortPreview can take a minute or more: it waits for the model like a reply.
func (s *Server) sortPreview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Network string `json:"network"`
	}
	if !decode(w, r, &in) || !s.knownNetwork(w, in.Network) {
		return
	}
	list, unsorted, err := s.mz.SortPreview(in.Network)
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	respond(w, http.StatusOK, map[string]any{"suggestions": list, "unsorted": unsorted})
}

func (s *Server) sortApply(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Network string     `json:"network"`
		Items   []SortView `json:"items"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "send a JSON body")
		return
	}
	if !s.knownNetwork(w, in.Network) {
		return
	}
	if len(in.Items) == 0 || len(in.Items) > maxSortItems {
		fail(w, http.StatusBadRequest, "send 1 to 100 memories to apply")
		return
	}
	by := s.who(r)
	applied, problems, err := s.mz.SortApply(in.Network, in.Items, by)
	if err != nil {
		fail(w, http.StatusInternalServerError, "memory is unavailable")
		return
	}
	if problems == nil {
		problems = []string{}
	}
	s.log.Info("console_action", "action", "memory_sort", "network", in.Network, "applied", applied,
		"problems", len(problems), "by", by)
	respond(w, http.StatusOK, map[string]any{"applied": applied, "problems": problems})
}

func (s *Server) profileHints(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	n, subject := q.Get("network"), strings.TrimSpace(q.Get("subject"))
	if !s.knownNetwork(w, n) || !validNick(w, subject) {
		return
	}
	hints, err := s.mz.ProfileHints(n, subject)
	if err != nil {
		fail(w, http.StatusInternalServerError, "memory is unavailable")
		return
	}
	if hints == nil {
		hints = []ProfileHintView{}
	}
	respond(w, http.StatusOK, map[string]any{"hints": hints})
}
