// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"database/sql"
	"errors"
	"time"
)

// A locked memory is kept by every forget path - the person's own "forget me", forgetting by id,
// an admin's +memories, compaction - until the operator unlocks it in the console.

// ErrMemoryLocked: the memory is locked; unlock it in the console first.
var ErrMemoryLocked = errors.New("that memory is locked; unlock it in the console first")

// notLocked is the SQL condition that leaves locked memories out of a delete.
const notLocked = `id NOT IN (SELECT id FROM memory_locks)`

func createMemoryLocks(db *sql.DB) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS memory_locks (
		id  INTEGER PRIMARY KEY,
		by  TEXT NOT NULL DEFAULT '',
		at  INTEGER NOT NULL
	);
	CREATE TRIGGER IF NOT EXISTS memory_locks_ad AFTER DELETE ON memories BEGIN
		DELETE FROM memory_locks WHERE id = old.id;
	END;`)
	return err
}

// Lock locks one memory. False if there is no such memory.
func (m *MemoryStore) Lock(network string, id int64, by string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res, err := m.db.Exec(`INSERT INTO memory_locks (id, by, at)
		SELECT id, ?, ? FROM memories WHERE id = ? AND network = ?
		ON CONFLICT(id) DO NOTHING`, by, time.Now().Unix(), id, network)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return true, nil
	}
	return m.lockedLocked(network, id), nil
}

// Unlock unlocks one memory. False if it wasn't locked.
func (m *MemoryStore) Unlock(network string, id int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res, err := m.db.Exec(`DELETE FROM memory_locks WHERE id IN (SELECT id FROM memories WHERE id = ? AND network = ?)`, id, network)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// IsLocked reports whether one memory is locked.
func (m *MemoryStore) IsLocked(network string, id int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lockedLocked(network, id)
}

func (m *MemoryStore) lockedLocked(network string, id int64) bool {
	var n int
	_ = m.db.QueryRow(`SELECT count(*) FROM memory_locks l JOIN memories mm ON mm.id = l.id
		WHERE l.id = ? AND mm.network = ?`, id, network).Scan(&n)
	return n > 0
}

// LockedIDs returns the locked memories on a network.
func (m *MemoryStore) LockedIDs(network string) (map[int64]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rows, err := m.db.Query(`SELECT l.id FROM memory_locks l JOIN memories mm ON mm.id = l.id WHERE mm.network = ?`, network)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// LockedCount is how many memories about subject are locked.
func (m *MemoryStore) LockedCount(network, subject string) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	_ = m.db.QueryRow(`SELECT count(*) FROM memories WHERE network = ? AND subject = ? AND id IN (SELECT id FROM memory_locks)`,
		network, normalizeSubject(subject)).Scan(&n)
	return n
}
