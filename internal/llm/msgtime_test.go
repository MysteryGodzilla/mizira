// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"testing"
	"time"

	"github.com/alexschlessinger/pollytool/messages"
	"github.com/alexschlessinger/pollytool/sessions"

	"B4reMetal/metald/internal/core"
)

// Each committed message records when it entered history, without touching the caller's message,
// and the time survives a restart.
func TestCommittedMessagesHaveTimes(t *testing.T) {
	db, err := core.Context()
	if err != nil {
		t.Fatal(err)
	}
	store := core.NewPersistentSessionStore(db, &sessions.Metadata{SystemPrompt: "you are a bot"})
	s, _ := store.Get("net/#msgtime")
	thinking := map[string]any{"other": "kept"}
	reply := messages.ChatMessage{Role: messages.MessageRoleAssistant, Content: "hi", Metadata: thinking}
	before := time.Now().Unix()
	commitExchange(s, []messages.ChatMessage{um("(nick:bob) hello"), reply})

	h := s.GetHistory()
	for _, m := range h[1:] {
		if at := core.MessageTime(m); at < before || at > time.Now().Unix() {
			t.Errorf("%s time = %d", m.Role, at)
		}
	}
	if h[2].Metadata["other"] != "kept" || len(thinking) != 1 {
		t.Errorf("metadata: history %v, caller's %v", h[2].Metadata, thinking)
	}

	store.Flush()
	restored, _ := core.NewPersistentSessionStore(db, &sessions.Metadata{SystemPrompt: "you are a bot"}).Get("net/#msgtime")
	if at := core.MessageTime(restored.GetHistory()[1]); at < before {
		t.Errorf("after a restart the time is %d", at)
	}
}
