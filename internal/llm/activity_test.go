// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"fmt"
	"sync"
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

// pollytool runs one turn's tool calls in parallel and reports each end from its own goroutine.
func TestParallelToolEndsShareTheHandlerSafely(t *testing.T) {
	ctx := mocktest.NewMockContext()
	h := newCallbackHandler(ctx, irc.NewChunker(make(chan string, 200), 350), ctx.GetConfig())
	draw := irc.ClaimTool[irc.ClaimDraw]
	results := []struct{ name, result string }{
		{draw, "url: https://files.example.com/a.png"},
		{draw, "Not done: the picture service is busy."},
		{"paralleltest__search", "Error: refused, that request is not allowed"},
		{"paralleltest__wave", "action: waves"},
		{"paralleltest__paste", "url: https://files.example.com/b.txt"},
	}
	var calls []messages.ChatMessageToolCall
	for i, r := range results {
		calls = append(calls, messages.ChatMessageToolCall{ID: fmt.Sprintf("p%d", i), Name: r.name})
	}
	h.onToolStart(calls)

	var wg sync.WaitGroup
	for round := 0; round < 20; round++ {
		for i, r := range results {
			wg.Add(1)
			go func() {
				defer wg.Done()
				h.onToolEnd(calls[i], r.result, time.Millisecond, nil)
			}()
		}
	}
	wg.Wait()
	if h.lastToolURL == "" || !h.shown {
		t.Fatalf("lastToolURL = %q, shown = %v; want both set", h.lastToolURL, h.shown)
	}
}
