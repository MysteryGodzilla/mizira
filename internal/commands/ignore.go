// Copyright (C) 2026 BareMetal
// Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// defaultIgnoreDuration is used when +ignore is given a nick but no duration.
const defaultIgnoreDuration = time.Hour

// IgnoreCommand handles +ignore, for temporarily muting a nick.
type IgnoreCommand struct{}

func (c *IgnoreCommand) Name() string    { return "+ignore" }
func (c *IgnoreCommand) AdminOnly() bool { return true }

func (c *IgnoreCommand) Execute(ctx irc.ChatContextInterface) {
	args := ctx.GetArgs()

	if len(args) < 2 || args[1] == "list" {
		listIgnores(ctx)
		return
	}

	if args[1] == "remove" || args[1] == "del" {
		if len(args) < 3 {
			ctx.Reply("Usage: +ignore remove <nick>")
			return
		}
		removeIgnore(ctx, args[2])
		return
	}

	nick := args[1]
	duration := defaultIgnoreDuration
	rest := args[2:]
	// The duration is optional. Something that starts with a digit is meant as one, so a typo
	// like "3hh" is an error rather than quietly becoming the reason.
	if len(rest) > 0 && rest[0] != "" && unicode.IsDigit(rune(rest[0][0])) {
		parsed, err := time.ParseDuration(rest[0])
		if err != nil || parsed <= 0 {
			ctx.Reply(fmt.Sprintf("Invalid duration %q - use e.g. 30m, 2h, 24h", rest[0]))
			return
		}
		duration = parsed
		rest = rest[1:]
	}
	reason := strings.Join(rest, " ")

	expiry, err := IgnoreNick(ctx.GetConfig(), ctx.GetNetwork(), ctx.GetBotNick(), nick, duration, reason,
		ctx.GetSource(), ctx.GetRequestID(), ctx.GetLogger())
	switch {
	case errors.Is(err, ErrIsAdmin):
		ctx.Reply(fmt.Sprintf("%s is an admin - admins are exempt from ignore", nick))
		return
	case errors.Is(err, ErrIsSelf):
		ctx.Reply("Refusing to ignore myself")
		return
	}
	ctx.Reply(fmt.Sprintf("Ignoring %s for %s (until %s UTC)",
		nick, shortDuration(duration), expiry.UTC().Format("2006-01-02 15:04")))
}

// shortDuration drops Go's zero units: "2h", "1h30m", "45m" rather than "2h0m0s".
func shortDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// UnignoreCommand is a convenience alias for "+ignore remove <nick>".
type UnignoreCommand struct{}

func (c *UnignoreCommand) Name() string    { return "+unignore" }
func (c *UnignoreCommand) AdminOnly() bool { return true }

func (c *UnignoreCommand) Execute(ctx irc.ChatContextInterface) {
	args := ctx.GetArgs()
	if len(args) < 2 {
		ctx.Reply("Usage: +unignore <nick>")
		return
	}
	removeIgnore(ctx, args[1])
}

func removeIgnore(ctx irc.ChatContextInterface, nick string) {
	if UnignoreNick(ctx.GetNetwork(), nick, ctx.GetSource(), ctx.GetLogger()) {
		ctx.Reply(fmt.Sprintf("No longer ignoring %s", nick))
		return
	}
	ctx.Reply(fmt.Sprintf("%s wasn't ignored", nick))
}

// maxIgnoreListLines keeps +ignore list from flooding the channel itself.
const maxIgnoreListLines = 5

func listIgnores(ctx irc.ChatContextInterface) {
	entries := core.Ignores().List(ctx.GetNetwork())
	if len(entries) == 0 {
		ctx.Reply("Nobody is ignored. Usage: +ignore <nick> [duration] [reason]")
		return
	}
	for i, e := range entries {
		if i == maxIgnoreListLines {
			ctx.Reply(fmt.Sprintf("...and %d more", len(entries)-i))
			return
		}
		ctx.Reply(describeIgnore(e, time.Now()))
	}
}

// describeIgnore is one +ignore list line: who, time left, and how it happened, e.g.
// `bob: 42m left · bot, during alice's message · "kept spamming links"`.
func describeIgnore(e core.IgnoreEntry, now time.Time) string {
	line := fmt.Sprintf("%s: %s left", e.Nick, formatRemaining(e.Expiry.Sub(now)))
	switch e.Kind {
	case core.IgnoreByBot:
		line += fmt.Sprintf(" · bot, during %s's message", e.By)
	case core.IgnoreByAdmin:
		line += " · admin " + e.By
	case core.IgnoreByFlood:
		line += " · flood"
	}
	if e.Reason != "" {
		if e.Kind == core.IgnoreByFlood {
			line += " · " + e.Reason
		} else {
			line += fmt.Sprintf(" · %q", e.Reason)
		}
	}
	return line
}

// formatRemaining shows a duration the way people say it: "42m", "1h05m", "under a minute".
func formatRemaining(d time.Duration) string {
	if d < time.Minute {
		return "under a minute"
	}
	d = d.Round(time.Minute)
	if h := int(d.Hours()); h > 0 {
		return fmt.Sprintf("%dh%02dm", h, int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}

// isAdminNick reports whether nick belongs to a configured admin.
func isAdminNick(ctx irc.ChatContextInterface, nick string) bool {
	return isAdmin(ctx.GetConfig(), nick)
}
