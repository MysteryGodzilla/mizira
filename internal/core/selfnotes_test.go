// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"path/filepath"
	"testing"
	"time"
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

// A note left pending past SelfNoteExpiry expires, stays listed among the decided ones, and can't be
// approved; listings page and filter.
func TestSelfNotesExpireAndPage(t *testing.T) {
	m, err := OpenMemoryStore(filepath.Join(t.TempDir(), "memories.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	old, _ := m.ProposeSelfNote("net", "carol", "carol is learning the cello", "cello")
	_, _ = m.ProposeSelfNote("net", "dave", "dave built a canoe", "canoe")
	_, _ = m.ProposeSelfNote("net", "dave", "dave likes otters", "otters")
	_, _ = m.db.Exec(`UPDATE self_notes SET created = ? WHERE id = ?`, time.Now().Add(-SelfNoteExpiry-time.Hour).Unix(), old)

	pending, _, _ := m.SelfNotesPage("net", SelfNotePending, "", 0, 10)
	if len(pending) != 2 {
		t.Errorf("pending: %+v", pending)
	}
	decided, _, _ := m.SelfNotesPage("net", SelfNoteDecided, "", 0, 10)
	if len(decided) != 1 || decided[0].ID != old || decided[0].Status != SelfNoteExpired || decided[0].DecidedBy != "expiry" {
		t.Errorf("decided: %+v", decided)
	}
	if ok, _ := m.DecideSelfNote("net", old, SelfNoteApproved, "x", "alice", 0); ok {
		t.Error("approved an expired note")
	}
	if id, _ := m.ProposeSelfNote("net", "carol", "carol is learning the cello", "again"); id != 0 {
		t.Error("an expired note was proposed again")
	}

	page, more, _ := m.SelfNotesPage("net", "", "", 0, 2)
	rest, more2, _ := m.SelfNotesPage("net", "", "", 2, 2)
	if len(page) != 2 || !more || len(rest) != 1 || more2 {
		t.Errorf("pages: %d more %v, then %d more %v", len(page), more, len(rest), more2)
	}
	if found, _, _ := m.SelfNotesPage("net", "", "OTTER", 0, 10); len(found) != 1 || found[0].Subject != "dave" {
		t.Errorf("search: %+v", found)
	}
}
