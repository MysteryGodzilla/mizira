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

func runBots(ctx *mocktest.MockChatContext, args ...string) string {
	ctx.WithArgs(append([]string{"+bots"}, args...)...)
	ctx.Replies = nil
	(&BotsCommand{}).Execute(ctx)
	return strings.Join(ctx.Replies, "\n")
}

func botsCtx(t *testing.T) *mocktest.MockChatContext {
	t.Helper()
	useTempOverrides(t)
	ctx := mocktest.NewMockContext().WithAdmin(true)
	cfg := ctx.GetConfig()
	cfg.Bot.BotPrefixes = nil
	cfg.Bot.BotNicks = nil
	cfg.Bot.ResponsePrefix = "[testbot]"
	cfg.Server.Nick = "testbot"
	return ctx
}

func TestBotsAddListRemovePrefix(t *testing.T) {
	ctx := botsCtx(t)
	cfg := ctx.GetConfig()

	if out := runBots(ctx, "add", "prefix", "\x0304[MetalAI]\x03"); !strings.Contains(out, "Now treating") {
		t.Fatalf("add: %q", out)
	}
	// Stored without colours and in lower case, so the same tag can't be added twice.
	if !reflect.DeepEqual(cfg.Bot.BotPrefixes, []string{"[metalai]"}) {
		t.Fatalf("prefixes = %q", cfg.Bot.BotPrefixes)
	}
	if out := runBots(ctx, "add", "prefix", "[metalai]"); !strings.Contains(out, "already") {
		t.Fatalf("duplicate add: %q", out)
	}
	if out := runBots(ctx, "list"); !strings.Contains(out, "[metalai]") {
		t.Fatalf("list: %q", out)
	}
	if out := runBots(ctx, "remove", "prefix", "[METALAI]"); !strings.Contains(out, "No longer") {
		t.Fatalf("remove: %q", out)
	}
	if len(cfg.Bot.BotPrefixes) != 0 {
		t.Fatalf("prefixes after remove = %q", cfg.Bot.BotPrefixes)
	}
}

func TestBotsAddListRemoveNick(t *testing.T) {
	ctx := botsCtx(t)
	cfg := ctx.GetConfig()

	if out := runBots(ctx, "add", "nick", "carolbot"); !strings.Contains(out, "Now treating") {
		t.Fatalf("add: %q", out)
	}
	if out := runBots(ctx, "add", "nick", "CarolBot"); !strings.Contains(out, "already") {
		t.Fatalf("duplicate add (other case): %q", out)
	}
	if out := runBots(ctx, "list"); !strings.Contains(out, "carolbot") {
		t.Fatalf("list: %q", out)
	}
	if out := runBots(ctx, "remove", "nick", "CAROLBOT"); !strings.Contains(out, "No longer") {
		t.Fatalf("remove: %q", out)
	}
	if len(cfg.Bot.BotNicks) != 0 {
		t.Fatalf("nicks after remove = %q", cfg.Bot.BotNicks)
	}
}

func TestBotsRefusesBadEntries(t *testing.T) {
	ctx := botsCtx(t)
	cfg := ctx.GetConfig()

	refused := [][]string{
		{"add", "prefix", "[a"},
		{"add", "prefix", "\x0304\x03ab"},
		{"add", "prefix", "[TestBot]"}, // own prefix
		{"add", "nick", "TestBot"},     // own nick: the owner chats on it
		{"add", "nick", "#chat"},       // not a nick
		{"add", "nick", "alice,bob"},   // not one nick
	}
	for _, args := range refused {
		if out := runBots(ctx, args...); !strings.HasPrefix(out, "Not added") {
			t.Errorf("%v: got %q, want a refusal", args, out)
		}
	}
	if len(cfg.Bot.BotPrefixes) != 0 || len(cfg.Bot.BotNicks) != 0 {
		t.Fatalf("refused entries were stored: %q %q", cfg.Bot.BotPrefixes, cfg.Bot.BotNicks)
	}
	for _, args := range [][]string{{"add"}, {"add", "nick"}, {"add", "host", "x"}, {"drop", "nick", "x"}} {
		if out := runBots(ctx, args...); out != botsUsage {
			t.Errorf("%v: got %q, want usage", args, out)
		}
	}
}

func TestBotsIsAdminOnly(t *testing.T) {
	if !(&BotsCommand{}).AdminOnly() {
		t.Fatal("+bots changes who the bot treats as a bot; it must be admin-only")
	}
}

func TestPersistBotsSurvivesRestart(t *testing.T) {
	useTempOverrides(t)
	PersistBots([]string{"[metalai]"}, []string{"carolbot"})

	cfg := mocktest.NewMockContext().GetConfig()
	cfg.Bot.BotPrefixes = nil
	cfg.Bot.BotNicks = nil
	ApplyOverrides(cfg)
	if !reflect.DeepEqual(cfg.Bot.BotPrefixes, []string{"[metalai]"}) || !reflect.DeepEqual(cfg.Bot.BotNicks, []string{"carolbot"}) {
		t.Fatalf("after restart: prefixes %q nicks %q", cfg.Bot.BotPrefixes, cfg.Bot.BotNicks)
	}
}
