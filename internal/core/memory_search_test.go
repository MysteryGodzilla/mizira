// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"path/filepath"
	"testing"
)

// "rats" finds a fact about a rat, facts holding every word come first, and a piece of a nick still
// matches as before.
func TestSearchWords(t *testing.T) {
	m, err := OpenMemoryStore(filepath.Join(t.TempDir(), "memories.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	rat, _ := m.Remember("net", "carol", "carol has a pet rat called Pip", "carol", "#chat")
	both, _ := m.Remember("net", "dave", "dave keeps two rats in a big cage", "dave", "#chat")
	_, _ = m.Remember("net", "eve", "eve dislikes cages", "eve", "#chat")
	_, _ = m.Remember("other", "bob", "bob has a rat", "bob", "#chat")

	got, err := m.SearchWords("net", "rats", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !((got[0].ID == rat && got[1].ID == both) || (got[0].ID == both && got[1].ID == rat)) {
		t.Errorf("rats: %+v", got)
	}
	if got, _ := m.SearchWords("net", "rats cage", 10); len(got) != 1 || got[0].ID != both {
		t.Errorf("rats cage: %+v", got)
	}
	if got, _ := m.SearchWords("net", "car", 10); len(got) != 1 || got[0].Subject != "carol" {
		t.Errorf("part of a nick: %+v", got)
	}
}

// An operator's reword keeps the memory's date; both a reword and a merge record an edit.
func TestRewordKeepsTheDate(t *testing.T) {
	m, err := OpenMemoryStore(filepath.Join(t.TempDir(), "memories.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	id, _ := m.Remember("net", "bob", "bob plays bass", "bob", "#chat")
	_, _ = m.db.Exec(`UPDATE memories SET created = 1000 WHERE id = ?`, id)

	if ok, err := m.Reword("net", id, "bob plays the bass guitar"); !ok || err != nil {
		t.Fatalf("reword: %v %v", ok, err)
	}
	got, _, _ := m.Get("net", id)
	if got.Fact != "bob plays the bass guitar" || got.Created.Unix() != 1000 {
		t.Errorf("after reword: %+v", got)
	}
	if edited, _ := m.EditedAt("net"); edited[id].IsZero() {
		t.Error("reword not recorded as an edit")
	}
	if ok, _ := m.Reword("other", id, "x"); ok {
		t.Error("reworded a memory from another network")
	}
	if _, err := m.Forget("net", id); err != nil {
		t.Fatal(err)
	}
	if edited, _ := m.EditedAt("net"); len(edited) != 0 {
		t.Error("a forgotten memory's edit record stayed")
	}
}
