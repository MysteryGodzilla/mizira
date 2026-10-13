// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"slices"
	"testing"
)

func TestSharedMemoryComesBackForEveryone(t *testing.T) {
	s := testStore(t)
	id, _ := s.Remember("net", "bob", "bob and carol both like yuri", "bob", "#chat")
	other, _ := s.Remember("net", "dave", "dave likes trains", "dave", "#chat")
	if ok, err := s.SetAlso("net", id, []string{"Carol", "bob", " ", "carol"}); !ok || err != nil {
		t.Fatalf("set also = %v %v", ok, err)
	}
	if got, _ := s.Recall("net", "carol", 10); len(got) != 1 || got[0].ID != id {
		t.Errorf("carol's memories = %+v", got)
	}
	if got, _ := s.Recall("other", "carol", 10); len(got) != 0 {
		t.Errorf("another network sees it: %+v", got)
	}
	meta, _ := s.Meta("net", []int64{id, other})
	if !slices.Equal(meta[id].Also, []string{"carol"}) || len(meta[other].Also) != 0 {
		t.Errorf("meta = %+v", meta)
	}
	counts, _ := s.SubjectCounts("net")
	if !slices.Contains(counts, SubjectCount{"carol", 1}) || !slices.Contains(counts, SubjectCount{"bob", 1}) {
		t.Errorf("counts = %+v", counts)
	}
	if ok, err := s.SetAlso("net", id, []string{"a", "b", "c", "d", "e", "f"}); ok || err == nil {
		t.Error("six others were accepted")
	}
	if ok, _ := s.SetAlso("net", 999, []string{"carol"}); ok {
		t.Error("a missing memory took others")
	}

	// Carol forgetting everything about herself takes her off it; it stays bob's.
	if _, err := s.ForgetSubject("net", "carol"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Recall("net", "carol", 10); len(got) != 0 {
		t.Errorf("carol still has %+v", got)
	}
	if got, _ := s.Recall("net", "bob", 10); len(got) != 1 {
		t.Errorf("bob lost the memory: %+v", got)
	}

	// Deleting a memory takes its kind and others with it.
	s.SetAlso("net", id, []string{"carol"})
	s.SetKind("net", id, KindLikes)
	s.Forget("net", id)
	if meta, _ := s.Meta("net", []int64{id}); len(meta) != 0 {
		t.Errorf("left behind: %+v", meta)
	}
}

func TestSetKind(t *testing.T) {
	s := testStore(t)
	id, _ := s.Remember("net", "bob", "bob's avatar has green hair", "bob", "#chat")
	if ok, err := s.SetKind("net", id, KindLook); !ok || err != nil {
		t.Fatalf("set = %v %v", ok, err)
	}
	if _, err := s.SetKind("net", id, "weather"); err == nil {
		t.Error("an unknown kind was accepted")
	}
	if ok, _ := s.SetKind("other", id, KindLook); ok {
		t.Error("set a kind on another network's memory")
	}
	if meta, _ := s.Meta("net", []int64{id}); meta[id].Kind != KindLook {
		t.Errorf("kind = %q", meta[id].Kind)
	}
	s.SetKind("net", id, "")
	if meta, _ := s.Meta("net", []int64{id}); meta[id].Kind != "" {
		t.Errorf("cleared kind = %q", meta[id].Kind)
	}
}

func TestMemorySource(t *testing.T) {
	for _, c := range []struct{ subject, author, want string }{
		{"bob", "bob", SourceSaid},
		{"Bob", "BOB", SourceSaid},
		{"botty", "bob [botty]", SourceSaid},
		{"bob", "bob [botty]", SourceSaid},
		{"bob", "alice", SourceTold},
		{"bob", "alice [botty]", SourceTold},
		{"bob", "console:token", SourceOperator},
		{"bob", "compaction:console:token", SourceOperator},
		{"bob", "fold:console:token", SourceGuessed},
	} {
		if got := MemorySource(c.subject, c.author); got != c.want {
			t.Errorf("MemorySource(%q, %q) = %q, want %q", c.subject, c.author, got, c.want)
		}
	}
	if AuthorNick("alice [botty]") != "alice" {
		t.Error("author nick")
	}
}

func TestUnsortedAndRefile(t *testing.T) {
	s := testStore(t)
	a, _ := s.Remember("net", "botty", "botty feels sad when dave is mean", "console:token", "#chat")
	b, _ := s.Remember("net", "bob", "bob plays chess", "bob", "#chat")
	s.SetKind("net", b, KindLikes)
	s.SetAlso("net", a, []string{"dave"})
	if list, n, _ := s.Unsorted("net", 10); n != 1 || len(list) != 1 || list[0].ID != a {
		t.Errorf("unsorted = %d %+v", n, list)
	}
	if ok, err := s.Refile("net", a, "Dave"); !ok || err != nil {
		t.Fatalf("refile = %v %v", ok, err)
	}
	if got, _ := s.Recall("net", "dave", 5); len(got) != 1 || got[0].Subject != "dave" {
		t.Errorf("dave = %+v", got)
	}
	if meta, _ := s.Meta("net", []int64{a}); len(meta[a].Also) != 0 {
		t.Errorf("dave is still one of its others: %+v", meta[a])
	}
	if ok, _ := s.Refile("other", a, "carol"); ok {
		t.Error("refiled on another network")
	}
}
