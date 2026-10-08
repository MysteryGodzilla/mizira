// Copyright (C) 2026 BareMetal
// Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/irc"
)

// ScreenCommand handles +screen: put a nick behind the inbound gate and the
// outbound reply check. Edits screennicks/filternicks in the live config and
// persists them with the other runtime overrides.
type ScreenCommand struct{}

func (c *ScreenCommand) Name() string    { return "+screen" }
func (c *ScreenCommand) AdminOnly() bool { return true }

func (c *ScreenCommand) Execute(ctx irc.ChatContextInterface) {
	args := ctx.GetArgs()
	if len(args) < 2 || args[1] == "list" {
		listScreens(ctx)
		return
	}
	if args[1] == "remove" || args[1] == "del" {
		if len(args) < 3 {
			ctx.Reply("Usage: +screen remove <nick>")
			return
		}
		removeScreen(ctx, args[2])
		return
	}

	nick := args[1]
	// Their earlier turns in this conversation were never screened, so ScreenNick drops them.
	dropped, err := ScreenNick(ctx.GetConfig(), ctx.GetSession(), ctx.GetBotNick(), nick, ctx.GetSource(), ctx.GetLogger())
	switch {
	case errors.Is(err, ErrIsAdmin):
		ctx.Reply(fmt.Sprintf("%s is an admin - not screening", nick))
		return
	case errors.Is(err, ErrIsSelf):
		ctx.Reply("Refusing to screen myself")
		return
	case errors.Is(err, ErrAlreadyScreened):
		ctx.Reply(fmt.Sprintf("%s is already screened", nick))
		return
	}
	ctx.Reply(fmt.Sprintf("Screening %s: messages gated, replies checked, %d earlier turns dropped", nick, dropped))
}

// UnscreenCommand is an alias for "+screen remove <nick>".
type UnscreenCommand struct{}

func (c *UnscreenCommand) Name() string    { return "+unscreen" }
func (c *UnscreenCommand) AdminOnly() bool { return true }

func (c *UnscreenCommand) Execute(ctx irc.ChatContextInterface) {
	args := ctx.GetArgs()
	if len(args) < 2 {
		ctx.Reply("Usage: +unscreen <nick>")
		return
	}
	removeScreen(ctx, args[1])
}

func removeScreen(ctx irc.ChatContextInterface, nick string) {
	if !UnscreenNick(ctx.GetConfig(), nick, ctx.GetSource(), ctx.GetLogger()) {
		ctx.Reply(fmt.Sprintf("%s wasn't screened", nick))
		return
	}
	ctx.Reply(fmt.Sprintf("No longer screening %s", nick))
}

func listScreens(ctx irc.ChatContextInterface) {
	bot := ctx.GetConfig().Bot
	if len(config.List(&bot.ScreenNicks)) == 0 && len(config.List(&bot.FilterNicks)) == 0 {
		ctx.Reply("Nobody is screened. Usage: +screen <nick> | +screen remove <nick>")
		return
	}
	ctx.Reply(fmt.Sprintf("Screening - inbound: %s; outbound: %s",
		joinOrNone(config.List(&bot.ScreenNicks)), joinOrNone(config.List(&bot.FilterNicks))))
}

func joinOrNone(list []string) string {
	if len(list) == 0 {
		return "(none)"
	}
	return strings.Join(list, ", ")
}

// addNick appends nick unless it is already listed (case-insensitive).
func addNick(list *[]string, nick string) bool {
	cur := config.List(list)
	for _, have := range cur {
		if strings.EqualFold(strings.TrimSpace(have), nick) {
			return false
		}
	}
	config.SetList(list, slices.Concat(cur, []string{nick}))
	return true
}

func removeNick(list *[]string, nick string) bool {
	kept := []string{} // not nil: an emptied list is saved as [], not dropped
	removed := false
	for _, have := range config.List(list) {
		if strings.EqualFold(strings.TrimSpace(have), nick) {
			removed = true
			continue
		}
		kept = append(kept, have)
	}
	config.SetList(list, kept)
	return removed
}
