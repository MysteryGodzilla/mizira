// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package behaviors

import (
	"testing"
	"time"

	"github.com/lrstanley/girc"

	"B4reMetal/metald/internal/commands"
	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

// botLineCtx is a message from another bot. Each test gets its own network name so the
// process-wide loop tracker starts fresh.
func botLineCtx(network string, addressed bool, limit int, args ...string) *mocktest.MockChatContext {
	ctx := mocktest.NewMockContext().WithAddressed(addressed).WithArgs(args...).WithSource("bob")
	ctx.BotLine = true
	cfg := ctx.GetConfig()
	cfg.Server.Name = network
	cfg.Bot.BotReplyLimit = limit
	cfg.Bot.BotCooldown = 10 * time.Minute
	return ctx
}

func addressedWithVersion() *AddressedBehavior {
	reg := commands.NewRegistry()
	reg.Register(&commands.VersionCommand{Version: "test"})
	return &AddressedBehavior{CmdRegistry: reg}
}

func TestBotLineRepliesUpToLimit(t *testing.T) {
	b := addressedWithVersion()
	ev := &girc.Event{Command: girc.PRIVMSG, Params: []string{"#test", "[otherbot] hey testbot"}}
	for i := 1; i <= 3; i++ {
		if !b.Check(botLineCtx("botloop-limit", true, 3, "[otherbot]", "hey", "testbot"), ev) {
			t.Fatalf("bot line %d should get a reply under a limit of 3", i)
		}
	}
	if b.Check(botLineCtx("botloop-limit", true, 3, "[otherbot]", "hey", "testbot"), ev) {
		t.Fatal("the 4th bot line in a row must not get a reply")
	}
	core.BotLoop().HumanSpoke("botloop-limit", "#test")
	if !b.Check(botLineCtx("botloop-limit", true, 3, "[otherbot]", "hey", "testbot"), ev) {
		t.Fatal("after a human speaks, the bots may chat again")
	}
}

func TestBotLineMustAddressTheBot(t *testing.T) {
	b := addressedWithVersion()
	ev := &girc.Event{Command: girc.PRIVMSG, Params: []string{"#test", "[otherbot] talking to someone else"}}
	if b.Check(botLineCtx("botloop-unaddressed", false, 3, "[otherbot]", "talking"), ev) {
		t.Fatal("a bot line that doesn't address us gets no reply")
	}
	// An unaddressed line must not use up the budget either.
	for i := 1; i <= 3; i++ {
		if !b.Check(botLineCtx("botloop-unaddressed", true, 3, "[otherbot]", "testbot"), ev) {
			t.Fatalf("addressed bot line %d was refused; unaddressed lines must not count", i)
		}
	}
}

// Bots never run commands, even ones a human could use without addressing the bot.
func TestBotLineNeverRunsCommands(t *testing.T) {
	b := addressedWithVersion()
	ev := &girc.Event{Command: girc.PRIVMSG, Params: []string{"#test", "+version"}}
	if b.Check(botLineCtx("botloop-command", false, 3, "+version"), ev) {
		t.Fatal("a bot line must not trigger a command")
	}
	human := mocktest.NewMockContext().WithAddressed(false).WithArgs("+version")
	if !b.Check(human, ev) {
		t.Fatal("test setup: the same command from a human should run")
	}
}

func TestBotLineZeroLimit(t *testing.T) {
	b := addressedWithVersion()
	ev := &girc.Event{Command: girc.PRIVMSG, Params: []string{"#test", "[otherbot] hey testbot"}}
	if b.Check(botLineCtx("botloop-zero", true, 0, "[otherbot]", "testbot"), ev) {
		t.Fatal("botreplylimit 0 means never reply to bots")
	}
}

func TestBotLineSkipsNonAddressedAndURLWatcher(t *testing.T) {
	ev := &girc.Event{Command: girc.PRIVMSG, Params: []string{"#test", "https://example.com"}}

	ctx := botLineCtx("botloop-other", false, 3, "https://example.com").WithURLWatcher(true)
	if (&URLBehavior{}).Check(ctx, ev) {
		t.Error("the URL watcher must not answer a bot's link")
	}

	ctx = botLineCtx("botloop-other", false, 3, "anything")
	ctx.GetConfig().Bot.Addressed = false
	if (&NonAddressedBehavior{}).Check(ctx, ev) {
		t.Error("non-addressed mode must not answer bot lines")
	}
}
