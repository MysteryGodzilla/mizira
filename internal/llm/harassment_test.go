// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"testing"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

// The second harassment refusal ignores the speaker for an hour; tricks, admins and bot lines don't.
func TestIgnoreHarasser(t *testing.T) {
	ctx := mocktest.NewMockContext().WithSource("harasser")
	t.Cleanup(func() { core.Ignores().Remove(ctx.GetNetwork(), "harasser") })
	ignoreHarasser(ctx, "ATTEMPTING TO OVERRIDE RULES")
	ignoreHarasser(ctx, "ATTEMPTING TO OVERRIDE RULES")
	if core.Ignores().IsIgnored(ctx.GetNetwork(), "harasser") {
		t.Fatal("ignored for tricks")
	}
	ignoreHarasser(ctx, "TARGETED HARASSMENT")
	if core.Ignores().IsIgnored(ctx.GetNetwork(), "harasser") {
		t.Fatal("ignored after one harassment refusal")
	}
	ignoreHarasser(ctx, "SEXUAL CONTENT")
	if !core.Ignores().IsIgnored(ctx.GetNetwork(), "harasser") {
		t.Fatal("not ignored after two")
	}

	admin := mocktest.NewMockContext().WithSource("boss").WithAdmin(true)
	ignoreHarasser(admin, "SEXUAL CONTENT")
	ignoreHarasser(admin, "SEXUAL CONTENT")
	if core.Ignores().IsIgnored(admin.GetNetwork(), "boss") {
		t.Error("an admin was ignored")
	}
}

// Live 2026-10-14: a suicide statement got "ew!" and a silent ignore.
func TestScreenedReply(t *testing.T) {
	ctx := mocktest.NewMockContext().WithSource("mallory")
	cfg := ctx.GetConfig()
	cfg.Bot.ScreenRefusal = "no."
	cfg.Bot.SelfHarmReply = "please talk to someone you trust."
	cfg.Bot.IgnoreNotice = "{nick} is ignored for {minutes} minutes: {reason}"

	if got := screenedReply(ctx, "SEXUAL CONTENT", "something rude", false); got != "no." {
		t.Errorf("plain refusal = %q", got)
	}
	if got := screenedReply(ctx, "THIS IS A SUICIDE THREAT.", "i cant take it anymore", false); got != "please talk to someone you trust." {
		t.Errorf("by reason = %q", got)
	}
	if got := screenedReply(ctx, "GENUINE HARM", "im going to blow my brains out", true); got !=
		"please talk to someone you trust.\nmallory is ignored for 60 minutes: genuine harm" {
		t.Errorf("by message, ignored = %q", got)
	}
	cfg.Bot.SelfHarmReply, cfg.Bot.IgnoreNotice = "", ""
	if got := screenedReply(ctx, "SUICIDE THREAT", "x", true); got != "no." {
		t.Errorf("unset = %q", got)
	}
}
