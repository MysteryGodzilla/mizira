// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package behaviors

import (
	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// botMayReply asks the loop tracker whether the bot may answer one more bot line in this
// channel, and counts the reply if so (A10).
func botMayReply(ctx irc.ChatContextInterface) bool {
	cfg := ctx.GetConfig()
	channel := cfg.Server.Channel
	ok, firstRefusal := core.BotLoop().Allow(ctx.GetNetwork(), channel, cfg.Bot.BotReplyLimit, cfg.Bot.BotCooldown)
	if firstRefusal {
		ctx.GetLogger().Info("bot_loop_limit",
			"limit", cfg.Bot.BotReplyLimit,
			"cooldown", cfg.Bot.BotCooldown.String())
	}
	return ok
}
