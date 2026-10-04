// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestSameFact(t *testing.T) {
	cases := []struct {
		subject, a, b string
		same          bool
	}{
		{"alice", "pip is a rat", "Pip is a rat.", true},
		{"alice", "alice's pet rat is called Pip", "Pip is a rat", true},
		{"alice", "alice has a pet rat called Pip", "alice's rat Pip", true},
		{"bob", "bob likes cats", "bob hates cats", false},
		{"bob", "bob likes cats", "bob doesn't like cats", false},
		{"bob", "bob is not a nurse", "bob is a nurse", false},
		{"carol", "carol has a dog called Biscuit", "carol has a dog called Max", false},
		{"carol", "carol lives in Osaka", "carol plays guitar", false},
		{"dave", "dave writes Rust code", "dave writes Rust code for fun at weekends", true},
		{"dave", "rat", "dave owns a rat and a cat", false}, // one shared word says nothing
	}
	for _, c := range cases {
		if got := SameFact(c.subject, c.a, c.b); got != c.same {
			t.Errorf("SameFact(%q, %q, %q) = %v, want %v", c.subject, c.a, c.b, got, c.same)
		}
	}
}

func TestRememberMergedKeepsOneFactAndTheLongerWording(t *testing.T) {
	m, err := OpenMemoryStore(filepath.Join(t.TempDir(), "memories.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	first, merged, err := m.RememberMerged("net", "alice", "Pip is a rat", "alice", "#chat")
	if err != nil || merged {
		t.Fatalf("first save: %v merged=%v", err, merged)
	}
	id, merged, err := m.RememberMerged("net", "alice", "alice's pet rat is called Pip", "bob", "#chat")
	if err != nil || !merged || id != first {
		t.Fatalf("repeat: id %d (want %d) merged=%v err=%v", id, first, merged, err)
	}
	if _, merged, _ := m.RememberMerged("net", "alice", "Pip is a rat", "carol", "#chat"); !merged {
		t.Error("the shorter wording again should merge")
	}
	held, _ := m.Recall("net", "alice", 10)
	if len(held) != 1 || held[0].Fact != "alice's pet rat is called Pip" {
		t.Errorf("held: %+v", held)
	}
	if _, merged, _ := m.RememberMerged("net", "alice", "alice dislikes rats", "carol", "#chat"); merged {
		t.Error("a different fact merged")
	}
	if n, _ := m.CountSubject("net", "alice"); n != 2 {
		t.Errorf("count = %d", n)
	}
}

func TestUpdateAndSubjectCounts(t *testing.T) {
	m, err := OpenMemoryStore(filepath.Join(t.TempDir(), "memories.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	id, _ := m.Remember("net", "bob", "bob plays chess", "bob", "#chat")
	_, _ = m.Remember("net", "bob", "bob lives in Osaka", "bob", "#chat")
	_, _ = m.Remember("net", "carol", "carol plays go", "carol", "#chat")
	_, _ = m.Remember("other", "carol", "carol plays go", "carol", "#chat")
	if ok, err := m.Update("net", id, "bob plays chess on Sundays"); !ok || err != nil {
		t.Fatalf("update: %v %v", ok, err)
	}
	if ok, _ := m.Update("other", id, "x"); ok {
		t.Error("updated across networks")
	}
	if got, _, _ := m.Get("net", id); got.Fact != "bob plays chess on Sundays" {
		t.Errorf("after update: %q", got.Fact)
	}
	if found, _ := m.Search("net", "sundays", 5); len(found) != 1 {
		t.Error("the search index didn't follow the update")
	}
	counts, _ := m.SubjectCounts("net")
	if len(counts) != 2 || counts[0] != (SubjectCount{"bob", 2}) || counts[1] != (SubjectCount{"carol", 1}) {
		t.Errorf("counts = %+v", counts)
	}
}

func TestReplaceSubject(t *testing.T) {
	m, err := OpenMemoryStore(filepath.Join(t.TempDir(), "memories.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	a, _ := m.Remember("net", "alice", "alice has a rat called Pip", "alice", "#chat")
	b, _ := m.Remember("net", "alice", "Pip is alice's rat", "bob", "#chat")
	c, _ := m.Remember("net", "alice", "alice likes green tea", "alice", "#chat")
	_, _ = m.Remember("net", "bob", "bob plays chess", "bob", "#chat")

	if _, err := m.ReplaceSubject("net", "alice", []int64{a, b}, []string{"x"}, "compaction", "#chat"); !errors.Is(err, ErrMemoriesChanged) {
		t.Errorf("a stale preview: %v", err)
	}
	n, err := m.ReplaceSubject("net", "Alice", []int64{c, b, a}, []string{"alice has a pet rat called Pip",
		" alice likes green tea ", "alice likes green tea", ""}, "compaction", "#chat")
	if err != nil || n != 2 {
		t.Fatalf("replace: %d %v", n, err)
	}
	held, _ := m.Recall("net", "alice", 10)
	if len(held) != 2 || held[0].Author != "compaction" {
		t.Errorf("after: %+v", held)
	}
	if others, _ := m.Recall("net", "bob", 10); len(others) != 1 {
		t.Error("another subject was touched")
	}
	if found, _ := m.Search("net", "pet rat", 5); len(found) != 1 {
		t.Error("search doesn't see the compacted facts")
	}
}
