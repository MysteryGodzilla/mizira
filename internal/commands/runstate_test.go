// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"context"
	"strings"
	"testing"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

func runStateCtx(t *testing.T) *mocktest.MockChatContext {
	t.Helper()
	useTempOverrides(t)
	core.SetState(core.Running)
	t.Cleanup(func() { core.SetState(core.Running) })
	return mocktest.NewMockContext().WithAdmin(true).WithSource("admin")
}

func run(ctx *mocktest.MockChatContext, cmd Command) string {
	ctx.Replies = nil
	cmd.Execute(ctx)
	return strings.Join(ctx.Replies, "\n")
}

func TestPauseResume(t *testing.T) {
	ctx := runStateCtx(t)
	if out := run(ctx, &PauseCommand{}); !strings.HasPrefix(out, "Paused") || core.State() != core.Paused {
		t.Fatalf("pause: %q, state %v", out, core.State())
	}
	if out := run(ctx, &PauseCommand{}); !strings.Contains(out, "Already paused") {
		t.Fatalf("second pause: %q", out)
	}
	if out := run(ctx, &ResumeCommand{}); out != "Resumed." || core.State() != core.Running {
		t.Fatalf("resume: %q, state %v", out, core.State())
	}
	if out := run(ctx, &ResumeCommand{}); out != "Not paused." {
		t.Fatalf("second resume: %q", out)
	}
}

// Pausing a stopped bot must not quietly turn the emergency stop into a soft one.
func TestPauseDoesNotWeakenStop(t *testing.T) {
	ctx := runStateCtx(t)
	run(ctx, &StopCommand{})
	if out := run(ctx, &PauseCommand{}); !strings.Contains(out, "Already stopped") || core.State() != core.Stopped {
		t.Fatalf("pause after stop: %q, state %v", out, core.State())
	}
}

func TestStopCancelsEverythingElse(t *testing.T) {
	ctx := runStateCtx(t)
	inflight, cancel := context.WithCancel(context.Background())
	defer core.Requests().Track("stoptest/#chat", "req-busy", "alice", cancel)()
	selfCtx, selfCancel := context.WithCancel(context.Background())
	defer core.Requests().Track("stoptest/#chat", "mock-request", "admin", selfCancel)()

	out := run(ctx, &StopCommand{})
	if core.State() != core.Stopped {
		t.Fatalf("state = %v, want stopped", core.State())
	}
	if inflight.Err() == nil {
		t.Fatal("+stop must cancel requests already running")
	}
	if selfCtx.Err() != nil {
		t.Fatal("+stop must not cancel itself")
	}
	if !strings.HasPrefix(out, "Stopped.") {
		t.Fatalf("reply: %q", out)
	}
}

func TestRunStateSurvivesRestart(t *testing.T) {
	for _, cmd := range []Command{&PauseCommand{}, &StopCommand{}} {
		ctx := runStateCtx(t)
		run(ctx, cmd)
		want := core.State()

		core.SetState(core.Running) // simulate a fresh process
		ApplyOverrides(mocktest.NewMockContext().GetConfig())
		if core.State() != want {
			t.Errorf("after %s and a restart: state %v, want %v", cmd.Name(), core.State(), want)
		}

		run(ctx, &ResumeCommand{})
		core.SetState(core.Paused)
		ApplyOverrides(mocktest.NewMockContext().GetConfig())
		if core.State() != core.Paused {
			t.Errorf("a resumed bot must not save a halted state; restart changed state to %v", core.State())
		}
	}
}

func TestRunStateCommandsAreAdminOnly(t *testing.T) {
	for _, cmd := range []Command{&PauseCommand{}, &ResumeCommand{}, &StopCommand{}} {
		if !cmd.AdminOnly() {
			t.Errorf("%s must be admin-only", cmd.Name())
		}
	}
}
