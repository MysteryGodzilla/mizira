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
// recap; people-notes are the same about the people who spoke. They wait for an admin: approved ones
// become memories, denied ones stay recorded so the same note isn't proposed again. Nothing pending
// ever reaches a prompt.

const (
	SelfNotePending  = "pending"
	SelfNoteApproved = "approved"
	SelfNoteDenied   = "denied"
	SelfNoteExpired  = "expired" // pending too long; kept, so it isn't proposed again
	// SelfNoteDecided lists every status but pending.
	SelfNoteDecided = "decided"
)

// SelfNoteExpiry is how long a note waits for a decision before it expires.
const SelfNoteExpiry = 14 * 24 * time.Hour

// SelfNote is one proposal.
type SelfNote struct {
	ID        int64
	Network   string
	Subject   string // who it's about; "" on notes from before people-notes, which are about the bot
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
	if err == nil && !hasColumn(db, "self_notes", "subject") {
		_, err = db.Exec(`ALTER TABLE self_notes ADD COLUMN subject TEXT NOT NULL DEFAULT ''`)
	}
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
	subject = normalizeSubject(subject)
	earlier, err := m.selfNotesAbout(network, subject)
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
	res, err := m.db.Exec(`INSERT INTO self_notes (network, subject, text, why, status, created) VALUES (?, ?, ?, ?, ?, ?)`,
		network, subject, text, why, SelfNotePending, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SelfNotes lists proposals on a network, newest first; status "" is every status.
func (m *MemoryStore) SelfNotes(network, status string, limit int) ([]SelfNote, error) {
	notes, _, err := m.SelfNotesPage(network, status, "", 0, limit)
	return notes, err
}

// SelfNotesPage lists one page of proposals, newest first: status "" is every status and
// SelfNoteDecided every status but pending; query matches the text, subject or quote. more reports
// whether a later page has any. Pending notes past SelfNoteExpiry expire first.
func (m *MemoryStore) SelfNotesPage(network, status, query string, offset, limit int) (notes []SelfNote, more bool, err error) {
	if limit <= 0 {
		limit = 50
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.expireSelfNotes(network); err != nil {
		return nil, false, err
	}
	like := "%" + strings.ToLower(strings.TrimSpace(query)) + "%"
	rows, err := m.db.Query(`SELECT id, network, subject, text, why, status, created, decided_by, decided_at, memory_id
		FROM self_notes WHERE network = ?
		AND (? = '' OR (? = 'decided' AND status != 'pending') OR status = ?)
		AND (lower(text) LIKE ? OR lower(subject) LIKE ? OR lower(why) LIKE ?)
		ORDER BY id DESC LIMIT ? OFFSET ?`,
		network, status, status, status, like, like, like, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	notes, err = scanSelfNotes(rows)
	if len(notes) > limit {
		notes, more = notes[:limit], true
	}
	return notes, more, err
}

// selfNotesAbout is every proposal about subject, and the bot's notes from before subjects were kept.
func (m *MemoryStore) selfNotesAbout(network, subject string) ([]SelfNote, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rows, err := m.db.Query(`SELECT id, network, subject, text, why, status, created, decided_by, decided_at, memory_id
		FROM self_notes WHERE network = ? AND (subject = ? OR subject = '')`, network, subject)
	if err != nil {
		return nil, err
	}
	return scanSelfNotes(rows)
}

// expireSelfNotes marks pending notes past SelfNoteExpiry expired. Callers hold m.mu.
func (m *MemoryStore) expireSelfNotes(network string) error {
	now := time.Now()
	_, err := m.db.Exec(`UPDATE self_notes SET status = ?, decided_by = 'expiry', decided_at = ?
		WHERE network = ? AND status = ? AND created < ?`,
		SelfNoteExpired, now.Unix(), network, SelfNotePending, now.Add(-SelfNoteExpiry).Unix())
	return err
}

func scanSelfNotes(rows *sql.Rows) ([]SelfNote, error) {
	defer rows.Close()
	var out []SelfNote
	for rows.Next() {
		var n SelfNote
		var created, decided int64
		if err := rows.Scan(&n.ID, &n.Network, &n.Subject, &n.Text, &n.Why, &n.Status, &created, &n.DecidedBy, &decided, &n.MemoryID); err != nil {
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
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.expireSelfNotes(network); err != nil {
		return SelfNote{}, false, err
	}
	rows, err := m.db.Query(`SELECT id, network, subject, text, why, status, created, decided_by, decided_at, memory_id
		FROM self_notes WHERE network = ? AND id = ?`, network, id)
	if err != nil {
		return SelfNote{}, false, err
	}
	notes, err := scanSelfNotes(rows)
	if err != nil || len(notes) == 0 {
		return SelfNote{}, false, err
	}
	return notes[0], true, nil
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
