// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"context"
	"testing"
	"time"
)

func TestActiveToolsFollowStartAndEnd(t *testing.T) {
	a := ToolStarted("imagegen__picture")
	b := ToolStarted("vision__view_image")
	got := ActiveTools()
	if len(got) < 2 || got[len(got)-2].Name != "imagegen__picture" || got[len(got)-1].Name != "vision__view_image" {
		t.Fatalf("running tools = %+v, want both, oldest first", got)
	}
	ToolEnded(a)
	ToolEnded(b)
	ToolEnded(b) // ending twice is harmless
	for _, tool := range ActiveTools() {
		if tool.Name == "imagegen__picture" || tool.Name == "vision__view_image" {
			t.Fatalf("%s still listed after it ended", tool.Name)
		}
	}
}

func TestLastActiveIsSetWhenARequestFinishes(t *testing.T) {
	before := time.Now()
	WithRequestLock(sourced{context.Background(), "alice"}, "test/#active", "addressed", func() {}, nil)
	if got := LastActive(); got.Before(before) {
		t.Fatalf("LastActive = %v, want after %v", got, before)
	}
}
