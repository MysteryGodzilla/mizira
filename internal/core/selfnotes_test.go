// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"path/filepath"
	"testing"
)

func TestSelfNoteLifecycle(t *testing.T) {
	m, err := OpenMemoryStore(filepath.Join(t.TempDir(), "memories.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if _, err := m.Remember("net", "botty", "botty loves green tea", "alice", "#chat"); err != nil {
		t.Fatal(err)
	}
	if id, _ := m.ProposeSelfNote("net", "botty", "Botty really loves green tea", ""); id != 0 {
		t.Error("proposed what room memory already says")
	}
	id, err := m.ProposeSelfNote("net", "botty", "Botty is called butterfly by alice", "night, butterfly")
	if err != nil || id == 0 {
		t.Fatalf("propose: %d %v", id, err)
	}
	if again, _ := m.ProposeSelfNote("net", "botty", "alice calls Botty butterfly", ""); again != 0 {
		t.Error("proposed the same note twice")
	}
	if other, _ := m.ProposeSelfNote("other", "botty", "Botty is called butterfly by alice", ""); other == 0 {
		t.Error("networks share proposals")
	}
	if pending, _ := m.SelfNotes("net", SelfNotePending, 10); len(pending) != 1 {
		t.Fatalf("pending: %+v", pending)
	}
	if ok, _ := m.DecideSelfNote("net", id, SelfNoteApproved, "Botty is 'butterfly' to alice", "bob", 7); !ok {
		t.Fatal("decide failed")
	}
	if ok, _ := m.DecideSelfNote("net", id, SelfNoteDenied, "x", "carol", 0); ok {
		t.Error("a decided note was decided again")
	}
	n, found, _ := m.SelfNote("net", id)
	if !found || n.Status != SelfNoteApproved || n.Text != "Botty is 'butterfly' to alice" || n.DecidedBy != "bob" || n.MemoryID != 7 {
		t.Errorf("after deciding: %+v", n)
	}
	if pending, _ := m.SelfNotes("net", SelfNotePending, 10); len(pending) != 0 {
		t.Errorf("still pending: %+v", pending)
	}
}
