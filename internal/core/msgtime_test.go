// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"testing"

	"github.com/alexschlessinger/pollytool/messages"
)

func TestMessageTime(t *testing.T) {
	for v, want := range map[any]int64{int64(5): 5, float64(6): 6, 7: 7, "x": 0} {
		m := messages.ChatMessage{Metadata: map[string]any{MessageTimeKey: v}}
		if got := MessageTime(m); got != want {
			t.Errorf("%v -> %d, want %d", v, got, want)
		}
	}
	if MessageTime(messages.ChatMessage{}) != 0 {
		t.Error("a message without a time")
	}
}
