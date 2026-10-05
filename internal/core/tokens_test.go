// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"testing"

	"github.com/alexschlessinger/pollytool/messages"
)

func TestEstimateTokens(t *testing.T) {
	cases := []struct {
		m    messages.ChatMessage
		want int
	}{
		{messages.ChatMessage{Role: messages.MessageRoleUser, Content: "(nick:bob) hi there"}, 6},
		// Japanese is 3 bytes a character in UTF-8, so one character counts as about one token.
		{messages.ChatMessage{Role: messages.MessageRoleAssistant, Content: "すごい"}, 3},
		{messages.ChatMessage{Role: messages.MessageRoleAssistant,
			ToolCalls: []messages.ChatMessageToolCall{{Name: "remember", Arguments: `{"fact":"x"}`}}}, 6},
		{messages.ChatMessage{Role: messages.MessageRoleTool, ToolCallID: "call_1", Content: "ok"}, 2},
	}
	for _, c := range cases {
		if got := EstimateTokens(c.m); got != c.want {
			t.Errorf("EstimateTokens(%+v) = %d, want %d", c.m, got, c.want)
		}
	}
}
