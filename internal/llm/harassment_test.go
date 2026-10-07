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
