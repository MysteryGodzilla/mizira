// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"testing"
	"time"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

func TestPresenceBlock(t *testing.T) {
	mockSys := mocktest.NewMockSystem()
	session, _ := mockSys.SessionStore.Get("presence/#chat")
	ctx := mocktest.NewMockContext().WithSystem(mockSys).WithSession(session).WithSource("alice")
	channel := ctx.GetConfig().Server.Channel
	ctx.ChannelUsers = map[string][]core.ChannelUser{channel: {{Nick: "alice"}, {Nick: "Carol"}, {Nick: "bob"}, {Nick: ctx.GetBotNick()}}}
	db, err := core.Context()
	if err != nil {
		t.Fatal(err)
	}
	db.LogLine(session.GetName(), "alice", "hi", time.Now().Add(-5*time.Minute))
	db.LogLine(session.GetName(), "bob", "old news", time.Now().Add(-3*time.Hour))
	db.LogLine(session.GetName(), "mallory", "gone now", time.Now().Add(-time.Minute))

	none := func(string) bool { return false }
	want := "in the channel now:\n  talking lately: alice · here but quiet: bob, Carol"
	if got := presenceBlock(ctx, none); got != want {
		t.Errorf("presence =\n%q\nwant\n%q", got, want)
	}
	if got := presenceBlock(ctx, func(string) bool { return true }); got != "" {
		t.Errorf("sent again unchanged: %q", got)
	}
	ctx.ChannelUsers = map[string][]core.ChannelUser{channel: {{Nick: ctx.GetBotNick()}}}
	if got := presenceBlock(ctx, none); got != "" {
		t.Errorf("alone = %q", got)
	}
}
