// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import "strings"

// SearchWords finds memories by their words through the full-text index, which stems them, so "rats"
// finds "rat": facts holding every word come first, best match first, then the substring matches
// Search finds (part of a nick, a word too short for the index).
func (m *MemoryStore) SearchWords(network, query string, limit int) ([]Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	var out []Memory
	seen := map[int64]bool{}
	if match := strings.ReplaceAll(FTSQuery(query), " OR ", " "); match != "" {
		m.mu.Lock()
		rows, err := m.db.Query(
			`SELECT m.id, m.network, m.subject, m.fact, m.author, m.channel, m.created
			 FROM memories_fts f JOIN memories m ON m.id = f.rowid
			 WHERE memories_fts MATCH ? AND m.network = ?
			 ORDER BY bm25(memories_fts) LIMIT ?`, match, network, limit)
		if err != nil {
			m.mu.Unlock()
			return nil, err
		}
		found, err := scanMemories(rows)
		rows.Close()
		m.mu.Unlock()
		if err != nil {
			return nil, err
		}
		for _, mem := range found {
			seen[mem.ID] = true
			out = append(out, mem)
		}
	}
	plain, err := m.Search(network, query, limit)
	if err != nil {
		return nil, err
	}
	for _, mem := range plain {
		if len(out) < limit && !seen[mem.ID] {
			out = append(out, mem)
		}
	}
	return out, nil
}
