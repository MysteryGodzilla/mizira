// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"strings"
	"time"

	"B4reMetal/metald/internal/irc"
)

// timeBlock is the time now in UTC, framed by timeframe. People's local times stay out of it: with
// a zone beside every message she brought up where people live, so local times come only from
// profile__localtime, when a question needs one.
func timeBlock(ctx irc.ChatContextInterface, now time.Time) string {
	return strings.TrimSpace(ctx.GetConfig().Bot.TimeFrame) + "\n  " + now.UTC().Format("Mon 2 Jan 2006 15:04") + " UTC"
}
