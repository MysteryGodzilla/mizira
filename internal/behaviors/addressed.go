// Copyright (C) 2023-2026 Alex Schlessinger and soulshack contributors
// Modified 2026 by BareMetal
// SPDX-License-Identifier: GPL-3.0-only

package behaviors

import (
	"strings"

	"github.com/lrstanley/girc"

	"B4reMetal/metald/internal/commands"
	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// AddressedBehavior handles messages addressed to the bot
type AddressedBehavior struct {
	CmdRegistry *commands.Registry
}

func (b *AddressedBehavior) Name() string {
	return "addressed"
}

func (b *AddressedBehavior) Events() []string {
	return []string{girc.PRIVMSG}
}

func (b *AddressedBehavior) Check(ctx irc.ChatContextInterface, event *girc.Event) bool {
	if ctx.IsPrivate() && ctx.GetConfig().Bot.IgnorePrivate {
		return false
	}
	if IsIgnoredSource(ctx) {
		return false
	}
	if len(ctx.GetArgs()) == 0 {
		return false
	}
	// T15: while paused or stopped, only admin commands get through, so +resume always works.
	// Everyone else is ignored and nothing is added to the conversation.
	if core.Halted() {
		return ctx.IsAdmin() && !ctx.IsBotLine() && b.isCommand(ctx, event)
	}
	// Other bots may chat with us, but never run commands, and only up to the loop limit.
	if ctx.IsBotLine() {
		return ctx.IsAddressed() && botMayReply(ctx)
	}

	// A registered command is handled even when the message doesn't address the bot by name.
	if b.isCommand(ctx, event) {
		return true
	}

	return ctx.IsAddressed() || ctx.IsPrivate()
}

// isCommand reports whether the message starts with a registered +command.
func (b *AddressedBehavior) isCommand(ctx irc.ChatContextInterface, event *girc.Event) bool {
	fields := strings.Fields(event.Last())
	if len(fields) == 0 {
		return false
	}
	_, ok := b.CmdRegistry.Get(irc.CanonicalCommand(fields[0], ctx.GetConfig().Bot.CommandPrefix))
	return ok
}

func (b *AddressedBehavior) Execute(ctx irc.ChatContextInterface, event *girc.Event) {
	// Bot lines share their owner's nick, so counting them would flood-mute the human too.
	// The bot loop limit caps them instead.
	if !ctx.IsBotLine() && CheckFlood(ctx) {
		return
	}
	// Lock-bypassing commands (e.g. +reset) exist to unstick a wedged lock,
	// so they must not queue behind whatever is holding it.
	if b.CmdRegistry.BypassesLock(ctx.GetCommand()) {
		b.CmdRegistry.Dispatch(ctx)
		return
	}
	core.WithRequestLock(ctx, ctx.GetLockKey(), "addressed", func() {
		b.CmdRegistry.Dispatch(ctx)
	}, func() {
		ctx.Reply("Request timed out waiting for previous operation to complete")
	})
}
