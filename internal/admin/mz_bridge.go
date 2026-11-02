// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package admin

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"B4reMetal/metald/internal/core"
)

// The status bridge: what she is doing, for a dashboard, behind its own read-only key (statustoken).
// It reports states and counts only, never chat text, nicks, memories or prompts.

type bridgeView struct {
	Version string `json:"version"`
	Started int64  `json:"started"`
	Now     int64  `json:"now"`
	// State is working, idle, paused or stopped (~stop: on IRC, answering nobody).
	State string `json:"state"`
	// IRC is false when she isn't on IRC this run (a console-only start, a tool that couldn't load).
	IRC        bool          `json:"irc"`
	Doing      []bridgeDoing `json:"doing"`
	LastActive int64         `json:"last_active"`
	Waiting    int           `json:"waiting"`
	Tasks      int           `json:"tasks"`
}

type bridgeDoing struct {
	What  string `json:"what"`
	Since int64  `json:"since"`
}

// toolLabels names a tool's work by the part of its name before "__".
var toolLabels = map[string]string{
	"imagegen":  "drawing a picture",
	"vision":    "looking at a picture",
	"websearch": "searching the web",
	"radio":     "on the radio",
	"memory":    "remembering",
	"history":   "searching chat history",
	"paste":     "writing a paste",
	"profile":   "checking a profile",
	"irc":       "chatting",
	"task":      "working on a task",
}

func toolLabel(name string) string {
	prefix, _, _ := strings.Cut(name, "__")
	if label, ok := toolLabels[prefix]; ok {
		return label
	}
	return prefix
}

func requestLabel(operation string) string {
	if operation == "task" || operation == "delegate" {
		return "working on a task"
	}
	return "chatting"
}

func (s *Server) mzBridgeRoute(mux *http.ServeMux) {
	if s.mz == nil {
		return
	}
	mux.Handle("GET /api/v1/bridge", s.bridgeAuth(s.bridge))
}

// bridgeAuth lets in the status token or the console's own. Wrong tokens count towards the same
// lockout as the console's.
func (s *Server) bridgeAuth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client, now := clientOf(r), time.Now()
		if client != "" {
			if until := s.auth.locked(client, now); !until.IsZero() {
				fail(w, http.StatusTooManyRequests, fmt.Sprintf("too many wrong tokens; try again in %d minute(s)",
					int(until.Sub(now).Minutes())+1))
				return
			}
		}
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || !(tokenIs(got, s.cfg.StatusToken) || tokenIs(got, s.cfg.Token)) {
			if ok && client != "" && s.auth.fail(client, now) {
				s.log.Warn("admin_auth_locked", "client", client, "failures", authFailures,
					"minutes", int(authLockout.Minutes()))
			}
			fail(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if client != "" {
			s.auth.succeed(client)
		}
		next(w, r)
	})
}

func tokenIs(got, want string) bool {
	return want != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func (s *Server) bridge(w http.ResponseWriter, _ *http.Request) {
	out := bridgeView{Version: s.cfg.Version, Started: s.cfg.Started.Unix(), Now: time.Now().Unix(),
		State: s.mz.RunState(), IRC: len(s.mz.Offline()) == 0, Doing: []bridgeDoing{},
		LastActive: unix(s.work.LastActive())}

	tools := s.work.Tools()
	for _, t := range tools {
		out.Doing = append(out.Doing, bridgeDoing{What: toolLabel(t.Name), Since: t.Since.Unix()})
	}
	running := 0
	for _, f := range s.work.Inflight() {
		if !f.Running {
			out.Waiting++
			continue
		}
		running++
		// A request between tool calls is still thinking or writing its reply.
		if len(tools) == 0 {
			what := requestLabel(f.Operation)
			if !slices.ContainsFunc(out.Doing, func(d bridgeDoing) bool { return d.What == what }) {
				out.Doing = append(out.Doing, bridgeDoing{What: what, Since: f.Since.Unix()})
			}
		}
	}
	if out.State == "running" {
		out.State = "idle"
		if running > 0 || len(out.Doing) > 0 {
			out.State = "working"
		}
	}

	active := []string{core.TaskProposed, core.TaskQueued, core.TaskRunning}
	for _, n := range s.cfg.Networks {
		list, err := s.tasks.List(n, recentTasks)
		if err != nil {
			continue
		}
		for _, t := range list {
			if slices.Contains(active, t.Status) {
				out.Tasks++
			}
		}
	}
	respond(w, http.StatusOK, out)
}
