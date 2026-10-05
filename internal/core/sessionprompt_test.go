// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"testing"

	"github.com/alexschlessinger/pollytool/messages"
	"github.com/alexschlessinger/pollytool/sessions"
)

// A new operator prompt reaches running conversations with their history, and conversations
// started later; a +prompt persona and work with its own prompt keep theirs.
func TestSetSystemPrompt(t *testing.T) {
	store := NewPersistentSessionStore(testContextDB(t), &sessions.Metadata{SystemPrompt: "old"})
	chat, _ := store.Get("net/#chat")
	chat.AddMessage(user("(nick:dave) hello"))
	persona, _ := store.Get("net/#persona")
	Prompts().Set("net/#persona", "a pirate", "dave")
	t.Cleanup(func() { Prompts().Clear("net/#persona") })
	work, _ := store.Get("task/net/1")
	work.SetMetadata(&sessions.Metadata{SystemPrompt: "old\n\ntask rules"})
	work.Clear()

	if n := store.SetSystemPrompt("old", "new"); n != 1 {
		t.Errorf("changed %d conversations, want 1", n)
	}
	h := chat.GetHistory()
	if len(h) != 2 || h[0].Role != messages.MessageRoleSystem || h[0].Content != "new" || h[1].Content != "(nick:dave) hello" {
		t.Errorf("channel history = %+v", h)
	}
	if got := persona.GetHistory()[0].Content; got != "old" {
		t.Errorf("persona conversation's prompt = %q, want it kept", got)
	}
	if got := work.GetHistory()[0].Content; got != "old\n\ntask rules" {
		t.Errorf("work's prompt = %q, want it kept", got)
	}
	later, _ := store.Get("net/#later")
	if got := later.GetHistory()[0].Content; got != "new" {
		t.Errorf("a conversation started later runs %q", got)
	}
}
