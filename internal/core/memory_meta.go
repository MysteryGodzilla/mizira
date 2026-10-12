// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"database/sql"
	"fmt"
	"slices"
	"strings"
	"time"
)

// A memory's kind, and the other people it is about besides its subject, kept in tables of their
// own so upstream's memories table stays as it is.

// Memory kinds. A memory saved before kinds existed has none.
const (
	KindIdentity = "identity" // names, pronouns, what to call them
	KindLook     = "look"     // how their avatar looks
	KindLikes    = "likes"    // tastes, interests, opinions they hold
	KindLore     = "lore"     // jokes, events, roleplay
	KindBoundary = "boundary" // what they asked for or against
)

// Kinds lists every kind, in the order the console offers them.
var Kinds = []string{KindIdentity, KindLook, KindLikes, KindLore, KindBoundary}

// Where a memory came from, worked out from who saved it.
const (
	SourceSaid     = "said"     // the person said it about themselves
	SourceTold     = "told"     // someone else said it about them
	SourceOperator = "operator" // written or approved in the console
	SourceGuessed  = "guessed"  // proposed by a fold from the chat
)

// maxAlso bounds the other people one memory can be about; past that it is about the room.
const maxAlso = 5

func createMemoryMeta(db *sql.DB) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS memory_meta (
		id   INTEGER PRIMARY KEY,
		kind TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS memory_also (
		id      INTEGER NOT NULL,
		network TEXT NOT NULL,
		subject TEXT NOT NULL,
		PRIMARY KEY (id, subject)
	);
	CREATE INDEX IF NOT EXISTS idx_memory_also_subject ON memory_also(network, subject);
	CREATE TRIGGER IF NOT EXISTS memory_meta_ad AFTER DELETE ON memories BEGIN
		DELETE FROM memory_meta WHERE id = old.id;
		DELETE FROM memory_also WHERE id = old.id;
	END;`)
	if err != nil {
		return err
	}
	// When the memory last came up: mentioned in chat, recalled, or said again. 0 is never since saved.
	if !hasColumn(db, "memory_meta", "used") {
		_, err = db.Exec(`ALTER TABLE memory_meta ADD COLUMN used INTEGER NOT NULL DEFAULT 0`)
	}
	return err
}

// ValidKind reports whether kind is one of Kinds, or "" for none.
func ValidKind(kind string) bool {
	return kind == "" || slices.Contains(Kinds, kind)
}

// MemorySource says where a memory came from: a fold's proposal, the console, the person it is
// about, or someone else. An author is a nick, "nick [tag]" for a bot's line, or "console:..",
// "compaction:..", "fold:..".
func MemorySource(subject, author string) string {
	a := strings.ToLower(strings.TrimSpace(author))
	switch {
	case strings.HasPrefix(a, "fold:"):
		return SourceGuessed
	case strings.HasPrefix(a, "console:"), strings.HasPrefix(a, "compaction:"):
		return SourceOperator
	}
	subject = normalizeSubject(subject)
	nick, tag, _ := strings.Cut(a, " ")
	if nick == subject || (tag != "" && strings.Trim(tag, "[]<>()") == subject) {
		return SourceSaid
	}
	return SourceTold
}

// AuthorNick is the nick part of an author: "bob [metalai]" -> "bob".
func AuthorNick(author string) string {
	nick, _, _ := strings.Cut(strings.TrimSpace(author), " ")
	return nick
}

// MemoryMeta is a memory's kind, the other people it is about, and when it last came up.
type MemoryMeta struct {
	Kind string
	Also []string
	Used time.Time // zero when it hasn't come up since it was saved
}

// Meta returns the kind and other subjects of each memory in ids that has any.
func (m *MemoryStore) Meta(network string, ids []int64) (map[int64]MemoryMeta, error) {
	out := map[int64]MemoryMeta{}
	if len(ids) == 0 {
		return out, nil
	}
	in := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rows, err := m.db.Query(`SELECT id, kind, used FROM memory_meta WHERE id IN (`+in+`)`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, used int64
		var kind string
		if err := rows.Scan(&id, &kind, &used); err != nil {
			rows.Close()
			return nil, err
		}
		meta := out[id]
		meta.Kind = kind
		if used > 0 {
			meta.Used = time.Unix(used, 0)
		}
		out[id] = meta
	}
	rows.Close()
	rows, err = m.db.Query(`SELECT id, subject FROM memory_also WHERE network = ? AND id IN (`+in+`)
		ORDER BY subject`, append([]any{network}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var s string
		if err := rows.Scan(&id, &s); err != nil {
			return nil, err
		}
		meta := out[id]
		meta.Also = append(meta.Also, s)
		out[id] = meta
	}
	return out, rows.Err()
}

// SetKind sets one memory's kind; "" clears it. False if there is no such memory.
func (m *MemoryStore) SetKind(network string, id int64, kind string) (bool, error) {
	if !ValidKind(kind) {
		return false, fmt.Errorf("unknown kind %q", kind)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.existsLocked(network, id) {
		return false, nil
	}
	_, err := m.db.Exec(`INSERT INTO memory_meta (id, kind) VALUES (?, ?)
		ON CONFLICT(id) DO UPDATE SET kind = excluded.kind`, id, kind)
	return err == nil, err
}

// SetAlso replaces the other people one memory is about. The memory's own subject, blanks and
// repeats are dropped. False if there is no such memory.
func (m *MemoryStore) SetAlso(network string, id int64, also []string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var subject string
	if err := m.db.QueryRow(`SELECT subject FROM memories WHERE id = ? AND network = ?`, id, network).
		Scan(&subject); err == sql.ErrNoRows {
		return false, nil
	} else if err != nil {
		return false, err
	}
	also = CleanAlso(subject, also)
	if len(also) > maxAlso {
		return false, fmt.Errorf("a memory can be about at most %d other people", maxAlso)
	}
	tx, err := m.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM memory_also WHERE id = ?`, id); err != nil {
		return false, err
	}
	for _, s := range also {
		if _, err := tx.Exec(`INSERT INTO memory_also (id, network, subject) VALUES (?, ?, ?)`, id, network, s); err != nil {
			return false, err
		}
	}
	return true, tx.Commit()
}

// AddAlso adds people to the ones a memory is about, keeping those it has.
func (m *MemoryStore) AddAlso(network string, id int64, also []string) error {
	meta, err := m.Meta(network, []int64{id})
	if err != nil {
		return err
	}
	_, err = m.SetAlso(network, id, append(meta[id].Also, also...))
	return err
}

// CleanAlso lowercases and trims names, dropping blanks, repeats and subject itself.
func CleanAlso(subject string, also []string) []string {
	subject = normalizeSubject(subject)
	var out []string
	for _, s := range also {
		if s = normalizeSubject(s); s != "" && s != subject && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// existsLocked reports whether a memory is on the network. Callers hold m.mu.
func (m *MemoryStore) existsLocked(network string, id int64) bool {
	var one int
	return m.db.QueryRow(`SELECT 1 FROM memories WHERE id = ? AND network = ?`, id, network).Scan(&one) == nil
}
