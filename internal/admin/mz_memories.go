// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// The Memories page: browse by subject, search, add, edit and forget, with room memory (facts about
// the channel and the bot) shown apart.

type SubjectView struct {
	Subject string `json:"subject"`
	Count   int    `json:"count"`
	Room    bool   `json:"room"` // about the channel or the bot: sent with every request
}

type MemoryView struct {
	ID      int64  `json:"id"`
	Subject string `json:"subject"`
	Fact    string `json:"fact"`
	Author  string `json:"author"`
	Created int64  `json:"created"`
	Edited  int64  `json:"edited,omitempty"` // when its wording last changed, if it has
	// SameAs is the id of another memory about the subject that says the same thing, if any.
	SameAs int64 `json:"sameAs,omitempty"`
	// Locked memories survive every forget from IRC; the console forgets one only once unlocked.
	Locked bool `json:"locked"`
}

var (
	// ErrNoSuchMemory is a memory id that isn't on the network.
	ErrNoSuchMemory = errors.New("no such memory")
	// ErrMemoryLocked is a forget of a locked memory.
	ErrMemoryLocked = errors.New("that memory is locked; unlock it first")
)

func (s *Server) mzMemoryRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /memories/subjects", s.memorySubjects)
	api.HandleFunc("GET /memories", s.memories)
	api.HandleFunc("POST /memories", s.addMemory)
	api.HandleFunc("PUT /memories/{id}", s.editMemory)
	api.HandleFunc("DELETE /memories/{id}", s.forgetMemory)
	api.HandleFunc("PUT /memories/{id}/lock", s.lockMemory)
	api.HandleFunc("DELETE /memories/{id}/lock", s.lockMemory)
	api.HandleFunc("POST /memories/compact/preview", s.compactPreview)
	api.HandleFunc("POST /memories/compact/apply", s.compactApply)
}

func (s *Server) memorySubjects(w http.ResponseWriter, r *http.Request) {
	n := r.URL.Query().Get("network")
	if !s.knownNetwork(w, n) {
		return
	}
	subjects, limit, err := s.mz.MemorySubjects(n)
	if err != nil {
		fail(w, http.StatusInternalServerError, "memory is unavailable")
		return
	}
	respond(w, http.StatusOK, map[string]any{"subjects": subjects, "perSubject": limit})
}

func (s *Server) memories(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	n := q.Get("network")
	if !s.knownNetwork(w, n) {
		return
	}
	list, err := s.mz.Memories(n, strings.TrimSpace(q.Get("subject")), strings.TrimSpace(q.Get("q")))
	if err != nil {
		fail(w, http.StatusInternalServerError, "memory is unavailable")
		return
	}
	respond(w, http.StatusOK, map[string]any{"memories": list})
}

func (s *Server) addMemory(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Network string `json:"network"`
		Subject string `json:"subject"`
		Fact    string `json:"fact"`
	}
	if !decode(w, r, &in) || !s.knownNetwork(w, in.Network) {
		return
	}
	by := s.who(r)
	id, merged, err := s.mz.AddMemory(in.Network, in.Subject, in.Fact, by)
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	s.log.Info("console_action", "action", "remember", "id", id, "subject", in.Subject, "merged", merged, "by", by)
	respond(w, http.StatusOK, map[string]any{"id": id, "merged": merged})
}

// memoryID reads the {id} in the path and the network in the query.
func (s *Server) memoryID(w http.ResponseWriter, r *http.Request) (string, int64, bool) {
	n := r.URL.Query().Get("network")
	if !s.knownNetwork(w, n) {
		return "", 0, false
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, http.StatusBadRequest, "memory ids are numbers")
		return "", 0, false
	}
	return n, id, true
}

func (s *Server) editMemory(w http.ResponseWriter, r *http.Request) {
	n, id, ok := s.memoryID(w, r)
	if !ok {
		return
	}
	var in struct {
		Fact string `json:"fact"`
	}
	if !decode(w, r, &in) {
		return
	}
	by := s.who(r)
	if err := s.mz.EditMemory(n, id, in.Fact, by); err != nil {
		s.memoryFail(w, err)
		return
	}
	s.log.Info("console_action", "action", "memory_edit", "id", id, "by", by)
	respond(w, http.StatusOK, map[string]any{"id": id})
}

func (s *Server) forgetMemory(w http.ResponseWriter, r *http.Request) {
	n, id, ok := s.memoryID(w, r)
	if !ok {
		return
	}
	by := s.who(r)
	if err := s.mz.ForgetMemory(n, id, by); err != nil {
		s.memoryFail(w, err)
		return
	}
	s.log.Info("console_action", "action", "forget", "id", id, "by", by)
	respond(w, http.StatusOK, map[string]any{"id": id})
}

func (s *Server) lockMemory(w http.ResponseWriter, r *http.Request) {
	n, id, ok := s.memoryID(w, r)
	if !ok {
		return
	}
	by, locked := s.who(r), r.Method == http.MethodPut
	if err := s.mz.LockMemory(n, id, locked, by); err != nil {
		s.memoryFail(w, err)
		return
	}
	action := "memory_unlock"
	if locked {
		action = "memory_lock"
	}
	s.log.Info("console_action", "action", action, "id", id, "by", by)
	respond(w, http.StatusOK, map[string]any{"id": id, "locked": locked})
}

func (s *Server) memoryFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNoSuchMemory):
		fail(w, http.StatusNotFound, err.Error())
		return
	case errors.Is(err, ErrMemoryLocked):
		fail(w, http.StatusConflict, err.Error())
		return
	}
	fail(w, http.StatusBadRequest, err.Error())
}

// ErrMemoriesChanged: the memories changed after the compaction preview; preview again.
var ErrMemoriesChanged = errors.New("the memories changed since the preview; preview again")

func (s *Server) compactPreview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Network string `json:"network"`
		Subject string `json:"subject"`
	}
	if !decode(w, r, &in) || !s.knownNetwork(w, in.Network) {
		return
	}
	if strings.TrimSpace(in.Subject) == "" {
		fail(w, http.StatusBadRequest, "give a subject")
		return
	}
	facts, basedOn, err := s.mz.CompactPreview(in.Network, in.Subject)
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	respond(w, http.StatusOK, map[string]any{"facts": facts, "basedOn": basedOn})
}

func (s *Server) compactApply(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Network string   `json:"network"`
		Subject string   `json:"subject"`
		BasedOn []int64  `json:"basedOn"`
		Facts   []string `json:"facts"`
	}
	// A whole subject's facts: allow more than the usual small body.
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		fail(w, http.StatusBadRequest, "send a JSON body")
		return
	}
	if !s.knownNetwork(w, in.Network) {
		return
	}
	if strings.TrimSpace(in.Subject) == "" || len(in.BasedOn) == 0 {
		fail(w, http.StatusBadRequest, "give the subject and the ids the preview was based on")
		return
	}
	by := s.who(r)
	stored, err := s.mz.CompactApply(in.Network, in.Subject, in.BasedOn, in.Facts, by)
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, ErrMemoriesChanged) {
			code = http.StatusConflict
		}
		fail(w, code, err.Error())
		return
	}
	s.log.Info("console_action", "action", "compact", "subject", in.Subject, "before", len(in.BasedOn), "after", stored, "by", by)
	respond(w, http.StatusOK, map[string]any{"stored": stored})
}
