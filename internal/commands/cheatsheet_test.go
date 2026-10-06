// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"slices"
	"testing"
)

// Grouped in CheatGroups order, written with the configured prefix, admin-only taken from the
// command itself, and a command without an entry still listed.
func TestCheatsheet(t *testing.T) {
	r := NewRegistry()
	r.Register(&StopCommand{})
	r.Register(&StatsCommand{})
	r.Register(&RecapCommand{})
	r.Register(&CompletionCommand{})
	r.Register(&unlisted{})
	got := Cheatsheet(r, "~")
	var names []string
	for _, e := range got {
		names = append(names, e.Name)
	}
	if !slices.Equal(names, []string{"~stats", "~recap", "~stop", "~unlisted"}) {
		t.Fatalf("order: %v", names)
	}
	recap := got[1]
	if !recap.Admin || recap.Group != "Conversation" || recap.Usage[2] != "~recap fold" {
		t.Errorf("recap: %+v", recap)
	}
	if got[0].Admin || got[3].Group != "Other" {
		t.Errorf("stats %+v, unlisted %+v", got[0], got[3])
	}
	if m := MissingHelp(r); !slices.Equal(m, []string{"+unlisted"}) {
		t.Errorf("missing: %v", m)
	}
}

type unlisted struct{ StatsCommand }

func (unlisted) Name() string { return "+unlisted" }
