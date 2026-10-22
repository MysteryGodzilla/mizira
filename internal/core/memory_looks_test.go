// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"strings"
	"testing"
)

func TestLooks(t *testing.T) {
	s := testStore(t)
	look := Look{
		Appearance: " adult woman in her mid-20s,  slim, green eyes, pointed elf ears ",
		Hair:       "short lavender bob",
		Outfit:     "lilac cardigan over a cream shirt, pleated skirt",
		Reference:  "https://files.example.com/bob.png",
		Variants: []LookVariant{
			{Name: "Business", Outfit: "charcoal blazer, purple turtleneck, grey pencil skirt"},
			{Name: "bun", Hair: "lavender hair in a neat low bun"},
		},
	}
	// A look can start a profile.
	if err := s.SetLook("net", "Bob", look, "console:token"); err != nil {
		t.Fatal(err)
	}
	people, _ := s.People("net")
	bob, ok := people.Get("bob")
	if !ok || bob.Look.Appearance != "adult woman in her mid-20s, slim, green eyes, pointed elf ears" ||
		len(bob.Look.Variants) != 2 || bob.Look.Variants[0].Name != "business" {
		t.Fatalf("bob = %+v", bob)
	}

	// Saving the rest of the profile keeps the look.
	if err := s.SetProfile("net", Profile{Subject: "bob", Pronouns: "he/him", AvatarPronouns: "she/her"}); err != nil {
		t.Fatal(err)
	}
	people, _ = s.People("net")
	if bob, _ = people.Get("bob"); !bob.Look.Set() || bob.Pronouns != "he/him" {
		t.Errorf("after a profile save: %+v", bob)
	}

	for text, want := range map[string][3]string{
		"bob reading in a cafe":                {look.Hair, look.Outfit, ""},
		"bob in her business look at the desk": {look.Hair, "charcoal blazer, purple turtleneck, grey pencil skirt", "business"},
		"bob with her hair in a bun":           {"lavender hair in a neat low bun", look.Outfit, "bun"},
		"bob, bunny ears":                      {look.Hair, look.Outfit, ""},
	} {
		h, o, v := bob.Look.For(text)
		if [3]string{h, o, v} != want {
			t.Errorf("For(%q) = %q, %q, %q", text, h, o, v)
		}
	}

	for name, bad := range map[string]Look{
		"no appearance":   {Hair: "red"},
		"braces":          {Appearance: "a {style} woman"},
		"too long":        {Appearance: strings.Repeat("a", maxLookText+1)},
		"bad variant":     {Appearance: "a", Variants: []LookVariant{{Name: "two words", Outfit: "x"}}},
		"empty variant":   {Appearance: "a", Variants: []LookVariant{{Name: "plain"}}},
		"twice":           {Appearance: "a", Variants: []LookVariant{{Name: "x1", Hair: "h"}, {Name: "x1", Hair: "h"}}},
		"local reference": {Appearance: "a", Reference: "file:///etc/passwd"},
	} {
		if err := s.SetLook("net", "carol", bad, ""); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	// An empty look removes it and keeps the profile.
	if err := s.SetLook("net", "bob", Look{}, ""); err != nil {
		t.Fatal(err)
	}
	people, _ = s.People("net")
	if bob, ok = people.Get("bob"); !ok || bob.Look.Set() || bob.Pronouns != "he/him" {
		t.Errorf("after removing the look: %+v", bob)
	}
}
