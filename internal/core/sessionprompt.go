// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"github.com/alexschlessinger/pollytool/messages"
	"github.com/alexschlessinger/pollytool/sessions"
)

// SetSystemPrompt puts prompt in place of old in every conversation still running old, and in every
// conversation created from now on, keeping their history. A conversation with a +prompt persona,
// or work with its own prompt, doesn't run old and is left alone. It returns how many changed.
func (s *PersistentSessionStore) SetSystemPrompt(old, prompt string) int {
	s.prompt.Store("", prompt)
	n := 0
	s.wrappers.Range(func(k, v any) bool {
		w := v.(*PersistentSession)
		if Prompts().Active(k.(string)) || w.GetMetadata().SystemPrompt != old {
			return true
		}
		mu := CommitLock(w)
		mu.Lock()
		withPrompt(w, prompt)
		mu.Unlock()
		n++
		return true
	})
	return n
}

// withPrompt sets a session's system prompt, which pollytool keeps as the first history message,
// and keeps the rest of the history.
func withPrompt(session sessions.Session, prompt string) {
	md := *session.GetMetadata()
	md.SystemPrompt = prompt
	session.SetMetadata(&md)
	history := session.GetHistory()
	if len(history) > 0 && history[0].Role == messages.MessageRoleSystem {
		history = history[1:]
	}
	session.Clear()
	for _, m := range history {
		session.AddMessage(m)
	}
}
