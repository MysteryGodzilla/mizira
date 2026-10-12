// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"strings"
	"testing"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

// A speaker's profile rides with their memories, and naming a bot brings up who runs it, even
// with nothing remembered about either.
func TestProfilesRideWithMemories(t *testing.T) {
	ctx := mocktest.NewMockContext().WithSource("bob")
	net := ctx.GetNetwork()
	mem, _ := core.Memories()
	id, _ := mem.Remember(net, "bob", "bob wears a long blue dress", "bob", "#chat")
	mem.SetKind(net, id, core.KindLook)
	mem.SetProfile(net, core.Profile{Subject: "bob", Pronouns: "he/him", AvatarPronouns: "she/her"})
	mem.SetProfile(net, core.Profile{Subject: "otherbot", RunBy: "carol"})
	t.Cleanup(func() {
		mem.ForgetSubject(net, "bob")
		mem.DeleteProfile(net, "bob")
		mem.DeleteProfile(net, "otherbot")
	})
	none := func(string) bool { return false }

	got := recallForSpeaker(ctx, none)
	for _, want := range []string{"bob: he/him; avatar she/her", "[look] bob's avatar (she/her): bob wears a long blue dress"} {
		if !strings.Contains(got, want) {
			t.Errorf("speaker block lacks %q:\n%s", want, got)
		}
	}
	if got := recallForSpeaker(ctx, func(s string) bool { return true }); got != "" {
		t.Errorf("sent again what the conversation carries: %q", got)
	}
	if got := relevantMemories(ctx, "is otherbot around?", none); !strings.Contains(got, "otherbot: a bot run by carol") {
		t.Errorf("bot link missing: %q", got)
	}
}

// A speaker's boundary goes first and fresh lore is sent (fading is tested in core).
func TestSpeakerMemoriesByKind(t *testing.T) {
	ctx := mocktest.NewMockContext().WithSource("dave")
	net := ctx.GetNetwork()
	mem, _ := core.Memories()
	like, _ := mem.Remember(net, "dave", "dave likes sourdough", "dave", "#chat")
	old, _ := mem.Remember(net, "dave", "dave once fell into the duck pond", "dave", "#chat")
	ask, _ := mem.Remember(net, "dave", "dave doesn't want to be called davey", "dave", "#chat")
	mem.SetKind(net, like, core.KindLikes)
	mem.SetKind(net, old, core.KindLore)
	mem.SetKind(net, ask, core.KindBoundary)
	t.Cleanup(func() { mem.ForgetSubject(net, "dave") })
	ctx.GetConfig().Bot.LoreFadeDays = 45

	got := recallForSpeaker(ctx, func(string) bool { return false })
	if !strings.Contains(got, "duck pond") || strings.Index(got, "davey") > strings.Index(got, "sourdough") {
		t.Errorf("fresh lore missing or boundary not first:\n%s", got)
	}
}
