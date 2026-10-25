// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"encoding/json"
	"testing"

	"github.com/alexschlessinger/pollytool/messages"

	"B4reMetal/metald/internal/irc"
)

// "a character sheet of you doing tasks" came as three picture calls, and two were refused.
func TestMergePictureCalls(t *testing.T) {
	const tool = "imagegen__picture"
	calls := []messages.ChatMessageToolCall{
		{ID: "1", Name: tool, Arguments: `{"prompt":"chibi alice sleeping","style":"chibi"}`},
		{ID: "2", Name: "web__search", Arguments: `{"query":"cats"}`},
		{ID: "3", Name: tool, Arguments: `{"prompt":"chibi alice typing on a laptop."}`},
		{ID: "4", Name: tool, Arguments: `{"prompt":"alice back in her hoodie","image_url":"https://example.com/a.png"}`},
		{ID: "5", Name: tool, Arguments: `{"prompt":"chibi alice holding a hammer"}`},
	}
	if n := mergePictureCalls(calls, tool); n != 2 {
		t.Fatalf("merged %d, want 2", n)
	}
	var first map[string]any
	json.Unmarshal([]byte(calls[0].Arguments), &first)
	want := "One picture of these 3 side by side, as a character sheet: 1) chibi alice sleeping. " +
		"2) chibi alice typing on a laptop. 3) chibi alice holding a hammer."
	if first["prompt"] != want || first["style"] != "chibi" {
		t.Errorf("first call = %v, want prompt %q and the chibi style", first, want)
	}
	for _, i := range []int{2, 4} {
		var args map[string]any
		json.Unmarshal([]byte(calls[i].Arguments), &args)
		if args[irc.MergedArg] != true {
			t.Errorf("call %s = %s, want it marked merged", calls[i].ID, calls[i].Arguments)
		}
	}
	if calls[1].Arguments != `{"query":"cats"}` || calls[3].Arguments != `{"prompt":"alice back in her hoodie","image_url":"https://example.com/a.png"}` {
		t.Errorf("other calls changed: %v", calls)
	}

	one := []messages.ChatMessageToolCall{{ID: "1", Name: tool, Arguments: `{"prompt":"a cat"}`}}
	if n := mergePictureCalls(one, tool); n != 0 || one[0].Arguments != `{"prompt":"a cat"}` {
		t.Errorf("a single call was changed: %v", one)
	}
}
