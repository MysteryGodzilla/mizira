// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"strconv"
	"strings"
	"time"

	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// ignoreHarasser ignores a speaker for core.HarassmentIgnore once the gatekeeper has refused them for
// harassment core.HarassmentLimit times within core.HarassmentWindow. Admins are never ignored, and a
// bot's line isn't held against the owner whose nick it shares.
func ignoreHarasser(ctx irc.ChatContextInterface, reason string) bool {
	if !harassmentDue(ctx, reason) {
		return false
	}
	ignoreSpeaker(ctx, reason)
	return true
}

// harassmentDue records a harassment refusal and reports whether it is the one that gets the speaker
// ignored.
func harassmentDue(ctx irc.ChatContextInterface, reason string) bool {
	if reason == screenUnavailable || !core.IsHarassment(reason) || ctx.IsAdmin() || ctx.IsBotLine() {
		return false
	}
	return core.NoteHarassment(ctx.GetNetwork(), ctx.GetSource(), time.Now()) >= core.HarassmentLimit
}

// ignoreSpeaker ignores the speaker for core.HarassmentIgnore. Output to an ignored nick is dropped,
// so anything to tell them goes first.
func ignoreSpeaker(ctx irc.ChatContextInterface, reason string) {
	until := core.Ignores().AddWithInfo(ctx.GetNetwork(), ctx.GetSource(), core.HarassmentIgnore,
		core.IgnoreInfo{Kind: core.IgnoreByScreen, By: "screening", Reason: reason})
	ctx.GetLogger().Warn("harassment_ignored", "source", ctx.GetSource(), "reason", reason,
		"timeout", core.HarassmentIgnore.String(), "until", until.UTC().Format(time.RFC3339))
}

// screenedReply is what a refused message gets: the self-harm reply when it speaks of suicide or
// self-harm (if one is set), otherwise the usual refusal, and the ignore notice on its own line when
// the refusal got the speaker ignored.
func screenedReply(ctx irc.ChatContextInterface, reason, msg string, ignored bool) string {
	cfg := ctx.GetConfig()
	reply := cfg.Bot.ScreenRefusal
	if cfg.Bot.SelfHarmReply != "" && core.IsSelfHarm(reason, msg) {
		reply = cfg.Bot.SelfHarmReply
	}
	if ignored && cfg.Bot.IgnoreNotice != "" {
		notice := strings.NewReplacer("{nick}", ctx.GetSource(),
			"{minutes}", strconv.Itoa(int(core.HarassmentIgnore.Minutes())),
			"{reason}", strings.TrimRight(strings.ToLower(strings.TrimSpace(reason)), ".")).Replace(cfg.Bot.IgnoreNotice)
		reply += "\n" + notice
	}
	return reply
}
