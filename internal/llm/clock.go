// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"fmt"
	"strings"
	"time"

	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// timeBlock is the time now in UTC, with the local time of the speaker and of anyone the message
// names whose profile has a time zone, framed by timeframe. Converted here rather than by the model,
// which gets offsets and daylight saving wrong. A speaker with no time zone is said to have none, so
// she can ask before anything that depends on their clock.
func timeBlock(ctx irc.ChatContextInterface, text string, now time.Time) string {
	cfg := ctx.GetConfig()
	line := now.UTC().Format("Mon 2 Jan 2006 15:04") + " UTC"
	store, err := core.Memories()
	if err != nil {
		return strings.TrimSpace(cfg.Bot.TimeFrame) + "\n  " + line
	}
	people, err := store.People(ctx.GetNetwork())
	if err != nil {
		return strings.TrimSpace(cfg.Bot.TimeFrame) + "\n  " + line
	}
	self := cfg.Bot.Trigger
	if self == "" {
		self = ctx.GetBotNick()
	}
	speaker := strings.ToLower(ctx.Speaker())
	names := []string{speaker}
	if named, err := store.Mentioned(ctx.GetNetwork(), text, []string{speaker, self, cfg.Server.Channel}); err == nil {
		names = append(names, named...)
	}
	for _, n := range names {
		if loc, ok := people.Location(n); ok {
			p, _ := people.Get(n)
			line += fmt.Sprintf("; %s: %s (%s)", n, now.In(loc).Format("Mon 2 Jan 15:04 MST"), p.Timezone)
		} else if n == speaker && speaker != "" {
			line += fmt.Sprintf("; %s: time zone not known", n)
		}
	}
	return strings.TrimSpace(cfg.Bot.TimeFrame) + "\n  " + line
}
