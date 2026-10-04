// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

// ScreenQuoted checks channel lines the model is about to read as quoted chat (the channel
// backlog, chat history search results). With screenall on, every non-admin's own message goes
// through the gatekeeper, so lines that never did must not reach the model unchecked: they are
// judged by the gatekeeper policy, and refused if the check fails.
func ScreenQuoted(ctx ChatContextInterface, lines string) (bool, string) {
	cfg := ctx.GetConfig()
	if !cfg.Bot.ScreenAll {
		return true, ""
	}
	return Classify(ctx, cfg.Bot.GatekeeperPolicy, "CHANNEL LINES", lines, false)
}
