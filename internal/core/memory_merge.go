// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode"
)

// Two facts about one subject say the same thing when the shorter one's words are nearly all in
// the longer (containment), or they share most of their words (overlap). Measured on stemmed words
// without stop words or the subject's own name.
const (
	sameContainment = 0.8
	sameOverlap     = 0.6
)

// negations flip a fact's meaning, so a fact with one never merges with a fact without. "t" is
// what is left of "don't", "isn't", "can't" once the apostrophe splits the word.
var negations = map[string]bool{"not": true, "never": true, "no": true, "cannot": true, "t": true}

// contracted are the halves of negated contractions; the "t" carries the meaning.
var contracted = map[string]bool{"don": true, "doesn": true, "didn": true, "isn": true, "wasn": true,
	"aren": true, "won": true, "can": true, "is": true, "an": true}

// factWords are a fact's content words, stemmed, and whether it is negated.
func factWords(subject, fact string) (map[string]bool, bool) {
	words := map[string]bool{}
	negated := false
	for _, w := range strings.FieldsFunc(strings.ToLower(fact), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if negations[w] {
			negated = true
			continue
		}
		if len(w) < 2 || stopwords[w] || contracted[w] || w == subject {
			continue
		}
		words[stem(w)] = true
	}
	return words, negated
}

// SameFact reports whether two facts about subject say the same thing.
func SameFact(subject, a, b string) bool {
	subject = normalizeSubject(subject)
	wa, na := factWords(subject, a)
	wb, nb := factWords(subject, b)
	if na != nb {
		return false
	}
	small, large := wa, wb
	if len(small) > len(large) {
		small, large = large, small
	}
	if len(small) < 2 {
		return false // one word in common says nothing
	}
	shared := 0
	for w := range small {
		if large[w] {
			shared++
		}
	}
	union := len(wa) + len(wb) - shared
	return float64(shared)/float64(len(small)) >= sameContainment || float64(shared)/float64(union) >= sameOverlap
}

// RememberMerged is Remember for facts that may repeat one already held: a fact that says the same
// as an existing one about the subject updates that one (keeping the longer wording) instead of
// adding a row. It returns the id saved and whether it merged.
func (m *MemoryStore) RememberMerged(network, subject, fact, author, channel string) (int64, bool, error) {
	if same, ok, err := m.Similar(network, subject, fact); err != nil {
		return 0, false, err
	} else if ok {
		if len(strings.TrimSpace(fact)) > len(same.Fact) {
			if _, err := m.Update(network, same.ID, fact); err != nil {
				return 0, false, err
			}
		}
		return same.ID, true, nil
	}
	id, err := m.Remember(network, subject, fact, author, channel)
	return id, false, err
}

// Similar finds a memory about subject that says the same as fact.
func (m *MemoryStore) Similar(network, subject, fact string) (Memory, bool, error) {
	held, err := m.Recall(network, subject, 1000)
	if err != nil {
		return Memory{}, false, err
	}
	for _, mem := range held {
		if mem.Fact == strings.TrimSpace(fact) || SameFact(subject, mem.Fact, fact) {
			return mem, true, nil
		}
	}
	return Memory{}, false, nil
}

// Update rewrites one memory's fact. False if there is no such memory; an error if the new text
// equals another memory about the same subject.
func (m *MemoryStore) Update(network string, id int64, fact string) (bool, error) {
	fact = strings.TrimSpace(fact)
	if fact == "" {
		return false, sql.ErrNoRows
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	res, err := m.db.Exec(`UPDATE memories SET fact = ?, created = ? WHERE id = ? AND network = ?`,
		fact, time.Now().Unix(), id, network)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SubjectCount is one subject and how many memories it holds.
type SubjectCount struct {
	Subject string
	Count   int
}

// SubjectCounts lists every subject on a network with its count, most memories first.
func (m *MemoryStore) SubjectCounts(network string) ([]SubjectCount, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rows, err := m.db.Query(`SELECT subject, COUNT(*) FROM memories WHERE network = ?
		GROUP BY subject ORDER BY COUNT(*) DESC, subject`, network)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SubjectCount
	for rows.Next() {
		var s SubjectCount
		if err := rows.Scan(&s.Subject, &s.Count); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ErrMemoriesChanged: a subject's memories changed after a compaction was previewed.
var ErrMemoriesChanged = errors.New("the memories changed since the preview")

// ReplaceSubject swaps every memory about subject for facts, in one transaction, but only if the
// subject still holds exactly the memories expected (by id), so a fact saved while the operator was
// reviewing isn't lost. Facts that repeat each other are stored once. It returns how many it stored.
func (m *MemoryStore) ReplaceSubject(network, subject string, expected []int64, facts []string, author, channel string) (int, error) {
	subject = normalizeSubject(subject)
	m.mu.Lock()
	defer m.mu.Unlock()
	tx, err := m.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id FROM memories WHERE network = ? AND subject = ?`, network, subject)
	if err != nil {
		return 0, err
	}
	var held []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		held = append(held, id)
	}
	rows.Close()
	slices.Sort(held)
	want := slices.Clone(expected)
	slices.Sort(want)
	if !slices.Equal(held, want) {
		return 0, ErrMemoriesChanged
	}

	if _, err := tx.Exec(`DELETE FROM memories WHERE network = ? AND subject = ?`, network, subject); err != nil {
		return 0, err
	}
	now := time.Now().Unix()
	stored := 0
	for _, f := range facts {
		if f = strings.TrimSpace(f); f == "" {
			continue
		}
		res, err := tx.Exec(`INSERT INTO memories (network, subject, fact, author, channel, created)
			VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(network, subject, fact) DO NOTHING`,
			network, subject, f, author, channel, now)
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			stored++
		}
	}
	return stored, tx.Commit()
}
