// Copyright (C) 2026 BareMetal
// Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// IgnoreStore tracks temporarily ignored nicks.
type IgnoreStore struct {
	mu      sync.RWMutex
	path    string
	entries map[string]ignoreRecord // lowercased scoped nick -> record
}

// Kinds of ignore, so an operator can tell who decided it.
const (
	IgnoreByFlood = "flood" // automatic flood timeout
	IgnoreByBot   = "bot"   // the model used its irc__ignore tool
	IgnoreByAdmin = "admin" // an admin ran +ignore
)

// IgnoreInfo explains an ignore: what kind it is, who caused it and why. For IgnoreByBot, By is
// the nick whose message the bot was answering, which matters because someone can try to talk
// the bot into ignoring somebody else (A11).
type IgnoreInfo struct {
	Kind   string    `json:"kind,omitempty"`
	By     string    `json:"by,omitempty"`
	Reason string    `json:"reason,omitempty"`
	SetAt  time.Time `json:"set_at,omitzero"`
}

// ignoreRecord is one stored ignore.
type ignoreRecord struct {
	Until time.Time `json:"until"`
	IgnoreInfo
}

// maxIgnoreReason caps a stored reason. Reasons can come from the model or a user's message,
// so they are kept short and on one line.
const maxIgnoreReason = 120

// cleanIgnoreReason puts a reason on one line: control characters (newlines, IRC colour and
// bold codes) become spaces, runs of spaces collapse, and it is cut to maxIgnoreReason.
func cleanIgnoreReason(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > maxIgnoreReason {
		s = string(r[:maxIgnoreReason-1]) + "…"
	}
	return s
}

var (
	globalIgnores     *IgnoreStore
	globalIgnoresOnce sync.Once
)

// Ignores returns the process-wide ignore store, loading persisted state on
// first use.
func Ignores() *IgnoreStore {
	globalIgnoresOnce.Do(func() {
		globalIgnores = NewIgnoreStore(DataPath("ignores.json"))
	})
	return globalIgnores
}

// NewIgnoreStore creates a store backed by path, loading any existing state.
func NewIgnoreStore(path string) *IgnoreStore {
	s := &IgnoreStore{path: path, entries: make(map[string]ignoreRecord)}
	s.load()
	return s
}

// normalizeNick lowercases for comparison - IRC nicks are case-insensitive,
// so "Dave" and "dave" must be the same entry.
func normalizeNick(nick string) string {
	return strings.ToLower(strings.TrimSpace(nick))
}

// Add ignores nick for d, replacing any existing entry. Returns the expiry.
func (s *IgnoreStore) Add(network, nick string, d time.Duration) time.Time {
	return s.AddWithInfo(network, nick, d, IgnoreInfo{})
}

// AddWithInfo is Add that also records why, so +ignore list can show it later.
func (s *IgnoreStore) AddWithInfo(network, nick string, d time.Duration, info IgnoreInfo) time.Time {
	nick = ScopeKey(network, nick)
	now := time.Now()
	expiry := now.Add(d)
	info.Reason = cleanIgnoreReason(info.Reason)
	info.By = cleanIgnoreReason(info.By)
	info.SetAt = now
	s.mu.Lock()
	s.entries[normalizeNick(nick)] = ignoreRecord{Until: expiry, IgnoreInfo: info}
	s.mu.Unlock()
	s.save()
	return expiry
}

// Remove un-ignores nick. Returns false if it wasn't ignored.
func (s *IgnoreStore) Remove(network, nick string) bool {
	nick = ScopeKey(network, nick)
	key := normalizeNick(nick)
	s.mu.Lock()
	_, existed := s.entries[key]
	delete(s.entries, key)
	s.mu.Unlock()
	if existed {
		s.save()
	}
	return existed
}

// IsIgnored reports whether nick is currently ignored, purging the entry if
// it has expired.
func (s *IgnoreStore) IsIgnored(network, nick string) bool {
	nick = ScopeKey(network, nick)
	key := normalizeNick(nick)

	s.mu.RLock()
	rec, ok := s.entries[key]
	s.mu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().Before(rec.Until) {
		return true
	}

	// Expired - drop it so the list stays clean.
	s.mu.Lock()
	if cur, still := s.entries[key]; still && !time.Now().Before(cur.Until) {
		delete(s.entries, key)
		s.mu.Unlock()
		s.save()
		return false
	}
	s.mu.Unlock()
	return false
}

// IgnoreEntry is one active ignore, for listing.
type IgnoreEntry struct {
	Nick   string
	Expiry time.Time
	IgnoreInfo
}

// List returns the currently-active ignores, sorted by nick, dropping any it has expired.
func (s *IgnoreStore) List(network string) []IgnoreEntry {
	now := time.Now()
	var out []IgnoreEntry
	var expired []string

	s.mu.RLock()
	for nick, rec := range s.entries {
		if !now.Before(rec.Until) {
			expired = append(expired, nick)
			continue
		}
		// This network only: the testbed must not disclose who is muted on the live network.
		if keyInNetwork(network, nick) {
			out = append(out, IgnoreEntry{Nick: UnscopeKey(network, nick), Expiry: rec.Until, IgnoreInfo: rec.IgnoreInfo})
		}
	}
	s.mu.RUnlock()

	if len(expired) > 0 {
		s.mu.Lock()
		for _, nick := range expired {
			if cur, ok := s.entries[nick]; ok && !now.Before(cur.Until) {
				delete(s.entries, nick)
			}
		}
		s.mu.Unlock()
		s.save()
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Nick < out[j].Nick })
	return out
}

// load reads persisted state. A missing or unreadable file is not an error -
// it just means nothing is ignored yet.
func (s *IgnoreStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		GetLogger().Warn("ignore_store_unreadable", "path", s.path, "error", err.Error())
		return
	}
	now := time.Now()
	for nick, value := range raw {
		rec, err := decodeIgnoreRecord(value)
		if err != nil {
			GetLogger().Warn("ignore_entry_unreadable", "nick", nick, "error", err.Error())
			continue
		}
		if now.Before(rec.Until) {
			s.entries[normalizeNick(nick)] = rec
		}
	}
}

// decodeIgnoreRecord reads one saved entry. Files written before ignores had details hold just
// the expiry time; those still load, with no details.
func decodeIgnoreRecord(value json.RawMessage) (ignoreRecord, error) {
	var rec ignoreRecord
	if len(value) > 0 && value[0] == '"' {
		err := json.Unmarshal(value, &rec.Until)
		return rec, err
	}
	err := json.Unmarshal(value, &rec)
	return rec, err
}

// save writes state atomically (temp file + rename) so a crash mid-write
// can't leave a truncated file that loses every active ignore.
func (s *IgnoreStore) save() {
	s.mu.RLock()
	snapshot := make(map[string]ignoreRecord, len(s.entries))
	for nick, rec := range s.entries {
		snapshot[nick] = rec
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		GetLogger().Error("ignore_store_marshal_failed", "error", err.Error())
		return
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		GetLogger().Error("ignore_store_write_failed", "path", tmp, "error", err.Error())
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		GetLogger().Error("ignore_store_rename_failed", "path", s.path, "error", err.Error())
		_ = os.Remove(tmp)
		return
	}
	_ = filepath.Clean(s.path)
}
