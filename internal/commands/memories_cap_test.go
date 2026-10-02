// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"fmt"
	"strings"
	"testing"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

func memoryCapCtx(t *testing.T, network string, facts int) *mocktest.MockChatContext {
	t.Helper()
	store, err := core.Memories()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.ForgetSubject(network, "jeff") })
	for i := range facts {
		if _, err := store.Remember(network, "jeff", fmt.Sprintf("fact %d", i), "alice", "#test"); err != nil {
			t.Fatal(err)
		}
	}
	ctx := mocktest.NewMockContext().WithSource("alice")
	ctx.GetConfig().Server.Name = network
	return ctx
}

// Anyone can list memories, and the count used to be theirs to choose: "+memories list 500"
// must not make the bot post 500 lines.
func TestMemoriesListIsCapped(t *testing.T) {
	ctx := memoryCapCtx(t, "memcap-list", 8)
	out := runCmd(ctx, &MemoriesCommand{}, "list", "500")
	if lines := strings.Split(out, "\n"); len(lines) != maxMemoryLines+1 {
		t.Fatalf("got %d lines, want header + %d:\n%s", len(lines), maxMemoryLines, out)
	}
	if !strings.HasPrefix(out, "8 memory(ies) stored, showing 5 most recent") {
		t.Fatalf("header = %q", strings.SplitN(out, "\n", 2)[0])
	}
	// A smaller number is still honoured.
	if lines := strings.Split(runCmd(ctx, &MemoriesCommand{}, "list", "2"), "\n"); len(lines) != 3 {
		t.Fatalf("list 2 gave %d lines", len(lines))
	}
}

func TestRecallIsCappedAndSaysHowMany(t *testing.T) {
	ctx := memoryCapCtx(t, "memcap-recall", 8)
	out := runCmd(ctx, &RecallCommand{}, "jeff")
	lines := strings.Split(out, "\n")
	if len(lines) != maxMemoryLines+1 || lines[0] != "8 memory(ies) about jeff, showing the 5 most recent:" {
		t.Fatalf("got:\n%s", out)
	}
}
