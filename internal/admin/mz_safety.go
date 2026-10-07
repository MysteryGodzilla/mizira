// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package admin

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// The Safety page: what the bot refused, screened out or quarantined recently, read from its log
// file, with counts per person.

type SafetyEventView struct {
	Time      int64  `json:"time"`
	Kind      string `json:"kind"`
	Event     string `json:"event"`
	Who       string `json:"who"`
	Channel   string `json:"channel"`
	Detail    string `json:"detail"`
	Message   string `json:"message,omitempty"` // the screened text, shown behind a click
	Suspicion string `json:"suspicion,omitempty"`
}

// PersonCount is how many safety events concern one person, by kind.
type PersonCount struct {
	Who    string         `json:"who"`
	Total  int            `json:"total"`
	ByKind map[string]int `json:"byKind"`
}

const (
	safetyMaxDays = 30
	safetyLimit   = 2000 // events read; the page shows the newest
	safetyShown   = 300
)

func (s *Server) mzSafetyRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /safety", s.safety)
}

func (s *Server) safety(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	days := 7
	if d := q.Get("days"); d != "" {
		n, err := strconv.Atoi(d)
		if err != nil || n < 1 || n > safetyMaxDays {
			fail(w, http.StatusBadRequest, "days is 1 to 30")
			return
		}
		days = n
	}
	all, err := s.mz.SafetyEvents(days, safetyLimit)
	if err != nil {
		fail(w, http.StatusInternalServerError, "the log file couldn't be read")
		return
	}

	people := map[string]*PersonCount{}
	kinds := map[string]int{}
	for _, e := range all {
		kinds[e.Kind]++
		if e.Kind == "console" || e.Who == "" {
			continue
		}
		key := strings.ToLower(e.Who)
		p := people[key]
		if p == nil {
			p = &PersonCount{Who: e.Who, ByKind: map[string]int{}}
			people[key] = p
		}
		p.Total++
		p.ByKind[e.Kind]++
	}
	counts := []PersonCount{}
	for _, p := range people {
		counts = append(counts, *p)
	}
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].Total != counts[j].Total {
			return counts[i].Total > counts[j].Total
		}
		return counts[i].Who < counts[j].Who
	})

	kind, who := q.Get("kind"), strings.ToLower(strings.TrimSpace(q.Get("who")))
	events := []SafetyEventView{}
	for _, e := range all {
		// The console's own audit trail is shown when asked for, not mixed into what people did.
		if kind == "" && e.Kind == "console" {
			continue
		}
		if (kind == "" || e.Kind == kind) && (who == "" || strings.ToLower(e.Who) == who) {
			events = append(events, e)
			if len(events) == safetyShown {
				break
			}
		}
	}
	respond(w, http.StatusOK, map[string]any{"events": events, "people": counts, "kinds": kinds, "days": days,
		"truncated": len(all) == safetyLimit})
}
