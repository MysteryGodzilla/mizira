// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"reflect"
	"strings"
	"testing"

	mocktest "B4reMetal/metald/internal/testing"
)

func runBotPrefix(ctx *mocktest.MockChatContext, args ...string) string {
	ctx.WithArgs(append([]string{"+botprefix"}, args...)...)
	ctx.Replies = nil
	(&BotPrefixCommand{}).Execute(ctx)
	return strings.Join(ctx.Replies, "\n")
}

func TestBotPrefixAddListRemove(t *testing.T) {
	useTempOverrides(t)
	ctx := mocktest.NewMockContext().WithAdmin(true)
	cfg := ctx.GetConfig()
	cfg.Bot.BotPrefixes = nil

	if out := runBotPrefix(ctx, "add", "\x0304[MetalAI]\x03"); !strings.Contains(out, "now count") {
		t.Fatalf("add: %q", out)
	}
	// Stored without colours and in lower case, so the same tag can't be added twice.
	if !reflect.DeepEqual(cfg.Bot.BotPrefixes, []string{"[metalai]"}) {
		t.Fatalf("prefixes = %q", cfg.Bot.BotPrefixes)
	}
	if out := runBotPrefix(ctx, "add", "[metalai]"); !strings.Contains(out, "already") {
		t.Fatalf("duplicate add: %q", out)
	}
	if out := runBotPrefix(ctx, "list"); !strings.Contains(out, "[metalai]") {
		t.Fatalf("list: %q", out)
	}
	if out := runBotPrefix(ctx, "remove", "[METALAI]"); !strings.Contains(out, "No longer") {
		t.Fatalf("remove: %q", out)
	}
	if len(cfg.Bot.BotPrefixes) != 0 {
		t.Fatalf("prefixes after remove = %q", cfg.Bot.BotPrefixes)
	}
}

func TestBotPrefixRefusesBadPrefixes(t *testing.T) {
	useTempOverrides(t)
	ctx := mocktest.NewMockContext().WithAdmin(true)
	cfg := ctx.GetConfig()
	cfg.Bot.BotPrefixes = nil
	cfg.Bot.ResponsePrefix = "[testbot]"

	for _, prefix := range []string{"[a", "\x0304\x03ab", "[TestBot]"} {
		if out := runBotPrefix(ctx, "add", prefix); !strings.HasPrefix(out, "Not added") {
			t.Errorf("add %q: got %q, want a refusal", prefix, out)
		}
	}
	if len(cfg.Bot.BotPrefixes) != 0 {
		t.Fatalf("refused prefixes were stored: %q", cfg.Bot.BotPrefixes)
	}
}

func TestBotPrefixIsAdminOnly(t *testing.T) {
	if !(&BotPrefixCommand{}).AdminOnly() {
		t.Fatal("+botprefix changes who the bot treats as a bot; it must be admin-only")
	}
}

func TestPersistBotPrefixesSurvivesRestart(t *testing.T) {
	useTempOverrides(t)
	PersistBotPrefixes([]string{"[metalai]"})

	cfg := mocktest.NewMockContext().GetConfig()
	cfg.Bot.BotPrefixes = nil
	ApplyOverrides(cfg)
	if !reflect.DeepEqual(cfg.Bot.BotPrefixes, []string{"[metalai]"}) {
		t.Fatalf("prefixes after restart = %q", cfg.Bot.BotPrefixes)
	}
}
