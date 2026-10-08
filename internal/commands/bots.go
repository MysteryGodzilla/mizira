// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"errors"
	"fmt"
	"strings"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/irc"
)

// BotsCommand handles +bots: who counts as another bot. A bot is recognised by its own nick
// (bots with their own account) or by a line prefix such as "[metalai]" (bots that share their
// owner's nick). Bot lines get replies only up to botreplylimit in a row (A10).
type BotsCommand struct{}

func (c *BotsCommand) Name() string    { return "+bots" }
func (c *BotsCommand) AdminOnly() bool { return true }

const botsUsage = "Usage: +bots list | +bots add nick <nick> | +bots add prefix <prefix> | +bots remove nick <nick> | +bots remove prefix <prefix>"

func (c *BotsCommand) Execute(ctx irc.ChatContextInterface) {
	args := ctx.GetArgs()
	bot := ctx.GetConfig().Bot
	if len(args) < 2 || args[1] == "list" {
		ctx.Reply(fmt.Sprintf("Bot nicks: %s. Bot prefixes: %s. Replies to bots: %d in a row, reset by a human or %s quiet.",
			joinOrNone(config.List(&bot.BotNicks)), joinOrNone(config.List(&bot.BotPrefixes)), bot.BotReplyLimit, bot.BotCooldown))
		return
	}
	if len(args) < 4 || (args[1] != "add" && args[1] != "remove") || (args[2] != "nick" && args[2] != "prefix") {
		ctx.Reply(botsUsage)
		return
	}
	adding := args[1] == "add"
	value := args[3]
	if args[2] == "prefix" {
		// A prefix may contain spaces (e.g. an emoji before the tag).
		value = strings.Join(args[3:], " ")
	}
	value, err := ChangeBots(ctx.GetConfig(), ctx.GetBotNick(), adding, BotKind(args[2]), value, ctx.GetSource(), ctx.GetLogger())
	switch {
	case errors.Is(err, ErrAlreadyListed):
		ctx.Reply(fmt.Sprintf("%s is already a bot %s", value, args[2]))
		return
	case errors.Is(err, ErrNotListed):
		ctx.Reply(fmt.Sprintf("%s wasn't a bot %s", value, args[2]))
		return
	case err != nil:
		ctx.Reply("Not added: " + err.Error())
		return
	}
	if adding {
		ctx.Reply(fmt.Sprintf("Now treating bot %s %s as a bot", args[2], value))
	} else {
		ctx.Reply(fmt.Sprintf("No longer treating bot %s %s as a bot", args[2], value))
	}
}
