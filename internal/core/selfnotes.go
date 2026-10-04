// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"database/sql"
	"strings"
	"time"
)

// Self-notes are notes the bot proposes about itself when a stretch of chat is folded into the
// recap. They wait for an admin: approved ones become room memory, denied ones stay recorded so
// the same note isn't proposed again. Nothing pending ever reaches a prompt.

const (
	SelfNotePending  = "pending"
	SelfNoteApproved = "approved"
	SelfNoteDenied   = "denied"
)

// SelfNote is one proposal.
type SelfNote struct {
	ID        int64
	Network   string
	Text      string
	Why       string // the words from the chat that prompted it
	Status    string
	Created   time.Time
	DecidedBy string
	DecidedAt time.Time
	MemoryID  int64 // the room memory an approval saved
}

func createSelfNotes(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS self_notes (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		network    TEXT NOT NULL DEFAULT '',
		text       TEXT NOT NULL,
		why        TEXT NOT NULL DEFAULT '',
		status     TEXT NOT NULL DEFAULT 'pending',
		created    INTEGER NOT NULL,
		decided_by TEXT NOT NULL DEFAULT '',
		decided_at INTEGER NOT NULL DEFAULT 0,
		memory_id  INTEGER NOT NULL DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_self_notes_status ON self_notes(network, status);`)
	return err
}

// ProposeSelfNote records a proposal unless one saying the same was proposed before (whatever
// became of it) or room memory about subject already says it. It returns the new id, or 0.
func (m *MemoryStore) ProposeSelfNote(network, subject, text, why string) (int64, error) {
	text, why = strings.TrimSpace(text), strings.TrimSpace(why)
	if text == "" {
		return 0, nil
	}
	if _, known, err := m.Similar(network, subject, text); err != nil || known {
		return 0, err
	}
	earlier, err := m.SelfNotes(network, "", 1000)
	if err != nil {
		return 0, err
	}
	for _, n := range earlier {
		if n.Text == text || SameFact(subject, n.Text, text) {
			return 0, nil
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	res, err := m.db.Exec(`INSERT INTO self_notes (network, text, why, status, created) VALUES (?, ?, ?, ?, ?)`,
		network, text, why, SelfNotePending, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SelfNotes lists proposals on a network, newest first; status "" is every status.
func (m *MemoryStore) SelfNotes(network, status string, limit int) ([]SelfNote, error) {
	if limit <= 0 {
		limit = 50
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rows, err := m.db.Query(`SELECT id, network, text, why, status, created, decided_by, decided_at, memory_id
		FROM self_notes WHERE network = ? AND (? = '' OR status = ?) ORDER BY id DESC LIMIT ?`,
		network, status, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SelfNote
	for rows.Next() {
		var n SelfNote
		var created, decided int64
		if err := rows.Scan(&n.ID, &n.Network, &n.Text, &n.Why, &n.Status, &created, &n.DecidedBy, &decided, &n.MemoryID); err != nil {
			return nil, err
		}
		n.Created = time.Unix(created, 0)
		if decided > 0 {
			n.DecidedAt = time.Unix(decided, 0)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// SelfNote returns one proposal.
func (m *MemoryStore) SelfNote(network string, id int64) (SelfNote, bool, error) {
	notes, err := m.SelfNotes(network, "", 1000)
	if err != nil {
		return SelfNote{}, false, err
	}
	for _, n := range notes {
		if n.ID == id {
			return n, true, nil
		}
	}
	return SelfNote{}, false, nil
}

// DecideSelfNote marks a pending proposal approved or denied, with the (possibly edited) text it was
// decided as. False if it isn't pending any more, so two admins can't both decide it.
func (m *MemoryStore) DecideSelfNote(network string, id int64, status, text, by string, memoryID int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res, err := m.db.Exec(`UPDATE self_notes SET status = ?, text = ?, decided_by = ?, decided_at = ?, memory_id = ?
		WHERE id = ? AND network = ? AND status = ?`,
		status, strings.TrimSpace(text), by, time.Now().Unix(), memoryID, id, network, SelfNotePending)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
