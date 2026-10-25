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
		h, o, v := bob.Look.For(text, 0)
		if [3]string{h, o, v} != want {
			t.Errorf("For(%q) = %q, %q, %q", text, h, o, v)
		}
	}
	for text, want := range map[string]string{
		"bob in her buisness look": "business",
		"bob, buns":                "",
		"carol in her business look next to bob with a bun": "bun",
		"bun hair carol, bob in her business look":          "business",
		"carol in her business look":                        "",
	} {
		if _, _, v := bob.Look.For(text, 0, "bob"); v != want {
			t.Errorf("For(%q, bob) = %q, want %q", text, v, want)
		}
	}
	// In a prompt the model wrote, "a sleek modern business office at night" named the scene.
	for text, want := range map[string]string{
		"a sleek modern business office at night with neon lights, bob leaning back": "",
		"bob dressed in her business suit at a desk":                                 "business",
		"business bob at a desk":                                                     "business",
	} {
		if _, _, v := bob.Look.For(text, 4, "bob"); v != want {
			t.Errorf("For(%q, 4, bob) = %q, want %q", text, v, want)
		}
	}

	// The defaults have a name too, "casual" unless the look gives one, and the usual words: after a run
	// of business pictures, "draw bob" carried the alt over and nothing asked for her own outfit.
	for text, want := range map[string]string{
		"casual bob at the park":                          "casual",
		"default bob at the park":                         "casual",
		"bob in her normal clothes":                       "casual",
		"bob in her casaul look":                          "casual",
		"casual carol next to business bob":               "business",
		"bob at a casual dinner with carol, nothing else": "casual",
	} {
		if _, _, v := bob.Look.For(text, 0, "bob"); v != want {
			t.Errorf("For(%q, bob) = %q, want %q", text, v, want)
		}
	}
	named := bob.Look
	named.DefaultName = "comfy"
	if h, o, v := named.For("comfy bob", 0, "bob"); v != "comfy" || h != look.Hair || o != look.Outfit {
		t.Errorf("a named default = %q, %q, %q", h, o, v)
	}
	if _, _, v := named.For("casual bob", 0, "bob"); v != "" {
		t.Errorf("casual with the default named comfy = %q, want none", v)
	}
	if err := s.SetLook("net", "dave", Look{Appearance: "a", DefaultName: " Comfy ", Outfit: "hoodie"}, ""); err != nil {
		t.Fatal(err)
	}
	people, _ = s.People("net")
	if dave, _ := people.Get("dave"); dave.Look.DefaultName != "comfy" || dave.Look.Default() != "comfy" {
		t.Errorf("dave's default = %q", dave.Look.DefaultName)
	}

	for name, bad := range map[string]Look{
		"default named twice": {Appearance: "a", DefaultName: "bun", Variants: []LookVariant{{Name: "bun", Hair: "h"}}},
		"two-word default":    {Appearance: "a", DefaultName: "every day"},
		"no appearance":       {Hair: "red"},
		"braces":              {Appearance: "a {style} woman"},
		"too long":            {Appearance: strings.Repeat("a", maxLookText+1)},
		"bad variant":         {Appearance: "a", Variants: []LookVariant{{Name: "two words", Outfit: "x"}}},
		"empty variant":       {Appearance: "a", Variants: []LookVariant{{Name: "plain"}}},
		"twice":               {Appearance: "a", Variants: []LookVariant{{Name: "x1", Hair: "h"}, {Name: "x1", Hair: "h"}}},
		"local reference":     {Appearance: "a", Reference: "file:///etc/passwd"},
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
