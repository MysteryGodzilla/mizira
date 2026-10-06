// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"path/filepath"
	"testing"
)

// A locked memory survives every way of forgetting until it is unlocked.
func TestLockedMemorySurvivesForgetting(t *testing.T) {
	m, err := OpenMemoryStore(filepath.Join(t.TempDir(), "memories.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	keep, _ := m.Remember("net", "bob", "bob plays the bass", "bob", "#chat")
	_, _ = m.Remember("net", "bob", "bob lives by the sea", "bob", "#chat")
	if ok, err := m.Lock("net", keep, "console:token"); !ok || err != nil {
		t.Fatalf("lock: %v %v", ok, err)
	}
	if ok, _ := m.Lock("other", keep, "console:token"); ok {
		t.Error("locked a memory from another network")
	}
	if ok, _ := m.Lock("net", 999, "console:token"); ok {
		t.Error("locked a memory that doesn't exist")
	}

	if ok, _ := m.Forget("net", keep); ok {
		t.Error("forgot a locked memory by id")
	}
	if n, _ := m.ForgetSubject("net", "bob"); n != 1 {
		t.Errorf("forget everything about bob removed %d, want only the unlocked one", n)
	}
	if n := m.LockedCount("net", "bob"); n != 1 {
		t.Errorf("locked count = %d", n)
	}
	_, _ = m.Remember("net", "carol", "carol has a cat", "carol", "#chat")
	if n, _ := m.Clear("net"); n != 1 {
		t.Errorf("wipe removed %d, want everything but the locked one", n)
	}
	if _, found, _ := m.Get("net", keep); !found {
		t.Fatal("the locked memory is gone")
	}

	// A longer repeat merges into it without rewording it.
	if id, merged, _ := m.RememberMerged("net", "bob", "bob plays the bass guitar every weekend", "bob", "#chat"); id != keep || !merged {
		t.Errorf("repeat of a locked fact: id %d merged %v", id, merged)
	}
	if got, _, _ := m.Get("net", keep); got.Fact != "bob plays the bass" {
		t.Errorf("locked fact reworded to %q", got.Fact)
	}

	if ok, _ := m.Unlock("net", keep); !ok {
		t.Fatal("unlock")
	}
	if ok, _ := m.Forget("net", keep); !ok {
		t.Error("couldn't forget it once unlocked")
	}
}

// Compaction rewrites only the unlocked facts and keeps the locked ones as they are.
func TestCompactionKeepsLockedMemories(t *testing.T) {
	m, err := OpenMemoryStore(filepath.Join(t.TempDir(), "memories.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	locked, _ := m.Remember("net", "bob", "bob plays the bass", "bob", "#chat")
	a, _ := m.Remember("net", "bob", "bob likes tea", "bob", "#chat")
	b, _ := m.Remember("net", "bob", "bob drinks tea daily", "bob", "#chat")
	_, _ = m.Lock("net", locked, "console:token")

	if _, err := m.ReplaceSubject("net", "bob", []int64{a, b, locked}, []string{"x"}, "compaction", "#chat"); err != ErrMemoriesChanged {
		t.Errorf("a preview that included the locked fact: %v, want ErrMemoriesChanged", err)
	}
	if n, err := m.ReplaceSubject("net", "bob", []int64{a, b}, []string{"bob drinks tea every day"}, "compaction", "#chat"); err != nil || n != 1 {
		t.Fatalf("replace: %d %v", n, err)
	}
	mems, _ := m.Recall("net", "bob", 10)
	if len(mems) != 2 {
		t.Fatalf("bob has %d memories, want the locked one and the compacted one", len(mems))
	}
	if _, found, _ := m.Get("net", locked); !found || !m.IsLocked("net", locked) {
		t.Error("the locked memory didn't survive compaction")
	}
}
