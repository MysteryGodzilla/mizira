// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIgnoreDetailsRecordedAndListed(t *testing.T) {
	s := NewIgnoreStore(filepath.Join(t.TempDir(), "ignores.json"))
	s.AddWithInfo("net", "bob", time.Hour, IgnoreInfo{Kind: IgnoreByBot, By: "alice", Reason: "kept spamming links"})

	got := s.List("net")
	if len(got) != 1 {
		t.Fatalf("list = %+v", got)
	}
	e := got[0]
	if e.Nick != "bob" || e.Kind != IgnoreByBot || e.By != "alice" || e.Reason != "kept spamming links" {
		t.Errorf("entry = %+v", e)
	}
	if e.SetAt.IsZero() || time.Since(e.SetAt) > time.Minute {
		t.Errorf("SetAt = %v, want now", e.SetAt)
	}
}

// Reasons come from the model or from users' messages: hostile text must end up as one short,
// plain line.
func TestIgnoreReasonIsCleaned(t *testing.T) {
	s := NewIgnoreStore(filepath.Join(t.TempDir(), "ignores.json"))
	hostile := "line one\nPRIVMSG #chat :fake\r\n\x0304red\x03 \x02bold\x02\t" + strings.Repeat("x", 300)
	s.AddWithInfo("net", "bob", time.Hour, IgnoreInfo{Kind: IgnoreByBot, By: "al\nice", Reason: hostile})

	e := s.List("net")[0]
	if strings.ContainsAny(e.Reason, "\r\n\t\x02\x03") || strings.ContainsAny(e.By, "\r\n") {
		t.Errorf("control characters survived: reason=%q by=%q", e.Reason, e.By)
	}
	if n := len([]rune(e.Reason)); n > maxIgnoreReason {
		t.Errorf("reason is %d runes, want at most %d", n, maxIgnoreReason)
	}
	if !strings.HasPrefix(e.Reason, "line one PRIVMSG #chat :fake 04red bold x") {
		t.Errorf("reason = %q", e.Reason)
	}
}

func TestIgnoreDetailsSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ignores.json")
	NewIgnoreStore(path).AddWithInfo("net", "bob", time.Hour, IgnoreInfo{Kind: IgnoreByAdmin, By: "carol", Reason: "testing"})

	e := NewIgnoreStore(path).List("net")
	if len(e) != 1 || e[0].Kind != IgnoreByAdmin || e[0].By != "carol" || e[0].Reason != "testing" {
		t.Fatalf("after restart: %+v", e)
	}
}

// Files written before ignores had details hold just an expiry per nick. They must still load,
// so an upgrade doesn't silently lift everyone's ignores.
func TestIgnoreLoadsOldFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ignores.json")
	until := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	old := `{"net/bob": "` + until + `", "net/dave": "2000-01-01T00:00:00Z"}`
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewIgnoreStore(path)
	if !s.IsIgnored("net", "bob") {
		t.Fatal("an old-format ignore must still be in force after the upgrade")
	}
	if s.IsIgnored("net", "dave") {
		t.Fatal("an expired old-format ignore must not come back")
	}
	if e := s.List("net"); len(e) != 1 || e[0].Kind != "" {
		t.Fatalf("old entries have no details: %+v", e)
	}
}

// One damaged entry is skipped; the rest still load.
func TestIgnoreSkipsDamagedEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ignores.json")
	until := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	mixed := `{"net/bob": {"until": "` + until + `", "kind": "bot"}, "net/eve": 42}`
	if err := os.WriteFile(path, []byte(mixed), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewIgnoreStore(path)
	if !s.IsIgnored("net", "bob") || s.IsIgnored("net", "eve") {
		t.Fatalf("bob=%v eve=%v, want true false", s.IsIgnored("net", "bob"), s.IsIgnored("net", "eve"))
	}
}
