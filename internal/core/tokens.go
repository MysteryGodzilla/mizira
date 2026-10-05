// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import "github.com/alexschlessinger/pollytool/messages"

// bytesPerToken is measured, not assumed: on 972 lines of real IRC chat Gemma's tokenizer averaged
// 3.26 bytes a token (English and Japanese alike), where pollytool's estimate assumes 4 and so
// undercounted history by 1.23x. 3 keeps the estimate a little above the real count.
const bytesPerToken = 3

// EstimateTokens estimates a message's size in tokens from its text. Measure history with this,
// never pollytool's GetMessageTokens/GetTotalTokens, which count each reply's whole prompt.
func EstimateTokens(m messages.ChatMessage) int {
	n := len(m.Content) + len(m.Reasoning) + len(m.ToolCallID)
	for _, p := range m.Parts {
		if p.Type == "text" {
			n += len(p.Text)
		}
	}
	for _, tc := range m.ToolCalls {
		n += len(tc.Name) + len(tc.Arguments)
	}
	return n / bytesPerToken
}
