// Copyright (C) 2026 BareMetal
// Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"context"
	"strings"
	"testing"

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
