// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"testing"

	"github.com/alexschlessinger/pollytool/messages"
	"github.com/alexschlessinger/pollytool/sessions"
)

// A refused reply becomes what was posted; the speaker's words and everything else stay.
func TestReplaceReply(t *testing.T) {
	store := NewPersistentSessionStore(testContextDB(t), &sessions.Metadata{SystemPrompt: "prompt"})
	s, _ := store.Get("net/#swap")
	s.AddMessage(user("(nick:bob) hi"))
	s.AddMessage(messages.ChatMessage{Role: messages.MessageRoleAssistant, Content: "hello bob"})
	s.AddMessage(user("(nick:remy) bonito night"))
	s.AddMessage(messages.ChatMessage{Role: messages.MessageRoleAssistant, Content: "um... [otherbot],\nplease queue it up!"})

	if !ReplaceReply(s, "um... [otherbot], please queue it up!", "I'd rather not") {
		t.Fatal("reply not found")
	}
	h := s.GetHistory()
	if len(h) != 5 || h[0].Content != "prompt" || h[3].Content != "(nick:remy) bonito night" || h[4].Content != "I'd rather not" || h[2].Content != "hello bob" {
		t.Errorf("history: %+v", h)
	}
	// Smaller models echo their own label; the posted reply has it stripped.
	s.AddMessage(user("(nick:carol) hi"))
	s.AddMessage(messages.ChatMessage{Role: messages.MessageRoleAssistant, Content: "(nick:botty) I'm okay."})
	if !ReplaceReply(s, "I'm okay.", "I'd rather not") || s.GetHistory()[6].Content != "I'd rather not" {
		t.Errorf("echoed label: %+v", s.GetHistory())
	}
	if ReplaceReply(s, "never said", "x") {
		t.Error("replaced a reply that isn't there")
	}
}
