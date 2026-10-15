// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"slices"
	"testing"
)

func TestProfiles(t *testing.T) {
	s := testStore(t)
	if err := s.SetProfile("net", Profile{Subject: "Bob", Pronouns: "he/him", AvatarPronouns: "she/her",
		Aliases: []string{"Bobby", "bob"}, By: "console:token"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetProfile("net", Profile{Subject: "botty", RunBy: "bob", Pronouns: "she/her"}); err != nil {
		t.Fatal(err)
	}
	people, err := s.People("net")
	if err != nil {
		t.Fatal(err)
	}
	bob, ok := people.Get("BOB")
	if !ok || bob.AvatarPronouns != "she/her" || !slices.Equal(bob.Aliases, []string{"bobby"}) || bob.Bot {
		t.Errorf("bob = %+v", bob)
	}
	if botty, _ := people.Get("botty"); !botty.Bot {
		t.Error("a bot run by someone isn't a bot")
	}
	if people.Resolve("Bobby") != "bob" || people.Resolve("carol") != "carol" {
		t.Error("resolve")
	}
	if !slices.Equal(people.Bots("bob"), []string{"botty"}) {
		t.Errorf("bob's bots = %v", people.Bots("bob"))
	}

	// Same avatar pronouns as the person's say nothing extra.
	s.SetProfile("net", Profile{Subject: "carol", Pronouns: "she/her", AvatarPronouns: "she/her"})
	if people, _ := s.People("net"); people.bySubject["carol"].AvatarPronouns != "" {
		t.Error("kept avatar pronouns equal to the person's")
	}

	for name, p := range map[string]Profile{
		"two words":          {Subject: "bob smith"},
		"empty":              {Subject: ""},
		"runs itself":        {Subject: "botty", RunBy: "botty"},
		"long pronouns":      {Subject: "dave", Pronouns: "a very long set of pronouns indeed"},
		"alias taken":        {Subject: "dave", Aliases: []string{"bobby"}},
		"alias is a subject": {Subject: "dave", Aliases: []string{"carol"}},
		"alias with a space": {Subject: "dave", Aliases: []string{"big dave"}},
		"markup":             {Subject: "dave", Pronouns: "{nick}"},
	} {
		if err := s.SetProfile("net", p); err == nil {
			t.Errorf("%s: accepted %+v", name, p)
		}
	}
	if list, _ := s.Profiles("other"); len(list) != 0 {
		t.Errorf("another network sees %+v", list)
	}
	if ok, _ := s.DeleteProfile("net", "BOB"); !ok {
		t.Error("delete")
	}
	if ok, _ := s.DeleteProfile("net", "bob"); ok {
		t.Error("deleted twice")
	}
}

func TestMentionedFindsAliasesAndProfiles(t *testing.T) {
	s := testStore(t)
	s.Remember("net", "bob", "bob plays chess", "bob", "#chat")
	s.SetProfile("net", Profile{Subject: "bob", Aliases: []string{"bobby"}})
	s.SetProfile("net", Profile{Subject: "botty", RunBy: "bob"})
	got, err := s.Mentioned("net", "ask bobby and [botty] about it", nil)
	if err != nil || !slices.Equal(got, []string{"bob", "botty"}) {
		t.Errorf("mentioned = %v %v", got, err)
	}
	if got, _ := s.Mentioned("net", "bobby said hi", []string{"bob"}); len(got) != 0 {
		t.Errorf("an excluded subject by its alias = %v", got)
	}
	if mems, _ := s.Relevant("net", "what does bobby play", nil, 5); len(mems) != 1 {
		t.Errorf("relevant by alias = %+v", mems)
	}
}

func TestTimezones(t *testing.T) {
	s := testStore(t)
	for tz, ok := range map[string]bool{"Australia/Brisbane": true, "America/New_York": true, "UTC": true,
		"": false, "Local": false, "Mars/Olympus": false, "brisbane": false} {
		if ValidTimezone(tz) != ok {
			t.Errorf("ValidTimezone(%q) = %v", tz, !ok)
		}
	}
	s.SetProfile("net", Profile{Subject: "bob", Pronouns: "he/him"})
	if err := s.SetTimezone("net", "bob", "Australia/Brisbane", "bob"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTimezone("net", "carol", "America/New_York", "carol"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetTimezone("net", "dave", "Mars/Olympus", "dave"); err == nil {
		t.Error("a made-up zone was saved")
	}
	people, _ := s.People("net")
	if bob, _ := people.Get("bob"); bob.Pronouns != "he/him" || bob.Timezone != "Australia/Brisbane" {
		t.Errorf("bob = %+v", bob)
	}
	if loc, ok := people.Location("carol"); !ok || loc.String() != "America/New_York" {
		t.Errorf("carol's location = %v %v", loc, ok)
	}
	if _, ok := people.Location("dave"); ok {
		t.Error("dave has a location")
	}
}
