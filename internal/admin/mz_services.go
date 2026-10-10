// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package admin

import (
	"net/http"

	"B4reMetal/metald/internal/core"
)

// ServiceView is one outside service on the dashboard: a few figures and a link to its own page.
// Keys stay with the bot; the page only gets the figures.
type ServiceView struct {
	ID    string             `json:"id"`
	Name  string             `json:"name"`
	Link  string             `json:"link,omitempty"`
	Stats []core.ServiceStat `json:"stats"`
	Meter *Meter             `json:"meter,omitempty"`
	Error string             `json:"error,omitempty"`
}

// Meter is a spend bar: used against a limit, red once over it, and a warning when the pace runs out
// before the period ends.
type Meter struct {
	Label   string  `json:"label"`
	Used    float64 `json:"used"`
	Limit   float64 `json:"limit"`
	Over    bool    `json:"over"`
	Note    string  `json:"note,omitempty"`
	Warning string  `json:"warning,omitempty"`
}

func (s *Server) services(w http.ResponseWriter, r *http.Request) {
	out := s.mz.Services(r.Context())
	if out == nil {
		out = []ServiceView{}
	}
	respond(w, http.StatusOK, map[string]any{"services": out})
}
