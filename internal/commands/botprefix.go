// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"fmt"
	"strings"

	"B4reMetal/metald/internal/irc"
)

// BotPrefixCommand handles +botprefix: the list of line prefixes that mark other bots
// (e.g. "[metalai]"). Their lines get replies only up to botreplylimit in a row (A10).
type BotPrefixCommand struct{}

func (c *BotPrefixCommand) Name() string    { return "+botprefix" }
func (c *BotPrefixCommand) AdminOnly() bool { return true }

func (c *BotPrefixCommand) Execute(ctx irc.ChatContextInterface) {
	args := ctx.GetArgs()
	bot := ctx.GetConfig().Bot
	if len(args) < 2 || args[1] == "list" {
		ctx.Reply(fmt.Sprintf("Bot prefixes: %s. Replies to bots: %d in a row, reset by a human or %s quiet.",
			joinOrNone(bot.BotPrefixes), bot.BotReplyLimit, bot.BotCooldown))
		return
	}
	if len(args) < 3 || (args[1] != "add" && args[1] != "remove") {
		ctx.Reply("Usage: +botprefix list | +botprefix add <prefix> | +botprefix remove <prefix>")
		return
	}

	// A prefix may contain spaces (e.g. an emoji before the tag).
	raw := strings.Join(args[2:], " ")
	prefix := irc.NormaliseBotPrefix(raw)

	if args[1] == "remove" {
		if !removeNick(&bot.BotPrefixes, prefix) {
			ctx.Reply(fmt.Sprintf("%s wasn't a bot prefix", prefix))
			return
		}
		PersistBotPrefixes(bot.BotPrefixes)
		ctx.GetLogger().Info("bot_prefix_removed", "prefix", prefix)
		ctx.Reply(fmt.Sprintf("No longer treating %s as a bot", prefix))
		return
	}

	if err := irc.ValidateBotPrefix(ctx.GetConfig(), raw); err != nil {
		ctx.Reply("Not added: " + err.Error())
		return
	}
	if !addNick(&bot.BotPrefixes, prefix) {
		ctx.Reply(fmt.Sprintf("%s is already a bot prefix", prefix))
		return
	}
	PersistBotPrefixes(bot.BotPrefixes)
	ctx.GetLogger().Info("bot_prefix_added", "prefix", prefix)
	ctx.Reply(fmt.Sprintf("Lines starting with %s now count as a bot", prefix))
}
