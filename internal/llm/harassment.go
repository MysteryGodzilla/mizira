// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"time"

	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// ignoreHarasser ignores a speaker for core.HarassmentIgnore once the gatekeeper has refused them for
// harassment core.HarassmentLimit times within core.HarassmentWindow. Admins are never ignored, and a
// bot's line isn't held against the owner whose nick it shares.
func ignoreHarasser(ctx irc.ChatContextInterface, reason string) {
	if reason == screenUnavailable || !core.IsHarassment(reason) || ctx.IsAdmin() || ctx.IsBotLine() {
		return
	}
	if core.NoteHarassment(ctx.GetNetwork(), ctx.GetSource(), time.Now()) < core.HarassmentLimit {
		return
	}
	until := core.Ignores().AddWithInfo(ctx.GetNetwork(), ctx.GetSource(), core.HarassmentIgnore,
		core.IgnoreInfo{Kind: core.IgnoreByScreen, By: "screening", Reason: reason})
	ctx.GetLogger().Warn("harassment_ignored", "source", ctx.GetSource(), "reason", reason,
		"timeout", core.HarassmentIgnore.String(), "until", until.UTC().Format(time.RFC3339))
}
