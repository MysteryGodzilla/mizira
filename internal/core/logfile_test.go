// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return info.Size()
}

func TestRotatingFileRotatesAndKeeps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "bot.log")
	r, err := OpenRotatingFile(path, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	line := []byte(strings.Repeat("x", 29) + "\n") // 30 bytes: 3 fit in 100
	for range 12 {
		if _, err := r.Write(line); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{path, path + ".1", path + ".2"} {
		if size := fileSize(t, p); size > 100 || size%30 != 0 {
			t.Errorf("%s is %d bytes: over the limit or holding a split record", p, size)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Error("only 2 old files should be kept")
	}
}

// A record bigger than the limit still goes in whole, into a fresh file.
func TestRotatingFileOversizedRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bot.log")
	r, err := OpenRotatingFile(path, 50, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.Write([]byte("small\n"))
	big := []byte(strings.Repeat("y", 120) + "\n")
	if n, err := r.Write(big); err != nil || n != len(big) {
		t.Fatalf("write = %d, %v", n, err)
	}
	if got := fileSize(t, path); got != int64(len(big)) {
		t.Errorf("current file = %d bytes, want the whole big record (%d)", got, len(big))
	}
	if got := fileSize(t, path+".1"); got != 6 {
		t.Errorf("old file = %d bytes, want the small record", got)
	}
}

func TestRotatingFileKeepZero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bot.log")
	r, err := OpenRotatingFile(path, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for range 5 {
		r.Write([]byte("123456\n"))
	}
	if _, err := os.Stat(path + ".1"); !os.IsNotExist(err) {
		t.Error("keep 0 means no old files")
	}
}

// Reopening after a restart appends and counts the existing size towards the limit.
func TestRotatingFileReopenAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bot.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("z", 90)), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := OpenRotatingFile(path, 100, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	r.Write([]byte("0123456789abcdef\n")) // 90 + 17 > 100: must rotate first
	if got := fileSize(t, path+".1"); got != 90 {
		t.Errorf("old contents should have rotated out whole, .1 = %d bytes", got)
	}
}

func TestRotatingFileUnusablePath(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "notadir")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenRotatingFile(filepath.Join(blocker, "bot.log"), 100, 1); err == nil {
		t.Fatal("opening a log under a regular file must fail, so the caller can fall back to the console")
	}
	if _, err := OpenRotatingFile(filepath.Join(dir, "bot.log"), 0, 1); err == nil {
		t.Fatal("a zero max size must be refused")
	}
}

// The file gets JSON lines whatever the console format is, so it can be searched and parsed.
func TestInitLoggerWritesJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bot.log")
	r, err := OpenRotatingFile(path, 1<<20, 1)
	if err != nil {
		t.Fatal(err)
	}
	InitLogger("info", "text", r)
	t.Cleanup(func() { InitLogger("info", "text", nil) })

	GetLogger().Info("test_event", "nick", "alice", "text", "line with \"quotes\" and\nnewline")
	GetLogger().Debug("hidden_event") // below the level: must not appear
	r.Close()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var events []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var rec map[string]any
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("not a JSON line: %q", sc.Text())
		}
		events = append(events, rec["msg"].(string))
	}
	if len(events) != 1 || events[0] != "test_event" {
		t.Fatalf("events = %v, want just test_event", events)
	}
}

func TestLogFilePath(t *testing.T) {
	abs, _ := filepath.Abs(filepath.Join("x", "bot.log"))
	tests := []struct {
		dataDir, setting, want string
	}{
		{"data", "logs/mizira.log", filepath.Join("data", "logs", "mizira.log")},
		{"", "logs/mizira.log", filepath.Join(".", "logs", "mizira.log")},
		{"data", abs, abs},
		{"data", "", ""},
		{"data", "off", ""},
	}
	for _, tt := range tests {
		if got := LogFilePath(tt.dataDir, tt.setting); got != tt.want {
			t.Errorf("LogFilePath(%q, %q) = %q, want %q", tt.dataDir, tt.setting, got, tt.want)
		}
	}
}
