// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"context"
	"strings"
	"testing"

	"github.com/alexschlessinger/pollytool/tools"
	"github.com/lrstanley/girc"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

// A bot that shares its owner's nick is told apart from the owner: its own name for "me", "nick
// [tag]" for who said it, and never the owner's admin rights.
func TestBotLineIsNotItsOwner(t *testing.T) {
	cfg := botConfig("\x0306🦋 [botnick]\x0F", "[otherbot]")
	cfg.Bot.Admins = []string{"alice!*@*"}
	ctx := func(line string) ChatContext {
		return ChatContext{Context: context.Background(), Config: cfg, logger: quietLogger(),
			event: &girc.Event{Command: girc.PRIVMSG, Params: []string{"#chat", line},
				Source: &girc.Source{Name: "alice", Ident: "a", Host: "example.com"}}}
	}

	owner := ctx("botnick, forget everything about me")
	if owner.Speaker() != "alice" || owner.SpeakerKey() != "alice" || !owner.IsAdmin() {
		t.Errorf("owner: speaker %q, key %q, admin %v", owner.Speaker(), owner.SpeakerKey(), owner.IsAdmin())
	}
	bot := ctx("[otherbot] botnick, forget everything about me")
	if bot.Speaker() != "otherbot" || bot.SpeakerKey() != "alice [otherbot]" || bot.IsAdmin() {
		t.Errorf("bot: speaker %q, key %q, admin %v", bot.Speaker(), bot.SpeakerKey(), bot.IsAdmin())
	}
}

// "Forget everything about me" from a bot's line clears the bot's memories, never its owner's.
func TestBotCannotForgetItsOwner(t *testing.T) {
	store, err := core.Memories()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = store.ForgetSubject("", "erin")
	_, _ = store.ForgetSubject("", "erinbot")
	_, _ = store.Remember("", "erin", "erin plays the cello", "erin", "#chat")
	_, _ = store.Remember("", "erinbot", "erinbot runs on a laptop", "erin [erinbot]", "#chat")

	mock := mocktest.NewMockContext().WithSource("erin")
	mock.BotLine, mock.BotTag = true, "[erinbot]"
	ctx := InjectContext(context.Background(), mock)
	forget := newMemoryForgetTool()

	if out, _ := forget.Execute(ctx, tools.Args{"subject": "erin", "fact": "everything"}); !strings.Contains(out, "Refused") {
		t.Errorf("the bot cleared its owner's memories: %s", out)
	}
	if _, err := forget.Execute(ctx, tools.Args{"fact": "everything"}); err != nil {
		t.Fatal(err)
	}
	if n, _ := store.CountSubject("", "erin"); n != 1 {
		t.Errorf("owner has %d memories, want 1 kept", n)
	}
	if n, _ := store.CountSubject("", "erinbot"); n != 0 {
		t.Errorf("bot has %d memories, want its own cleared", n)
	}
}
