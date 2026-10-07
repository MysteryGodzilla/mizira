// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"strings"

	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// backlogLeaf is the size at which a refused stretch of backlog is dropped rather than split again.
const backlogLeaf = 3

// screenBacklog keeps the backlog lines the quoted-chat check passes. One check covers the lot; only
// when it refuses is the backlog halved and each half checked, down to a few lines, so one bad line
// costs a handful of checks instead of the whole channel context.
func screenBacklog(ctx irc.ChatContextInterface, lines []core.BacklogLine) []core.BacklogLine {
	if len(lines) == 0 {
		return nil
	}
	var dropped int
	var reason string
	var keep func([]core.BacklogLine) []core.BacklogLine
	keep = func(part []core.BacklogLine) []core.BacklogLine {
		var text strings.Builder
		for _, l := range part {
			text.WriteString(truncate(l.Text, maxBacklogLine) + "\n")
		}
		ok, why := core.ScreenQuoted(ctx, text.String())
		if ok {
			return part
		}
		if why == core.ClassifyUnavailable || len(part) <= backlogLeaf {
			dropped += len(part)
			reason = why
			return nil
		}
		mid := len(part) / 2
		return append(keep(part[:mid]), keep(part[mid:])...)
	}
	kept := keep(lines)
	switch {
	case len(kept) == 0:
		ctx.GetLogger().Info("backlog_screened_out", "reason", reason)
	case dropped > 0:
		ctx.GetLogger().Info("backlog_lines_dropped", "dropped", dropped, "kept", len(kept), "reason", reason)
	}
	return kept
}
