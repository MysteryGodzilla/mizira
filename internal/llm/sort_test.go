// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"slices"
	"testing"

	"B4reMetal/metald/internal/core"
)

func TestParseSort(t *testing.T) {
	mems := []core.Memory{
		{ID: 10, Subject: "botty", Fact: "botty feels sad when dave is mean"},
		{ID: 11, Subject: "spanish people", Fact: "carol noted that Spanish people speak Spanish"},
		{ID: 12, Subject: "bob", Fact: "bob and carol both like yuri"},
		{ID: 13, Subject: "bob", Fact: "bob's avatar has green hair"},
		{ID: 14, Subject: "carol", Fact: "carol plays chess"},
	}
	known := []string{"bob", "botty", "carol", "dave", "spanish people"}
	answer := `Here you go:
1 | lore | dave | botty
2. | likes | carol | -
3 | likes | bob | carol, mallory, botty
4 | identity | bob | -
5 | likes | #chat | -
3 | lore | dave | -
9 | lore | bob | -
not a line`
	got := parseSort(answer, mems, known, "botty", "#chat")
	want := []SortSuggestion{
		{ID: 10, Fact: mems[0].Fact, Subject: "botty", Kind: core.KindLore, About: "dave"},
		{ID: 11, Fact: mems[1].Fact, Subject: "spanish people", Kind: core.KindLikes, About: "carol"},
		{ID: 12, Fact: mems[2].Fact, Subject: "bob", Kind: core.KindLikes, About: "bob", Others: []string{"carol"}},
		{ID: 13, Fact: mems[3].Fact, Subject: "bob", Kind: core.KindLook, About: "bob"},
		{ID: 14, Fact: mems[4].Fact, Subject: "carol", Kind: core.KindLikes, About: "carol"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d suggestions: %+v", len(got), got)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.ID != w.ID || g.Kind != w.Kind || g.About != w.About || g.Subject != w.Subject || !slices.Equal(g.Others, w.Others) {
			t.Errorf("suggestion %d = %+v\n               want %+v", i, g, w)
		}
	}
}
