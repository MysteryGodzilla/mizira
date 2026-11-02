// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"testing"
	"time"

	"github.com/alexschlessinger/pollytool/messages"

	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
	mocktest "B4reMetal/metald/internal/testing"
)

func running(name string) int {
	n := 0
	for _, t := range core.ActiveTools() {
		if t.Name == name {
			n++
		}
	}
	return n
}

func TestToolCallsShowAsActivityUntilTheyEnd(t *testing.T) {
	ctx := mocktest.NewMockContext()
	h := newCallbackHandler(ctx, irc.NewChunker(make(chan string, 10), 350), ctx.GetConfig())
	draw := messages.ChatMessageToolCall{ID: "c1", Name: "activitytest__picture"}
	look := messages.ChatMessageToolCall{ID: "c2", Name: "activitytest__look"}

	h.onToolStart([]messages.ChatMessageToolCall{draw, look})
	h.onToolStart([]messages.ChatMessageToolCall{draw}) // a retry of the same call is still one
	if running(draw.Name) != 1 || running(look.Name) != 1 {
		t.Fatalf("running = %+v, want each call once", core.ActiveTools())
	}
	h.onToolEnd(draw, "url: https://files.example.com/a.png", time.Millisecond, nil)
	if running(draw.Name) != 0 || running(look.Name) != 1 {
		t.Fatalf("after the picture ended: %+v", core.ActiveTools())
	}
	// A cancelled request never reports its last call back.
	h.endTools()
	if running(look.Name) != 0 {
		t.Fatalf("endTools left %+v", core.ActiveTools())
	}
}
