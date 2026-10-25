// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alexschlessinger/pollytool/messages"

	"B4reMetal/metald/internal/irc"
)

// mergePictureCalls folds a turn's new-picture calls to tool into its first, and returns how many it
// folded: asked for "a character sheet of different versions of you", the model drew each version as
// a call of its own, and as the tool runs once per request, all but one were refused. The first call
// draws them all side by side; the others are marked (irc.MergedArg) to answer that they're in it.
// Edits (an image_url) are left alone.
func mergePictureCalls(calls []messages.ChatMessageToolCall, tool string) int {
	var at []int
	var prompts []string
	var first map[string]any
	for i, c := range calls {
		var args map[string]any
		if c.Name != tool || json.Unmarshal([]byte(c.Arguments), &args) != nil {
			continue
		}
		prompt, _ := args["prompt"].(string)
		if edit, _ := args["image_url"].(string); edit != "" || strings.TrimSpace(prompt) == "" {
			continue
		}
		if first == nil {
			first = args
		} else if style, _ := args["style"].(string); style != "" && first["style"] == nil {
			first["style"] = style
		}
		at = append(at, i)
		prompts = append(prompts, strings.TrimRight(strings.TrimSpace(prompt), "."))
	}
	if len(at) < 2 {
		return 0
	}
	var b strings.Builder
	fmt.Fprintf(&b, "One picture of these %d side by side, as a character sheet:", len(prompts))
	for n, p := range prompts {
		fmt.Fprintf(&b, " %d) %s.", n+1, p)
	}
	first["prompt"] = b.String()
	merged, _ := json.Marshal(first)
	calls[at[0]].Arguments = string(merged)
	for _, i := range at[1:] {
		calls[i].Arguments = `{"` + irc.MergedArg + `":true}`
	}
	return len(at) - 1
}
