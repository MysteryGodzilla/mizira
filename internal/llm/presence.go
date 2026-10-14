// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"slices"
	"strings"
	"time"

	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// talkingWindow is how recently someone spoke to count as talking now rather than just here.
const talkingWindow = time.Hour

// presenceBlock is who is in the channel now, those who spoke in the last hour first, framed by
// presenceframe, or "" in a PM, with nobody else here, or when the conversation already carries the
// same list: it goes out only when it changes. Memories and the recap name people who left long ago;
// this is who she can actually talk to.
func presenceBlock(ctx irc.ChatContextInterface, known func(string) bool) string {
	cfg := ctx.GetConfig()
	if ctx.IsPrivate() {
		return ""
	}
	var here []string
	for _, u := range ctx.GetChannelUsers(cfg.Server.Channel) {
		if !strings.EqualFold(u.Nick, ctx.GetBotNick()) {
			here = append(here, u.Nick)
		}
	}
	if len(here) == 0 {
		return ""
	}
	spoke := map[string]time.Time{}
	if db, err := core.Context(); err == nil {
		if last, err := db.LastSpoke(ctx.GetSession().GetName(), time.Now().Add(-talkingWindow)); err == nil {
			spoke = last
		}
	}
	var talking, quiet []string
	for _, n := range here {
		if _, ok := spoke[strings.ToLower(n)]; ok {
			talking = append(talking, n)
		} else {
			quiet = append(quiet, n)
		}
	}
	slices.SortFunc(talking, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	slices.SortFunc(quiet, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	var parts []string
	if len(talking) > 0 {
		parts = append(parts, "talking lately: "+strings.Join(talking, ", "))
	}
	if len(quiet) > 0 {
		parts = append(parts, "here but quiet: "+strings.Join(quiet, ", "))
	}
	line := strings.Join(parts, " · ")
	if known(line) {
		return ""
	}
	return strings.TrimSpace(cfg.Bot.PresenceFrame) + "\n  " + line
}
