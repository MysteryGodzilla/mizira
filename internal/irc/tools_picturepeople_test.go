// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

const bobFemale = `[{"name":"bob","aliases":["bobby"],"drawn_as":"female"`

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
	if got != bobFemale+`,"notes":["bob's avatar has long silver hair and a red ribbon"]}]` {
		t.Errorf("people = %q", got)
	}
	if got := PicturePeople(net, "a cat on a windowsill", 45, time.Now()); got != "" {
		t.Errorf("nobody named = %q", got)
	}

	rec := &argsRecorder{}
	mock := mocktest.NewMockContext()
	mock.GetConfig().Server.Name = net
	ctx := InjectContext(context.Background(), mock)
	NewPicturePeopleTool(rec).Execute(ctx, map[string]any{"prompt": "bob reading", "people": `[{"name":"bob","drawn_as":"male"}]`})
	if who, _ := rec.got["people"].(string); !strings.HasPrefix(who, bobFemale) {
		t.Errorf("the model's people weren't replaced: %q", who)
	}
	asked := mocktest.NewMockContext().WithArgs("mizira", "draw", "bob", "in", "a", "cafe")
	asked.GetConfig().Server.Name = net
	NewPicturePeopleTool(rec).Execute(InjectContext(context.Background(), asked), map[string]any{"prompt": "a girl in a cafe"})
	if who, _ := rec.got["people"].(string); !strings.HasPrefix(who, bobFemale) {
		t.Errorf("a name only in the request was missed: %q", who)
	}
	// Every request addressed her by name, so her look went into every picture. Live, her nick is the
	// owner's account and she answers to her trigger.
	asked.BotNick = "dave"
	asked.GetConfig().Bot.Trigger = "bob"
	asked.WithArgs("bob", "draw", "a", "dog", "next", "to", "a", "cat")
	NewPicturePeopleTool(rec).Execute(InjectContext(context.Background(), asked), map[string]any{"prompt": "a dog next to a cat"})
	if who, ok := rec.got["people"]; ok {
		t.Errorf("addressing the bot put it in the picture: %q", who)
	}
	for _, request := range [][]string{
		{"bob", "draw", "bob", "waving"},
		{"bob", "draw", "yourself", "waving"},
		{"draw", "yourself", "waving,", "bob"},
	} {
		asked.WithArgs(request...)
		NewPicturePeopleTool(rec).Execute(InjectContext(context.Background(), asked), map[string]any{"prompt": "a girl waving"})
		if who, _ := rec.got["people"].(string); !strings.HasPrefix(who, bobFemale) {
			t.Errorf("%q: the bot asked to be in the picture was missed: %q", request, who)
		}
	}
	// "Back in her default outfit" left the edit nothing to go on.
	NewPicturePeopleTool(rec).Execute(ctx, map[string]any{"prompt": "bob back in his usual outfit", "image_url": "https://files.example.com/a.png"})
	if who, _ := rec.got["people"].(string); !strings.HasPrefix(who, bobFemale) {
		t.Errorf("an edit wasn't given people: %q", who)
	}
}

func TestSpeakerNamed(t *testing.T) {
	for in, want := range map[string]string{
		"do me in yamamoto style":        "do alice in yamamoto style",
		"draw hori and me on a sofa":     "draw hori and alice on a sofa",
		"a picture of myself as a pilot": "a picture of alice as a pilot",
		"draw me a cat":                  "draw me a cat",
		"make one for me please":         "make one for me please",
		"send it to me":                  "send it to me",
		"a fox in the snow":              "a fox in the snow",
	} {
		if got := speakerNamed(in, "alice"); got != want {
			t.Errorf("speakerNamed(%q) = %q, want %q", in, got, want)
		}
	}
}

// The prompt writer described people afresh each time (glasses she doesn't wear, children for adults):
// a profile look goes in as set, and a variant named in the request picks its hair and outfit.
func TestPicturePeopleProfileLook(t *testing.T) {
	const net = "picture-looks"
	store, _ := core.Memories()
	store.SetProfile(net, core.Profile{Subject: "carol", Pronouns: "she/her"})
	mem, _ := store.Remember(net, "carol", "carol's avatar wears round glasses", "carol", "#test")
	store.SetKind(net, mem, core.KindLook)
	store.SetLook(net, "carol", core.Look{Appearance: "adult woman, green eyes", Hair: "short lavender bob", Outfit: "lilac cardigan",
		Variants: []core.LookVariant{{Name: "business", Outfit: "charcoal blazer"}}}, "")
	t.Cleanup(func() { store.DeleteProfile(net, "carol"); store.Forget(net, mem) })

	var got []PicturePerson
	if err := json.Unmarshal([]byte(PicturePeople(net, "carol in her business look at a desk", 45, time.Now())), &got); err != nil || len(got) != 1 {
		t.Fatalf("people = %v, %v", got, err)
	}
	want := PicturePerson{Name: "carol", DrawnAs: "female", Look: "adult woman, green eyes, short lavender bob", Outfit: "charcoal blazer", Variant: "business"}
	if got[0].Name != want.Name || got[0].Look != want.Look || got[0].Outfit != want.Outfit || got[0].Variant != want.Variant || len(got[0].Notes) != 0 {
		t.Errorf("carol = %+v, want %+v (no look memories)", got[0], want)
	}
}

// The model wrote someone's look memories into a picture prompt while their profile look was added
// too, and the picture had two of them: with a profile look, their look memories stay out of chat.
func TestWithoutSetLooks(t *testing.T) {
	const net = "picture-hide-looks"
	store, _ := core.Memories()
	store.SetLook(net, "carol", core.Look{Appearance: "adult woman", Hair: "a lavender bob", Outfit: "a white t-shirt",
		Variants: []core.LookVariant{{Name: "office", Outfit: "a blazer"}, {Name: "wedding", Outfit: "a white gown"}}}, "")
	look, _ := store.Remember(net, "carol", "carol's avatar has long silver hair", "carol", "#test")
	store.SetKind(net, look, core.KindLook)
	lore, _ := store.Remember(net, "carol", "carol runs the pub quiz", "carol", "#test")
	other, _ := store.Remember(net, "dave", "dave's avatar has a red cap", "dave", "#test")
	store.SetKind(net, other, core.KindLook)
	t.Cleanup(func() {
		store.DeleteProfile(net, "carol")
		store.Forget(net, look)
		store.Forget(net, lore)
		store.Forget(net, other)
	})

	mems := []core.Memory{{ID: look, Subject: "carol"}, {ID: lore, Subject: "carol"}, {ID: other, Subject: "dave"}}
	var kept []int64
	for _, m := range WithoutSetLooks(net, mems) {
		kept = append(kept, m.ID)
	}
	if len(kept) != 2 || kept[0] != lore || kept[1] != other {
		t.Errorf("kept %v, want carol's lore and dave's look (%d, %d)", kept, lore, other)
	}
	// Left with neither, she couldn't say what carol looks like and drew someone else as her.
	people, _ := store.People(net)
	want := "carol: looks like adult woman, a lavender bob (added to pictures for you: in a picture prompt write only their name); " +
		"usually wears a white t-shirt; alt looks (put the alt's name in a picture prompt to use it): office, wedding"
	if line := ProfileLine(people, "carol"); line != want {
		t.Errorf("profile line = %q", line)
	}
}

// "draw baremetal" came out in pc98 because the last picture was: a drawing style needs asking for.
func TestPictureStyleNeedsAsking(t *testing.T) {
	sha := strings.Repeat("a", 64)
	t.Setenv("IMAGE_STYLES", "pc98 | draw | 0.8 cfg 2.5 | pc98dither | PC-98 art | https://civitai.com/p | "+sha+"\n"+
		"clay | edit | 1 | Turn it into clay | clay figure | https://huggingface.co/c | "+sha)
	rec := &argsRecorder{}
	for _, c := range []struct {
		request []string
		style   string
		kept    bool
	}{
		{[]string{"mizira", "draw", "bob", "in", "a", "library"}, "pc98", false},
		{[]string{"mizira", "draw", "bob,", "PC-98", "style"}, "pc98", true},
		{[]string{"mizira", "pc98", "bob", "please"}, "pc98", true},
		{[]string{"mizira", "make", "it", "look", "like", "a", "sculpture"}, "clay", true},
	} {
		asked := mocktest.NewMockContext().WithArgs(c.request...)
		NewPicturePeopleTool(rec).Execute(InjectContext(context.Background(), asked), map[string]any{"prompt": "bob reading", "style": c.style})
		if _, kept := rec.got["style"]; kept != c.kept {
			t.Errorf("%q with style %s: kept = %v", c.request, c.style, kept)
		}
		if got := rec.got["request"]; got != strings.Join(c.request, " ") {
			t.Errorf("request passed on = %q", got)
		}
	}
}
