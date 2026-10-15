// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"database/sql"
	"slices"
	"strings"
	"time"
)

// Each kind ages its own way. A boundary is always sent first; identity and looks never fade; lore
// stops being sent unasked once it hasn't come up for a while, and comes back when it does. A
// memory another one replaces (a look merged into a new one, a like contradicted) is kept as an
// earlier version.

// kindRank orders what a speaker's memories send first: what they asked for, who they are, how they
// look, what they like, then the unsorted, then lore.
var kindRank = map[string]int{KindBoundary: 0, KindIdentity: 1, KindLook: 2, KindLikes: 3, "": 4, KindLore: 5}

// Faded reports whether a lore memory has gone fadeDays without coming up. Other kinds never fade.
func Faded(mem Memory, meta MemoryMeta, fadeDays int, now time.Time) bool {
	if meta.Kind != KindLore || fadeDays <= 0 {
		return false
	}
	last := mem.Created
	if meta.Used.After(last) {
		last = meta.Used
	}
	return now.Sub(last) > time.Duration(fadeDays)*24*time.Hour
}

// Ranked drops faded lore and orders the rest by kind, newest first within a kind, keeping at most
// limit. Every boundary is kept, whatever the limit.
func Ranked(mems []Memory, meta map[int64]MemoryMeta, fadeDays, limit int, now time.Time) []Memory {
	var out []Memory
	for _, m := range mems {
		if !Faded(m, meta[m.ID], fadeDays, now) {
			out = append(out, m)
		}
	}
	slices.SortStableFunc(out, func(a, b Memory) int {
		if d := kindRank[meta[a.ID].Kind] - kindRank[meta[b.ID].Kind]; d != 0 {
			return d
		}
		return b.Created.Compare(a.Created)
	})
	if limit > 0 && len(out) > limit {
		keep := limit
		for keep < len(out) && meta[out[keep].ID].Kind == KindBoundary {
			keep++
		}
		out = out[:keep]
	}
	return out
}

// MarkUsed records that memories came up now: mentioned in chat, recalled, or said again.
func (m *MemoryStore) MarkUsed(ids []int64, now time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if _, err := m.db.Exec(`INSERT INTO memory_meta (id, kind, used) SELECT id, '', ? FROM memories WHERE id = ?
			ON CONFLICT(id) DO UPDATE SET used = excluded.used`, now.Unix(), id); err != nil {
			return err
		}
	}
	return nil
}

// RankedFor is a subject's memories as they are sent unasked: faded lore left out, by kind.
func (m *MemoryStore) RankedFor(network, subject string, fadeDays, limit int, now time.Time) ([]Memory, map[int64]MemoryMeta, error) {
	all, err := m.Recall(network, subject, 1000)
	if err != nil {
		return nil, nil, err
	}
	meta, err := m.Meta(network, ids(all))
	if err != nil {
		return nil, nil, err
	}
	return Ranked(all, meta, fadeDays, limit, now), meta, nil
}

func ids(mems []Memory) []int64 {
	out := make([]int64, len(mems))
	for i, mem := range mems {
		out[i] = mem.ID
	}
	return out
}

// OfKind keeps the memories of one kind.
func OfKind(mems []Memory, meta map[int64]MemoryMeta, kind string) []Memory {
	var out []Memory
	for _, mem := range mems {
		if meta[mem.ID].Kind == kind {
			out = append(out, mem)
		}
	}
	return out
}

// Earlier is a memory's wording before another memory replaced it.
type Earlier struct {
	ID         int64
	MemoryID   int64 // the memory it was
	Subject    string
	Fact       string
	Kind       string
	Author     string
	Created    time.Time
	ReplacedBy int64 // the memory that holds it now
	Replaced   time.Time
	Why        string
}

func createMemoryHistory(db *sql.DB) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS memory_history (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		network     TEXT NOT NULL,
		memory_id   INTEGER NOT NULL,
		subject     TEXT NOT NULL,
		fact        TEXT NOT NULL,
		kind        TEXT NOT NULL DEFAULT '',
		author      TEXT NOT NULL DEFAULT '',
		created     INTEGER NOT NULL,
		replaced_by INTEGER NOT NULL,
		replaced    INTEGER NOT NULL,
		why         TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_memory_history_subject ON memory_history(network, subject);`)
	return err
}

// Supersede moves the old memories into the history as earlier versions of keep, deleting them, and
// gives keep the wording fact (dated now) unless fact is "". Locked memories aren't touched.
func (m *MemoryStore) Supersede(network string, keep int64, fact string, old []int64, why string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	for _, id := range old {
		if id == keep {
			continue
		}
		if _, err := tx.Exec(`INSERT INTO memory_history
			(network, memory_id, subject, fact, kind, author, created, replaced_by, replaced, why)
			SELECT network, id, subject, fact, COALESCE((SELECT kind FROM memory_meta WHERE id = memories.id), ''),
			  author, created, ?, ?, ? FROM memories WHERE id = ? AND network = ? AND `+notLocked,
			keep, now, why, id, network); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM memories WHERE id = ? AND network = ? AND `+notLocked, id, network); err != nil {
			return err
		}
	}
	if fact = strings.TrimSpace(fact); fact != "" {
		if _, err := tx.Exec(`UPDATE memories SET fact = ?, created = ? WHERE id = ? AND network = ?`,
			fact, now, keep, network); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// History lists a subject's earlier versions, newest first.
func (m *MemoryStore) History(network, subject string, limit int) ([]Earlier, error) {
	if limit <= 0 {
		limit = 50
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rows, err := m.db.Query(`SELECT id, memory_id, subject, fact, kind, author, created, replaced_by, replaced, why
		FROM memory_history WHERE network = ? AND subject = ? ORDER BY replaced DESC, id DESC LIMIT ?`,
		network, normalizeSubject(subject), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Earlier
	for rows.Next() {
		var e Earlier
		var created, replaced int64
		if err := rows.Scan(&e.ID, &e.MemoryID, &e.Subject, &e.Fact, &e.Kind, &e.Author, &created,
			&e.ReplacedBy, &replaced, &e.Why); err != nil {
			return nil, err
		}
		e.Created, e.Replaced = time.Unix(created, 0), time.Unix(replaced, 0)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Contradicts reports whether two facts about subject say the same thing, one negated: "likes
// pineapple" and "doesn't like pineapple".
func Contradicts(subject, a, b string) bool {
	subject = normalizeSubject(subject)
	wa, na := stemmedWords(subject, a)
	wb, nb := stemmedWords(subject, b)
	if na == nb || len(wa) < 1 || len(wa) != len(wb) {
		return false
	}
	for w := range wa {
		if !wb[w] {
			return false
		}
	}
	return true
}

// stemmedWords is factWords with stop words dropped after stemming, so "likes" and "like" both go.
func stemmedWords(subject, fact string) (map[string]bool, bool) {
	words, negated := factWords(subject, fact)
	for w := range words {
		if stopwords[w] {
			delete(words, w)
		}
	}
	return words, negated
}

// Correct rewords one memory, keeping its date, and keeps the old wording as an earlier version of
// it. False if there is no such memory; ErrMemoryLocked if it is locked.
func (m *MemoryStore) Correct(network string, id int64, fact string) (bool, error) {
	if m.IsLocked(network, id) {
		return false, ErrMemoryLocked
	}
	m.mu.Lock()
	res, err := m.db.Exec(`INSERT INTO memory_history
		(network, memory_id, subject, fact, kind, author, created, replaced_by, replaced, why)
		SELECT network, id, subject, fact, COALESCE((SELECT kind FROM memory_meta WHERE id = memories.id), ''),
		  author, created, id, ?, 'corrected' FROM memories WHERE id = ? AND network = ?`,
		time.Now().Unix(), id, network)
	m.mu.Unlock()
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	return m.Reword(network, id, fact)
}
