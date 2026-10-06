// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"database/sql"
	"strings"
	"time"
)

// When a memory's wording last changed, kept apart from its date so an operator's edit doesn't move
// it to the top of a newest-first list.

func createMemoryEdits(db *sql.DB) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS memory_edits (
		id INTEGER PRIMARY KEY,
		at INTEGER NOT NULL
	);
	CREATE TRIGGER IF NOT EXISTS memory_edits_ad AFTER DELETE ON memories BEGIN
		DELETE FROM memory_edits WHERE id = old.id;
	END;`)
	return err
}

// Reword changes one memory's fact without changing its date. False if there is no such memory.
func (m *MemoryStore) Reword(network string, id int64, fact string) (bool, error) {
	fact = strings.TrimSpace(fact)
	if fact == "" {
		return false, sql.ErrNoRows
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	res, err := m.db.Exec(`UPDATE memories SET fact = ? WHERE id = ? AND network = ?`, fact, id, network)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	return true, m.markEdited(id)
}

// markEdited records that a memory's wording changed now. Callers hold m.mu.
func (m *MemoryStore) markEdited(id int64) error {
	_, err := m.db.Exec(`INSERT INTO memory_edits (id, at) VALUES (?, ?)
		ON CONFLICT(id) DO UPDATE SET at = excluded.at`, id, time.Now().Unix())
	return err
}

// EditedAt returns when each edited memory on a network was last reworded.
func (m *MemoryStore) EditedAt(network string) (map[int64]time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rows, err := m.db.Query(`SELECT e.id, e.at FROM memory_edits e JOIN memories mm ON mm.id = e.id WHERE mm.network = ?`, network)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]time.Time{}
	for rows.Next() {
		var id, at int64
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		out[id] = time.Unix(at, 0)
	}
	return out, rows.Err()
}
