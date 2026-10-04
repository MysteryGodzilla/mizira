// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAdoptUnscopedContext(t *testing.T) {
	c, err := OpenContextDB(filepath.Join(t.TempDir(), "context.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	now := time.Now().Unix()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO sessions (key, history, updated) VALUES (?, '[]', ?)`, []any{"#chat", now}},
		{`INSERT INTO sessions (key, history, updated) VALUES (?, '[]', ?)`, []any{"alice", now}},
		{`INSERT INTO sessions (key, history, updated) VALUES (?, '[]', ?)`, []any{"other/#chat", now}},
		{`INSERT INTO recaps (key, recap, updated) VALUES (?, 'what happened', ?)`, []any{"#chat", now}},
		{`INSERT INTO chatlog (key, nick, text, at) VALUES (?, 'bob', 'hello there', ?)`, []any{"#chat", now}},
	} {
		if _, err := c.db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	n, err := c.AdoptUnscoped("net")
	if err != nil || n != 4 {
		t.Fatalf("moved %d, %v", n, err)
	}
	if c.Recap("net/#chat") != "what happened" || c.Recap("#chat") != "" {
		t.Error("the recap didn't move")
	}
	var keys []string
	rows, _ := c.db.Query(`SELECT key FROM sessions ORDER BY key`)
	for rows.Next() {
		var k string
		_ = rows.Scan(&k)
		keys = append(keys, k)
	}
	rows.Close()
	if len(keys) != 3 || keys[0] != "net/#chat" || keys[1] != "net/alice" || keys[2] != "other/#chat" {
		t.Errorf("sessions: %v", keys)
	}
	if again, _ := c.AdoptUnscoped("net"); again != 0 {
		t.Errorf("a second run moved %d", again)
	}
}

func TestAdoptUnscopedIgnoresAndSelfNotes(t *testing.T) {
	s := NewIgnoreStore(filepath.Join(t.TempDir(), "ignores.json"))
	s.Add("", "mallory", time.Hour)
	s.Add("other", "eve", time.Hour)
	if n := s.AdoptUnscoped("net"); n != 1 || !s.IsIgnored("net", "mallory") || s.IsIgnored("", "mallory") || !s.IsIgnored("other", "eve") {
		t.Errorf("ignores: moved %d", n)
	}

	m, err := OpenMemoryStore(filepath.Join(t.TempDir(), "memories.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	id, _ := m.ProposeSelfNote("", "botty", "Botty hums while thinking", "")
	if n, err := m.AdoptUnscopedSelfNotes("net"); err != nil || n != 1 {
		t.Fatalf("self-notes: %d %v", n, err)
	}
	if _, ok, _ := m.SelfNote("net", id); !ok {
		t.Error("the self-note didn't move")
	}
}
