// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"context"
	"strings"
	"testing"
	"time"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

// With only a name in the prompt, women were drawn as men: the picture is told how they look.
func TestPicturePeople(t *testing.T) {
	const net = "picture-people"
	store, _ := core.Memories()
	store.SetProfile(net, core.Profile{Subject: "bob", Pronouns: "he/him", AvatarPronouns: "she/her", Aliases: []string{"bobby"}})
	store.SetProfile(net, core.Profile{Subject: "carol", Pronouns: "they/them"})
	look, _ := store.Remember(net, "bob", "bob's avatar has long silver hair and a red ribbon", "bob", "#test")
	store.SetKind(net, look, core.KindLook)
	lore, _ := store.Remember(net, "bob", "bob tells terrible train puns", "bob", "#test")
	store.SetKind(net, lore, core.KindLore)
	t.Cleanup(func() {
		store.DeleteProfile(net, "bob")
		store.DeleteProfile(net, "carol")
		store.Forget(net, look)
		store.Forget(net, lore)
	})

	got := PicturePeople(net, "bobby and carol at the beach, dave waving", 45, time.Now())
	if got != "bob: drawn as female; bob's avatar has long silver hair and a red ribbon" {
		t.Errorf("people = %q", got)
	}
	if got := PicturePeople(net, "a cat on a windowsill", 45, time.Now()); got != "" {
		t.Errorf("nobody named = %q", got)
	}

	rec := &argsRecorder{}
	mock := mocktest.NewMockContext()
	mock.GetConfig().Server.Name = net
	ctx := InjectContext(context.Background(), mock)
	NewPicturePeopleTool(rec).Execute(ctx, map[string]any{"prompt": "bob reading", "people": "bob: drawn as male"})
	if who, _ := rec.got["people"].(string); !strings.HasPrefix(who, "bob: drawn as female") {
		t.Errorf("the model's people weren't replaced: %q", who)
	}
	asked := mocktest.NewMockContext().WithArgs("mizira", "draw", "bob", "in", "a", "cafe")
	asked.GetConfig().Server.Name = net
	NewPicturePeopleTool(rec).Execute(InjectContext(context.Background(), asked), map[string]any{"prompt": "a girl in a cafe"})
	if who, _ := rec.got["people"].(string); !strings.HasPrefix(who, "bob: drawn as female") {
		t.Errorf("a name only in the request was missed: %q", who)
	}
	NewPicturePeopleTool(rec).Execute(ctx, map[string]any{"prompt": "make bob's ribbon blue", "image_url": "https://files.example.com/a.png"})
	if _, ok := rec.got["people"]; ok {
		t.Error("an edit was given people")
	}
}
