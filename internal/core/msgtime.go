// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import "github.com/alexschlessinger/pollytool/messages"

// MessageTimeKey is the metadata key holding when a message entered history (unix seconds).
const MessageTimeKey = "mizira_at"

// MessageTime reads when a message entered history, or 0 for one saved before times were kept.
func MessageTime(m messages.ChatMessage) int64 {
	switch v := m.Metadata[MessageTimeKey].(type) {
	case int64:
		return v
	case float64: // read back from JSON
		return int64(v)
	case int:
		return int64(v)
	}
	return 0
}
