// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package behaviors

import (
	"testing"

	"github.com/lrstanley/girc"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

// While paused or stopped, only admin +commands get through: +resume must always work, and
// nothing else may start new work or reach the conversation.
func TestHaltedOnlyAdminCommandsGetThrough(t *testing.T) {
	b := addressedWithVersion()
	chat := &girc.Event{Command: girc.PRIVMSG, Params: []string{"#test", "testbot: hello"}}
	cmd := &girc.Event{Command: girc.PRIVMSG, Params: []string{"#test", "+version"}}

	for _, state := range []core.RunState{core.Paused, core.Stopped} {
		core.SetState(state)
		tests := []struct {
			name  string
			ctx   *mocktest.MockChatContext
			event *girc.Event
			want  bool
		}{
			{"admin command", mocktest.NewMockContext().WithAdmin(true).WithArgs("+version"), cmd, true},
			{"admin chat", mocktest.NewMockContext().WithAdmin(true).WithAddressed(true).WithArgs("testbot:", "hello"), chat, false},
			{"user command", mocktest.NewMockContext().WithArgs("+version"), cmd, false},
			{"user chat", mocktest.NewMockContext().WithAddressed(true).WithArgs("testbot:", "hello"), chat, false},
			{"private message", mocktest.NewMockContext().WithPrivate(true).WithArgs("hello"), chat, false},
			{"bot line", botLineCtx("halted-bot", true, 3, "[otherbot]", "testbot"), chat, false},
		}
		for _, tt := range tests {
			if got := b.Check(tt.ctx, tt.event); got != tt.want {
				t.Errorf("%v / %s: Check = %v, want %v", state, tt.name, got, tt.want)
			}
		}

		url := mocktest.NewMockContext().WithURLWatcher(true).WithAddressed(false).WithArgs("https://example.com")
		if (&URLBehavior{}).Check(url, &girc.Event{Command: girc.PRIVMSG, Params: []string{"#test", "https://example.com"}}) {
			t.Errorf("%v: URL watcher ran", state)
		}
		non := mocktest.NewMockContext().WithArgs("hello")
		non.GetConfig().Bot.Addressed = false
		if (&NonAddressedBehavior{}).Check(non, chat) {
			t.Errorf("%v: non-addressed reply ran", state)
		}
		join := mocktest.NewMockContext()
		join.GetConfig().Bot.Greeting = "wave"
		if (&JoinBehavior{}).Check(join, &girc.Event{Command: girc.JOIN, Source: &girc.Source{Name: join.GetBotNick()}, Params: []string{"#test"}}) {
			t.Errorf("%v: greeting ran", state)
		}
	}
	core.SetState(core.Running)

	// Sanity: the same messages work normally once resumed.
	if !b.Check(mocktest.NewMockContext().WithAddressed(true).WithArgs("testbot:", "hello"), chat) {
		t.Fatal("after resume, addressed chat must be answered again")
	}
}
