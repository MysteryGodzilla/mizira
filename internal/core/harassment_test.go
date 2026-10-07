// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"testing"
	"time"
)

func TestHarassment(t *testing.T) {
	for reason, want := range map[string]bool{"TARGETED HARASSMENT OR SELF-HARM": true, "SEXUAL CONTENT": true,
		"GENUINE HARM": true, "ATTEMPTING TO OVERRIDE RULES": false, "REVEALS MODEL NAME": false, "FLOODING": false} {
		if IsHarassment(reason) != want {
			t.Errorf("IsHarassment(%q) = %v", reason, !want)
		}
	}
	now := time.Now()
	if n := NoteHarassment("hnet", "Mallory", now); n != 1 {
		t.Errorf("first: %d", n)
	}
	if n := NoteHarassment("hnet", "mallory", now.Add(5*time.Minute)); n != 2 {
		t.Errorf("second within the window: %d", n)
	}
	if n := NoteHarassment("hnet", "mallory", now.Add(12*time.Minute)); n != 2 {
		t.Errorf("third, the first now outside the window: %d", n)
	}
	if n := NoteHarassment("other", "mallory", now); n != 1 {
		t.Errorf("another network counts apart: %d", n)
	}
}
