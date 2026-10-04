// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

// Naming a network that ran unnamed changes every key its data is stored under ("#chat" becomes
// "net/#chat"). Memories and reminders already move with AdoptUnscopedMemories and
// AdoptUnscopedReminders; these move the rest, so the conversation, recap, chat history, ignores and
// self-notes carry over. Each is a no-op once nothing is left unscoped.

// AdoptUnscoped moves conversations, recaps and chat-log lines saved without a network under
// network. An unscoped key has no "/": a channel ("#chat") or a nick for a private chat. A key
// already taken under the network is left alone rather than overwritten.
func (c *ContextDB) AdoptUnscoped(network string) (int64, error) {
	if network == "" {
		return 0, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	tx, err := c.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var moved int64
	for _, stmt := range []string{
		`UPDATE OR IGNORE sessions SET key = ? || '/' || key WHERE instr(key, '/') = 0`,
		`UPDATE OR IGNORE recaps SET key = ? || '/' || key WHERE instr(key, '/') = 0`,
		`UPDATE chatlog SET key = ? || '/' || key WHERE instr(key, '/') = 0`,
	} {
		res, err := tx.Exec(stmt, network)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		moved += n
	}
	return moved, tx.Commit()
}

// AdoptUnscoped moves ignores saved without a network under network.
func (s *IgnoreStore) AdoptUnscoped(network string) int {
	if network == "" {
		return 0
	}
	s.mu.Lock()
	n := 0
	for key, r := range s.entries {
		if !containsSlash(key) {
			scoped := ScopeKey(network, key)
			if _, taken := s.entries[scoped]; !taken {
				s.entries[scoped] = r
			}
			delete(s.entries, key)
			n++
		}
	}
	s.mu.Unlock()
	if n > 0 {
		s.save()
	}
	return n
}

// AdoptUnscopedSelfNotes moves self-notes saved without a network under network.
func (m *MemoryStore) AdoptUnscopedSelfNotes(network string) (int64, error) {
	if network == "" {
		return 0, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	res, err := m.db.Exec(`UPDATE self_notes SET network = ? WHERE network = ''`, network)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func containsSlash(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			return true
		}
	}
	return false
}
