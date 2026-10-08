// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"regexp"
	"strings"

	"github.com/alexschlessinger/pollytool/messages"
)

// echoedLabel is the "(nick:x)" label a model sometimes copies onto its own answer; the posted
// reply has it stripped, the history copy doesn't.
var echoedLabel = regexp.MustCompile(`^\s*\(nick:[^)]*\)\s*`)

// ReplaceReply swaps a refused reply in history for what was posted instead, so the conversation
// shows what the channel saw and keeps the speaker's own words. It looks for the newest assistant
// message saying reply; false if there is none, and history is unchanged.
func ReplaceReply(session interface {
	GetHistory() []messages.ChatMessage
	Clear()
	AddMessage(messages.ChatMessage)
}, reply, posted string) bool {
	want := strings.Join(strings.Fields(reply), " ")
	if want == "" {
		return false
	}
	mu := CommitLock(session)
	mu.Lock()
	defer mu.Unlock()
	history := session.GetHistory()
	for i := len(history) - 1; i >= 0; i-- {
		m := history[i]
		if m.Role != messages.MessageRoleAssistant || strings.Join(strings.Fields(echoedLabel.ReplaceAllString(m.Content, "")), " ") != want {
			continue
		}
		history[i].Content = posted
		start := 0
		if len(history) > 0 && history[0].Role == messages.MessageRoleSystem {
			start = 1 // Clear puts the system prompt back
		}
		session.Clear()
		for _, h := range history[start:] {
			session.AddMessage(h)
		}
		return true
	}
	return false
}
