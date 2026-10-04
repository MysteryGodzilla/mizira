// Copyright (C) 2026 alice
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"strings"
	"testing"
	"time"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

func TestDescribeIgnore(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		entry core.IgnoreEntry
		want  string
	}{
		{core.IgnoreEntry{Nick: "bob", Expiry: now.Add(42 * time.Minute),
			IgnoreInfo: core.IgnoreInfo{Kind: core.IgnoreByBot, By: "alice", Reason: "kept spamming links"}},
			`bob: 42m left · bot, during alice's message · "kept spamming links"`},
		{core.IgnoreEntry{Nick: "carol", Expiry: now.Add(8 * time.Minute),
			IgnoreInfo: core.IgnoreInfo{Kind: core.IgnoreByFlood, Reason: "6 messages in 30s"}},
			`carol: 8m left · flood · 6 messages in 30s`},
		{core.IgnoreEntry{Nick: "dave", Expiry: now.Add(2*time.Hour + 5*time.Minute),
			IgnoreInfo: core.IgnoreInfo{Kind: core.IgnoreByAdmin, By: "alice", Reason: "testing"}},
			`dave: 2h05m left · admin alice · "testing"`},
		{core.IgnoreEntry{Nick: "eve", Expiry: now.Add(20 * time.Second)}, // old entry, no details
			`eve: under a minute left`},
	}
	for _, tt := range tests {
		if got := describeIgnore(tt.entry, now); got != tt.want {
			t.Errorf("got  %s\nwant %s", got, tt.want)
		}
	}
}

func ignoreCtx(t *testing.T, network string) *mocktest.MockChatContext {
	t.Helper()
	ctx := mocktest.NewMockContext().WithAdmin(true).WithSource("alice")
	ctx.GetConfig().Server.Name = network
	ctx.GetConfig().Bot.Admins = []string{"alice!*@*"}
	t.Cleanup(func() {
		for _, e := range core.Ignores().List(network) {
			core.Ignores().Remove(network, e.Nick)
		}
	})
	return ctx
}

func runIgnore(ctx *mocktest.MockChatContext, args ...string) string {
	ctx.WithArgs(append([]string{"+ignore"}, args...)...)
	ctx.Replies = nil
	(&IgnoreCommand{}).Execute(ctx)
	return strings.Join(ctx.Replies, "\n")
}

func TestIgnoreCommandReasons(t *testing.T) {
	ctx := ignoreCtx(t, "ignore-reasons")

	runIgnore(ctx, "bob", "30m", "being", "rude")
	runIgnore(ctx, "carol", "spamming", "links") // no duration: default, all of it is the reason
	if out := runIgnore(ctx, "dave", "3hh"); !strings.Contains(out, "Invalid duration") {
		t.Fatalf("a mistyped duration must be an error, not a reason: %q", out)
	}

	byNick := map[string]core.IgnoreEntry{}
	for _, e := range core.Ignores().List("ignore-reasons") {
		byNick[e.Nick] = e
	}
	if e := byNick["bob"]; e.Reason != "being rude" || e.Kind != core.IgnoreByAdmin || e.By != "alice" {
		t.Errorf("bob = %+v", e)
	}
	if e := byNick["carol"]; e.Reason != "spamming links" || time.Until(e.Expiry) < 59*time.Minute {
		t.Errorf("carol = %+v (want the default hour)", e)
	}
	if _, ok := byNick["dave"]; ok {
		t.Error("dave must not be ignored after an invalid duration")
	}
}

// The list must not flood the channel itself.
func TestIgnoreListIsCapped(t *testing.T) {
	ctx := ignoreCtx(t, "ignore-cap")
	for _, nick := range []string{"n1", "n2", "n3", "n4", "n5", "n6", "n7"} {
		core.Ignores().AddWithInfo("ignore-cap", nick, time.Hour, core.IgnoreInfo{Kind: core.IgnoreByFlood})
	}
	out := runIgnore(ctx, "list")
	lines := strings.Split(out, "\n")
	if len(lines) != maxIgnoreListLines+1 || lines[len(lines)-1] != "...and 2 more" {
		t.Fatalf("list:\n%s", out)
	}
}
