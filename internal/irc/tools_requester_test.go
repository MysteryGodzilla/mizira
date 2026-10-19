// Copyright (C) 2026 BareMetal
// Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alexschlessinger/pollytool/tools"

	mocktest "B4reMetal/metald/internal/testing"
)

type argsRecorder struct {
	tools.Tool
	got map[string]any
}

func (a *argsRecorder) Execute(_ context.Context, args map[string]any) (string, error) {
	a.got = args
	return "ok", nil
}

func TestRequesterToolPassesTheSpeaker(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
		args map[string]any
		want any
	}{
		{"speaker added", InjectContext(context.Background(), mocktest.NewMockContext().WithSource("alice")),
			map[string]any{"style": "jazz"}, "alice"},
		{"model's value replaced", InjectContext(context.Background(), mocktest.NewMockContext().WithSource("alice")),
			map[string]any{"requested_by": "mallory"}, "alice"},
		{"no irc context drops it", context.Background(),
			map[string]any{"requested_by": "mallory"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &argsRecorder{}
			if _, err := NewRequesterTool(rec).Execute(tc.ctx, tc.args); err != nil {
				t.Fatal(err)
			}
			if got := rec.got["requested_by"]; got != tc.want {
				t.Fatalf("requested_by = %v, want %v", got, tc.want)
			}
			if _, ok := tc.args["requested_by"]; ok && tc.args["requested_by"] != "mallory" {
				t.Fatal("the caller's args map was modified")
			}
		})
	}
}

// A refusal must not read as a spent limit: the model told the channel a limit "reset failed".
func TestAdminOnlyToolRefusalSaysWhy(t *testing.T) {
	ctx := InjectContext(context.Background(), mocktest.NewMockContext().WithSource("alice"))
	rec := &argsRecorder{}
	got, err := NewAdminOnlyTool(rec).Execute(ctx, map[string]any{})
	if err != nil || rec.got != nil {
		t.Fatalf("tool ran for a non-admin: %v %v", rec.got, err)
	}
	for _, want := range []string{"admins only", "alice", "isn't a usage limit"} {
		if !strings.Contains(got, want) {
			t.Errorf("refusal %q lacks %q", got, want)
		}
	}
}

// A person could ask another bot to ask, and get round per-person limits.
func TestPeopleOnlyToolRefusesBots(t *testing.T) {
	bot := mocktest.NewMockContext().WithSource("bob")
	bot.BotLine, bot.BotTag = true, "[botty]"
	rec := &argsRecorder{}
	got, err := NewPeopleOnlyTool(rec).Execute(InjectContext(context.Background(), bot), map[string]any{})
	if err != nil || rec.got != nil || !strings.Contains(got, "not for other bots (botty)") {
		t.Fatalf("bot line: ran=%v reply=%q err=%v", rec.got != nil, got, err)
	}
	person := mocktest.NewMockContext().WithSource("alice")
	if _, err := NewPeopleOnlyTool(rec).Execute(InjectContext(context.Background(), person), map[string]any{}); err != nil || rec.got == nil {
		t.Fatalf("a person's call didn't run: %v", err)
	}
}

func TestRefusedForLooksThroughEveryWrapper(t *testing.T) {
	tool := NewAdminOnlyTool(NewPeopleOnlyTool(&argsRecorder{}))
	admin := mocktest.NewMockContext().WithAdmin(true)
	adminBot := mocktest.NewMockContext().WithAdmin(true)
	adminBot.BotLine = true
	for name, tc := range map[string]struct {
		ctx  *mocktest.MockChatContext
		want bool
	}{"admin": {admin, false}, "non-admin": {mocktest.NewMockContext(), true}, "admin's bot": {adminBot, true}} {
		if got := RefusedFor(tool, tc.ctx); got != tc.want {
			t.Errorf("%s: RefusedFor = %v, want %v", name, got, tc.want)
		}
	}
}

// The model drew a second picture in the same turn and posted both; after a refusal it kept
// retrying. A repeat is answered with what the first call got.
func TestOncePerRequestTool(t *testing.T) {
	rec := &argsRecorder{}
	once := NewOncePerRequestTool(rec)
	ctx := InjectContext(context.Background(), mocktest.NewMockContext())
	if out, _ := once.Execute(ctx, map[string]any{}); out != "ok" {
		t.Fatalf("first call = %q", out)
	}
	out, _ := once.Execute(ctx, map[string]any{})
	if !strings.HasPrefix(out, OnceRepeated) || !strings.Contains(out, `"ok"`) {
		t.Fatalf("second call in the same request = %q", out)
	}
	now := time.Now()
	if _, again := once.first("another-request", now); again {
		t.Fatal("a new request was refused")
	}
	if _, again := once.first("mock-request", now.Add(2*time.Hour)); again {
		t.Fatal("old requests aren't forgotten")
	}
}
