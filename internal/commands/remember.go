// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"fmt"
	"strings"

	"github.com/lrstanley/girc"

	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// RememberCommand handles +remember: save a fact without relying on the model choosing to call
// its memory tool. The reply names the saved id, so the user has proof it was stored.
//
//	+remember <fact>          a fact about yourself
//	+remember <nick>: <fact>  a fact about someone else
type RememberCommand struct{}

func (c *RememberCommand) Name() string    { return "+remember" }
func (c *RememberCommand) AdminOnly() bool { return false }

// RequiresLock is true because the memory check asks the model (the classifier), and every
// model call goes through the request locks and the global concurrency limit.
func (c *RememberCommand) RequiresLock() bool { return true }

func (c *RememberCommand) Execute(ctx irc.ChatContextInterface) {
	subject, fact := parseRemember(ctx.GetSource(), ctx.GetArgs()[1:])
	if fact == "" {
		ctx.Reply("Usage: +remember <fact about you> | +remember <nick>: <fact>")
		return
	}
	res := irc.RememberChecked(ctx, subject, fact)
	switch {
	case res.Saved:
		ctx.Reply(fmt.Sprintf("Saved #%d about %s: %s", res.ID, subject, fact))
	case res.Instruction:
		ctx.Reply(fmt.Sprintf("Not saved: that reads as an instruction, not a fact (%s)", res.Reason))
	default:
		ctx.Reply("Not saved: " + res.Reason)
	}
}

// parseRemember splits "+remember" arguments into who the fact is about and the fact. A first
// word ending in ':' that is a valid nick or channel names the subject; otherwise the fact is about the
// speaker.
func parseRemember(speaker string, args []string) (subject, fact string) {
	if len(args) > 1 {
		// A channel names room memory: "+remember #chat: <fact>".
		if nick, ok := strings.CutSuffix(args[0], ":"); ok && (girc.IsValidNick(nick) || girc.IsValidChannel(nick)) {
			return nick, strings.TrimSpace(strings.Join(args[1:], " "))
		}
	}
	return speaker, strings.TrimSpace(strings.Join(args, " "))
}

// RecallCommand handles +recall [nick]: what is stored about someone (yourself by default).
type RecallCommand struct{}

func (c *RecallCommand) Name() string    { return "+recall" }
func (c *RecallCommand) AdminOnly() bool { return false }

func (c *RecallCommand) Execute(ctx irc.ChatContextInterface) {
	subject := ctx.GetSource()
	if args := ctx.GetArgs(); len(args) > 1 {
		subject = strings.TrimSuffix(args[1], ":")
	}
	store, err := core.Memories()
	if err != nil {
		ctx.GetLogger().Error("memory_store_unavailable", "error", err.Error())
		ctx.Reply("memory is unavailable right now")
		return
	}
	(&MemoriesCommand{}).about(ctx, store, subject)
}
