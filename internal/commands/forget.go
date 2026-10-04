// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"strings"

	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// ForgetCommand is +forget <id>, short for +memories forget <id>, the form people reach for first.
type ForgetCommand struct{}

func (c *ForgetCommand) Name() string    { return "+forget" }
func (c *ForgetCommand) AdminOnly() bool { return false }

func (c *ForgetCommand) Execute(ctx irc.ChatContextInterface) {
	store, err := core.Memories()
	if err != nil {
		ctx.GetLogger().Error("memory_store_unavailable", "error", err.Error())
		ctx.Reply("memory is unavailable")
		return
	}
	args := ctx.GetArgs()
	if len(args) > 1 {
		// "[6]" and "#6" as +memories shows them.
		args = append([]string{args[0], "forget", strings.Trim(args[1], "[]#")}, args[2:]...)
	} else {
		args = []string{args[0], "forget"}
	}
	(&MemoriesCommand{}).forget(ctx, store, args)
}
