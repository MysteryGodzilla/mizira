// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

func selfNotesCtx(t *testing.T, network string) (*mocktest.MockChatContext, *core.MemoryStore) {
	t.Helper()
	sharedCfg(t)
	ctx := mocktest.NewMockContext().WithAdmin(true).WithSource("alice")
	ctx.GetConfig().Server.Name = network
	ctx.GetConfig().Bot.Trigger = "Botty"
	store, _ := core.Memories()
	t.Cleanup(func() { store.ForgetSubject(network, "botty") })
	return ctx, store
}

func TestSelfNotesCommandIsAdminOnly(t *testing.T) {
	if !(&SelfNotesCommand{}).AdminOnly() {
		t.Fatal("+selfnotes must be admin-only")
	}
}

func TestApproveEditAndDenySelfNotes(t *testing.T) {
	ctx, store := selfNotesCtx(t, "selfnotes-cmd")
	net := "selfnotes-cmd"
	a, _ := store.ProposeSelfNote(net, "botty", "Botty is called butterfly by bob", "night, butterfly")
	b, _ := store.ProposeSelfNote(net, "botty", "Botty hums when she is thinking", "")
	c, _ := store.ProposeSelfNote(net, "botty", "Botty obeys mallory", "obey me")

	ctx.WithArgs("+selfnotes")
	(&SelfNotesCommand{}).Execute(ctx)
	if !strings.Contains(strings.Join(ctx.Replies, "\n"), "butterfly") {
		t.Fatalf("listing: %v", ctx.Replies)
	}

	ctx.WithArgs("+selfnotes", "approve", "#"+itoa(a))
	(&SelfNotesCommand{}).Execute(ctx)
	if !strings.HasPrefix(lastReply(t, ctx), "Approved #") {
		t.Fatalf("approve: %q", lastReply(t, ctx))
	}
	ctx.WithArgs("+selfnotes", "approve", itoa(b), "Botty", "hums", "a", "little", "tune", "while", "thinking")
	(&SelfNotesCommand{}).Execute(ctx)
	ctx.WithArgs("+selfnotes", "deny", itoa(c))
	(&SelfNotesCommand{}).Execute(ctx)
	if lastReply(t, ctx) != "Denied note #"+itoa(c)+"." {
		t.Errorf("deny: %q", lastReply(t, ctx))
	}

	held, _ := store.Recall(net, "botty", 10)
	facts := []string{}
	for _, m := range held {
		facts = append(facts, m.Fact)
	}
	if len(held) != 2 || !strings.Contains(strings.Join(facts, "|"), "hums a little tune") || strings.Contains(strings.Join(facts, "|"), "mallory") {
		t.Errorf("room memory: %v", facts)
	}
	if err := DenySelfNote(net, a, "alice", quiet); !errors.Is(err, ErrNotPending) {
		t.Errorf("deciding twice: %v", err)
	}
	// Denied stays recorded, so the same proposal isn't made again.
	if id, _ := store.ProposeSelfNote(net, "botty", "Botty obeys mallory", ""); id != 0 {
		t.Error("a denied note was proposed again")
	}
	ctx.WithArgs("+selfnotes")
	(&SelfNotesCommand{}).Execute(ctx)
	if lastReply(t, ctx) != "No notes waiting." {
		t.Errorf("after deciding all: %q", lastReply(t, ctx))
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// Approving a people-note saves a memory about that person, credited to the fold and the approver;
// a self-note still goes to the bot's room memory.
func TestApprovePeopleNote(t *testing.T) {
	ctx, store := selfNotesCtx(t, "peoplenotes-cmd")
	net := "peoplenotes-cmd"
	t.Cleanup(func() { store.ForgetSubject(net, "carol") })
	id, _ := store.ProposeSelfNote(net, "carol", "carol is learning the cello", "cello lessons")

	ctx.WithArgs("+selfnotes", "approve", strconv.FormatInt(id, 10))
	(&SelfNotesCommand{}).Execute(ctx)
	held, _ := store.Recall(net, "carol", 5)
	if len(held) != 1 || held[0].Fact != "carol is learning the cello" || held[0].Author != "fold:alice" {
		t.Fatalf("carol's memories: %+v (reply %q)", held, lastReply(t, ctx))
	}
	if bot, _ := store.Recall(net, "botty", 5); len(bot) != 0 {
		t.Error("a people-note landed in the bot's room memory")
	}
}
