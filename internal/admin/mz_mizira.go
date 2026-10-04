// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
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
