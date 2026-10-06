// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package admin

import (
	"errors"
	"net/http"
	"slices"
)

// Self-notes: what the bot proposed about itself from folded chat, for the operator to approve
// (as worded or edited) into room memory, or deny.

type SelfNoteView struct {
	ID        int64  `json:"id"`
	Subject   string `json:"subject"` // who it's about: the bot (a self-note) or a person (a people-note)
	Text      string `json:"text"`
	Why       string `json:"why"`
	Status    string `json:"status"` // pending, approved or denied
	Created   int64  `json:"created"`
	DecidedBy string `json:"decidedBy"`
	DecidedAt int64  `json:"decidedAt"`
	MemoryID  int64  `json:"memoryId"`
}

// ErrNotPending is a self-note already decided.
var ErrNotPending = errors.New("that note isn't waiting for a decision")

func (s *Server) mzSelfNoteRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /selfnotes", s.selfNotes)
	api.HandleFunc("POST /selfnotes/{id}/approve", s.approveSelfNote)
	api.HandleFunc("POST /selfnotes/{id}/deny", s.denySelfNote)
}

func (s *Server) selfNotes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	n, status := q.Get("network"), q.Get("status")
	if !s.knownNetwork(w, n) {
		return
	}
	if !slices.Contains([]string{"", "pending", "approved", "denied"}, status) {
		fail(w, http.StatusBadRequest, "status is pending, approved or denied")
		return
	}
	list, err := s.mz.SelfNotes(n, status)
	if err != nil {
		fail(w, http.StatusInternalServerError, "memory is unavailable")
		return
	}
	respond(w, http.StatusOK, map[string]any{"notes": list})
}

func (s *Server) approveSelfNote(w http.ResponseWriter, r *http.Request) {
	n, id, ok := s.memoryID(w, r)
	if !ok {
		return
	}
	var in struct {
		Text string `json:"text"`
	}
	if !decode(w, r, &in) {
		return
	}
	by := s.who(r)
	memID, merged, err := s.mz.ApproveSelfNote(n, id, in.Text, by)
	if err != nil {
		s.selfNoteFail(w, err)
		return
	}
	s.log.Info("console_action", "action", "selfnote_approve", "id", id, "memory", memID, "edited", in.Text != "", "by", by)
	respond(w, http.StatusOK, map[string]any{"id": id, "memoryId": memID, "merged": merged})
}

func (s *Server) denySelfNote(w http.ResponseWriter, r *http.Request) {
	n, id, ok := s.memoryID(w, r)
	if !ok {
		return
	}
	by := s.who(r)
	if err := s.mz.DenySelfNote(n, id, by); err != nil {
		s.selfNoteFail(w, err)
		return
	}
	s.log.Info("console_action", "action", "selfnote_deny", "id", id, "by", by)
	respond(w, http.StatusOK, map[string]any{"id": id})
}

func (s *Server) selfNoteFail(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotPending) {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	fail(w, http.StatusBadRequest, err.Error())
}
