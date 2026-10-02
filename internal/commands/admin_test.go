// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"strings"
	"testing"

	mocktest "B4reMetal/metald/internal/testing"
)

func runAdmins(ctx *mocktest.MockChatContext, args ...string) string {
	ctx.WithArgs(append([]string{"+admins"}, args...)...)
	ctx.Replies = nil
	(&AdminCommand{}).Execute(ctx)
	return strings.Join(ctx.Replies, "\n")
}

func TestAdminsAddWildcardsAndCloaks(t *testing.T) {
	useTempOverrides(t)
	ctx := mocktest.NewMockContext().WithAdmin(true).WithSystem(mocktest.NewMockSystem())
	ctx.GetConfig().Bot.Admins = nil

	if out := runAdmins(ctx, "add", "alice!*@cloak/staff/alice"); out != "Added admin: alice!*@cloak/staff/alice" {
		t.Fatalf("cloak mask: %q", out)
	}
	if out := runAdmins(ctx, "add", "bob!*@*"); !strings.Contains(out, "Added admin") || !strings.Contains(out, "note:") {
		t.Fatalf("broad mask should be added with a note: %q", out)
	}
}

func TestAdminsAddRefusesEveryone(t *testing.T) {
	useTempOverrides(t)
	ctx := mocktest.NewMockContext().WithAdmin(true).WithSystem(mocktest.NewMockSystem())
	ctx.GetConfig().Bot.Admins = nil

	for _, mask := range []string{"*!*@*", "alice", "alice!*@a,b"} {
		if out := runAdmins(ctx, "add", mask); !strings.HasPrefix(out, "Invalid hostmask") {
			t.Errorf("add %q: got %q, want a refusal", mask, out)
		}
	}
	if len(ctx.GetConfig().Bot.Admins) != 0 {
		t.Fatalf("refused masks were stored: %q", ctx.GetConfig().Bot.Admins)
	}
}
