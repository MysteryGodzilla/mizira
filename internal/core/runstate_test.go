// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"context"
	"testing"
)

func TestRunStateRoundTrip(t *testing.T) {
	t.Cleanup(func() { SetState(Running) })
	for _, s := range []RunState{Running, Paused, Stopped} {
		got, ok := ParseRunState(s.String())
		if !ok || got != s {
			t.Errorf("ParseRunState(%q) = %v, %v", s.String(), got, ok)
		}
		SetState(s)
		if Halted() != (s != Running) {
			t.Errorf("Halted() wrong in state %v", s)
		}
	}
	// A hand-edited or corrupt saved value must not be read as a state.
	for _, bad := range []string{"", "Paused", "halt", "running "} {
		if _, ok := ParseRunState(bad); ok {
			t.Errorf("ParseRunState(%q) accepted", bad)
		}
	}
}

func TestCancelAllSparesTheCaller(t *testing.T) {
	f := &InFlight{byKey: make(map[string][]*entry)}
	ctxA, cancelA := context.WithCancel(context.Background())
	ctxB, cancelB := context.WithCancel(context.Background())
	ctxSelf, cancelSelf := context.WithCancel(context.Background())
	defer f.Track("net/#chat", "req-a", "alice", cancelA)()
	defer f.Track("net/bob", "req-b", "bob", cancelB)()
	defer f.Track("net/#chat", "req-stop", "admin", cancelSelf)()

	if n := f.CancelAll("req-stop"); n != 2 {
		t.Fatalf("cancelled %d, want 2", n)
	}
	if ctxA.Err() == nil || ctxB.Err() == nil {
		t.Error("every other request, in any conversation, must be cancelled")
	}
	if ctxSelf.Err() != nil {
		t.Error("the +stop request itself must survive to send its confirmation")
	}
}
