// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"testing"
	"time"
)

func TestClockTime(t *testing.T) {
	mel, err := time.LoadLocation("Australia/Melbourne")
	if err != nil {
		t.Skip("no zoneinfo")
	}
	bne, _ := time.LoadLocation("Australia/Brisbane")
	// Thu 15 Oct 2026, 03:00 UTC: 14:00 in Melbourne (daylight saving, +11), 13:00 in Brisbane (+10).
	now := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		loc       *time.Location
		at, day   string
		wantUTC   string
		wantError bool
	}{
		{mel, "4pm", "", "2026-10-15 05:00", false},
		{bne, "4pm", "", "2026-10-15 06:00", false},
		{mel, "16:30", "", "2026-10-15 05:30", false},
		{mel, "9.15 am", "", "2026-10-15 22:15", false}, // already past today: tomorrow
		{mel, "12am", "", "2026-10-15 13:00", false},
		{mel, "12pm", "tomorrow", "2026-10-16 01:00", false},
		{mel, "8pm", "friday", "2026-10-16 09:00", false},
		{mel, "8pm", "thursday", "2026-10-15 09:00", false},  // today, still ahead
		{mel, "10am", "thursday", "2026-10-21 23:00", false}, // today's has passed: next week
		{mel, "6pm", "2026-10-20", "2026-10-20 07:00", false},
		{mel, "9am", "today", "", true},
		{mel, "13pm", "", "", true},
		{mel, "25:00", "", "", true},
		{mel, "soon", "", "", true},
		{mel, "4pm", "someday", "", true},
	} {
		got, err := clockTime(now, c.loc, c.at, c.day)
		if c.wantError {
			if err == nil {
				t.Errorf("%s %s: got %v, want an error", c.at, c.day, got)
			}
			continue
		}
		if err != nil || got.UTC().Format("2006-01-02 15:04") != c.wantUTC {
			t.Errorf("%s %s in %s = %v %v, want %s UTC", c.at, c.day, c.loc, got.UTC(), err, c.wantUTC)
		}
	}
}
