// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"slices"
	"testing"
	"time"
)

func TestRankedOrdersByKindAndDropsFadedLore(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	mem := func(id int64, ago time.Duration) Memory { return Memory{ID: id, Created: now.Add(-ago)} }
	mems := []Memory{mem(1, day), mem(2, 2*day), mem(3, 100*day), mem(4, 100*day), mem(5, 3*day), mem(6, 200*day), mem(7, 0)}
	meta := map[int64]MemoryMeta{
		1: {Kind: KindLore},
		2: {Kind: KindLikes},
		3: {Kind: KindLore},                      // faded
		4: {Kind: KindLore, Used: now.Add(-day)}, // old but came up yesterday
		5: {Kind: KindBoundary},
		6: {Kind: KindIdentity}, // old, never fades
	}
	got := Ranked(mems, meta, 45, 0, now)
	want := []int64{5, 6, 2, 7, 1, 4}
	if !slices.Equal(ids(got), want) {
		t.Errorf("ranked = %v, want %v", ids(got), want)
	}
	if got := Ranked(mems, meta, 0, 0, now); len(got) != 7 {
		t.Errorf("fading off still dropped: %v", ids(got))
	}
	// A limit never drops a boundary.
	meta[2] = MemoryMeta{Kind: KindBoundary}
	if got := Ranked(mems, meta, 45, 1, now); !slices.Equal(ids(got), []int64{2, 5}) {
		t.Errorf("limited = %v", ids(got))
	}
}

func TestSupersedeKeepsEarlierVersions(t *testing.T) {
	s := testStore(t)
	old, _ := s.Remember("net", "bob", "bob's avatar has blue hair", "bob", "#chat")
	locked, _ := s.Remember("net", "bob", "bob's avatar wears a cape", "console:token", "#chat")
	s.SetKind("net", old, KindLook)
	s.Lock("net", locked, "console:token")
	keep, _ := s.Remember("net", "bob", "bob's avatar has green hair", "bob", "#chat")

	if err := s.Supersede("net", keep, "bob's avatar has green hair and a red scarf", []int64{old, locked, keep}, "look merged"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Recall("net", "bob", 10)
	if len(got) != 2 {
		t.Fatalf("memories = %+v", got)
	}
	if m, _, _ := s.Get("net", keep); m.Fact != "bob's avatar has green hair and a red scarf" {
		t.Errorf("kept = %q", m.Fact)
	}
	h, _ := s.History("net", "bob", 10)
	if len(h) != 1 || h[0].Fact != "bob's avatar has blue hair" || h[0].Kind != KindLook || h[0].ReplacedBy != keep ||
		h[0].Why != "look merged" || h[0].MemoryID != old {
		t.Errorf("history = %+v", h)
	}
	if h, _ := s.History("other", "bob", 10); len(h) != 0 {
		t.Errorf("another network sees %+v", h)
	}
}

func TestContradicts(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"bob likes pineapple on pizza", "bob doesn't like pineapple on pizza", true},
		{"bob is not a morning person", "bob is a morning person", true},
		{"bob likes pineapple on pizza", "bob likes pineapple on pizza", false},
		{"bob likes pineapple", "bob doesn't like anchovies", false},
		{"bob loves tea", "bob doesn't like coffee or tea", false},
	} {
		if got := Contradicts("bob", c.a, c.b); got != c.want {
			t.Errorf("Contradicts(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestMarkUsedAndRelevantFaded(t *testing.T) {
	s := testStore(t)
	id, _ := s.Remember("net", "bob", "bob once fell into the duck pond at the picnic", "bob", "#chat")
	s.SetKind("net", id, KindLore)
	// Saved long ago: faded.
	s.db.Exec(`UPDATE memories SET created = ? WHERE id = ?`, time.Now().Add(-100*24*time.Hour).Unix(), id)

	if got, _ := s.RelevantFaded("net", "has bob been around?", nil, 5, 45); len(got) != 0 {
		t.Errorf("faded lore sent for a bare mention: %+v", got)
	}
	if got, _ := s.RelevantFaded("net", "remember the duck pond picnic?", nil, 5, 45); len(got) != 1 {
		t.Fatalf("lore the words match = %+v", got)
	}
	// It came up, so it is back.
	if got, _ := s.RelevantFaded("net", "has bob been around?", nil, 5, 45); len(got) != 1 {
		t.Errorf("refreshed lore = %+v", got)
	}
	if meta, _ := s.Meta("net", []int64{id}); meta[id].Used.IsZero() || meta[id].Kind != KindLore {
		t.Errorf("meta = %+v", meta[id])
	}
}

func TestCompactionCarriesKindsAndKeepsHistory(t *testing.T) {
	s := testStore(t)
	look, _ := s.Remember("net", "bob", "bob's avatar has long silver hair", "bob", "#chat")
	joke, _ := s.Remember("net", "bob", "bob and carol tell train puns", "bob", "#chat")
	s.SetKind("net", look, KindLook)
	s.SetKind("net", joke, KindLore)
	s.SetAlso("net", joke, []string{"carol"})
	shared, _ := s.Remember("net", "dave", "dave and bob share a desk", "dave", "#chat")
	s.SetAlso("net", shared, []string{"bob"})

	n, err := s.ReplaceSubject("net", "bob", []int64{look, joke},
		[]string{"bob's avatar has long silver hair and a ribbon", "bob and carol tell terrible train puns"}, "compaction:console:token", "#chat")
	if err != nil || n != 2 {
		t.Fatalf("replace = %d %v", n, err)
	}
	got, _ := s.Recall("net", "bob", 10)
	meta, _ := s.Meta("net", ids(got))
	kinds := map[string]MemoryMeta{}
	for _, m := range got {
		kinds[m.Fact] = meta[m.ID]
	}
	if kinds["bob's avatar has long silver hair and a ribbon"].Kind != KindLook {
		t.Errorf("look lost its kind: %+v", kinds)
	}
	if k := kinds["bob and carol tell terrible train puns"]; k.Kind != KindLore || !slices.Equal(k.Also, []string{"carol"}) {
		t.Errorf("lore = %+v", k)
	}
	if _, ok := kinds["dave and bob share a desk"]; !ok {
		t.Error("a memory shared from dave was compacted away")
	}
	if h, _ := s.History("net", "bob", 10); len(h) != 2 || h[0].Why != "compacted" {
		t.Errorf("history = %+v", h)
	}
}

func TestCleanLook(t *testing.T) {
	for in, want := range map[string]string{
		"bob's avatar has green hair.":                         "bob's avatar has green hair.",
		"<think>hmm</think>\n\"bob's avatar has green hair.\"": "bob's avatar has green hair.",
		"- bob's avatar has green hair.\n\nI merged them.":     "bob's avatar has green hair.",
	} {
		if got, err := cleanLook(in); err != nil || got != want {
			t.Errorf("cleanLook(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := cleanLook("   "); err == nil {
		t.Error("empty answer accepted")
	}
}
