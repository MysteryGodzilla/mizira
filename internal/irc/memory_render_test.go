// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"path/filepath"
	"testing"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

func testPeople(t *testing.T) core.People {
	t.Helper()
	s, err := core.OpenMemoryStore(filepath.Join(t.TempDir(), "memories.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, p := range []core.Profile{
		{Subject: "bob", Pronouns: "he/him", AvatarPronouns: "she/her", Aliases: []string{"bobby"}},
		{Subject: "botty", RunBy: "bob", Pronouns: "she/her"},
		{Subject: "otherbot", RunBy: "carol"},
	} {
		if err := s.SetProfile("net", p); err != nil {
			t.Fatal(err)
		}
	}
	people, err := s.People("net")
	if err != nil {
		t.Fatal(err)
	}
	return people
}

// A person whose avatar is another gender: a look is the avatar's, with its pronouns; anything else
// is the person's.
func TestMemoryLine(t *testing.T) {
	people := testPeople(t)
	look := core.Memory{Subject: "bob", Fact: "bob wears a long blue dress", Author: "bob"}
	joke := core.Memory{Subject: "bob", Fact: "bob jokes about trains", Author: "alice"}
	for _, c := range []struct {
		m    core.Memory
		meta core.MemoryMeta
		want string
	}{
		{look, core.MemoryMeta{Kind: core.KindLook}, "[look] bob's avatar (she/her): bob wears a long blue dress"},
		{look, core.MemoryMeta{}, "bob wears a long blue dress"},
		{joke, core.MemoryMeta{Kind: core.KindLore}, "[lore] bob jokes about trains (per alice)"},
		{joke, core.MemoryMeta{Also: []string{"carol", "dave"}}, "bob jokes about trains (also about carol, dave; per alice)"},
		{core.Memory{Subject: "carol", Fact: "carol has red hair", Author: "fold:console:token"},
			core.MemoryMeta{Kind: core.KindLook}, "[look] carol has red hair (guessed from chat)"},
		{core.Memory{Subject: "carol", Fact: "carol likes tea", Author: "console:token"}, core.MemoryMeta{}, "carol likes tea"},
	} {
		if got := MemoryLine(c.m, c.meta, people); got != c.want {
			t.Errorf("MemoryLine = %q\n            want %q", got, c.want)
		}
	}
}

// A bot and the person who runs it each say who the other is.
func TestProfileLine(t *testing.T) {
	people := testPeople(t)
	for subject, want := range map[string]string{
		"bob":      "bob: he/him; avatar she/her; also called bobby; runs the bot botty",
		"botty":    "botty: a bot run by bob; she/her",
		"carol":    "carol: runs the bot otherbot",
		"otherbot": "otherbot: a bot run by carol",
		"dave":     "",
	} {
		if got := ProfileLine(people, subject); got != want {
			t.Errorf("ProfileLine(%s) = %q, want %q", subject, got, want)
		}
	}
	if got := ProfileLine(core.People{}, "bob"); got != "" {
		t.Errorf("no profiles = %q", got)
	}
}

func TestRememberAsFilesKindAndOthers(t *testing.T) {
	mock := mocktest.NewMockContext().WithSource("alice").WithAdmin(true)
	cfg := mock.GetConfig()
	cfg.Bot.Trigger = "botty"
	cfg.Server.Name = "remember-as"
	store, _ := core.Memories()
	t.Cleanup(func() { store.Clear("remember-as") })

	res := RememberAs(mock, "botty", "botty and dave share a desk", core.KindLore,
		SplitNames("Dave, "+cfg.Server.Channel+" and botty"))
	if !res.Saved {
		t.Fatalf("not saved: %+v", res)
	}
	meta, _ := store.Meta("remember-as", []int64{res.ID})
	if meta[res.ID].Kind != core.KindLore || len(meta[res.ID].Also) != 1 || meta[res.ID].Also[0] != "dave" {
		t.Errorf("meta = %+v", meta[res.ID])
	}
	if got, _ := store.Recall("remember-as", "dave", 5); len(got) != 1 {
		t.Errorf("dave's memories = %+v", got)
	}
	// Someone already known, named in the fact, is filed too without being listed; an unknown name isn't.
	store.Remember("remember-as", "eve", "eve paints", "eve", "#test")
	res = RememberAs(mock, "botty", "botty paints with Eve and zed", "", nil)
	if meta, _ := store.Meta("remember-as", []int64{res.ID}); len(meta[res.ID].Also) != 1 || meta[res.ID].Also[0] != "eve" {
		t.Errorf("named people = %+v", meta[res.ID])
	}
	if res := RememberAs(mock, "botty", "botty hums", "weather", nil); !res.Saved {
		t.Errorf("an unknown kind stopped the save: %+v", res)
	} else if meta, _ := store.Meta("remember-as", []int64{res.ID}); meta[res.ID].Kind != "" {
		t.Errorf("unknown kind kept: %+v", meta)
	}
}

func TestSplitNames(t *testing.T) {
	got := SplitNames("bob, carol and dave; eve & mallory")
	want := []string{"bob", "carol", "dave", "eve", "mallory"}
	if len(got) != len(want) {
		t.Fatalf("SplitNames = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("SplitNames = %v", got)
		}
	}
	if len(SplitNames("  ")) != 0 {
		t.Error("blank gave names")
	}
}
