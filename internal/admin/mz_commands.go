// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package admin

import "net/http"

// The Commands page: every IRC command, what it does and who may use it, from the bot's own registry.

type CommandView struct {
	Name    string   `json:"name"`
	Admin   bool     `json:"admin"`
	Group   string   `json:"group"`
	Usage   []string `json:"usage"`
	Text    string   `json:"text"`
	Feature string   `json:"feature,omitempty"` // the feature it needs ("work", "radio"), if any
}

type CheatsheetView struct {
	Name     string        `json:"name"`     // what to address her as
	NeedName bool          `json:"needName"` // commands only work after her name
	Groups   []string      `json:"groups"`
	Commands []CommandView `json:"commands"`
}

func (s *Server) commands(w http.ResponseWriter, _ *http.Request) {
	respond(w, http.StatusOK, s.mz.Commands())
}
