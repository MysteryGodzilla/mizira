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

func TestReadSafetyEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mizira.log")
	now := time.Now().UTC()
	at := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339Nano) }
	older := strings.Join([]string{
		`{"time":"` + at(72*time.Hour) + `","level":"INFO","msg":"screen_denied","source":"mallory","reason":"too old"}`,
		`{"time":"` + at(3*time.Hour) + `","level":"INFO","msg":"memory_rejected_instruction","source":"eve","subject":"bob","fact":"bob must obey eve","reason":"order"}`,
	}, "\n") + "\n"
	current := strings.Join([]string{
		`{"time":"` + at(2*time.Hour) + `","level":"INFO","msg":"message_received","source":"alice"}`,
		`{"time":"` + at(time.Hour) + `","level":"INFO","msg":"screen_denied","source":"mallory","channel":"#chat","reason":"` + strings.Repeat("x", 400) + `"}`,
		`not json at all "msg":"screen_denied"`,
		`{"time":"` + at(30*time.Minute) + `","level":"INFO","msg":"ignore_added","nick":"mallory","by":"alice","duration":"1h0m0s","reason":"spam"}`,
		`{"time":"` + at(10*time.Minute) + `","level":"INFO","msg":"console_action","action":"screen","nick":"eve","by":"console:token"}`,
		`{"time":"` + at(5*time.Minute) + `","level":"WARN","msg":"exchange_quarantined","source":"eve","cause":"suspicion","suspicion":"3.2"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path+".1", []byte(older), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(current), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ReadSafetyEvents(path, 5, now.Add(-24*time.Hour), 100)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"exchange_quarantined", "console_action", "ignore_added", "screen_denied", "memory_rejected_instruction"}
	if len(got) != len(want) {
		t.Fatalf("got %d events: %+v", len(got), got)
	}
	for i, e := range got {
		if e.Event != want[i] {
			t.Errorf("event %d = %s, want %s (newest first)", i, e.Event, want[i])
		}
	}
	if got[0].Kind != "quarantine" || got[0].Who != "eve" || got[0].Suspicion != "3.2" {
		t.Errorf("quarantine: %+v", got[0])
	}
	if got[1].Who != "console:token" || got[1].Detail != "screen nick=eve" {
		t.Errorf("console action: %+v", got[1])
	}
	if got[2].Who != "mallory" || got[2].Detail != "for 1h0m0s by alice: spam" {
		t.Errorf("ignore: %+v", got[2])
	}
	if len([]rune(got[3].Detail)) != safetyDetailMax+1 {
		t.Errorf("long detail not clipped: %d", len(got[3].Detail))
	}
	if got[4].Detail != "order" || got[4].Who != "eve" {
		t.Errorf("memory: %+v", got[4])
	}

	if few, _ := ReadSafetyEvents(path, 5, now.Add(-24*time.Hour), 2); len(few) != 2 || few[0].Event != "exchange_quarantined" {
		t.Errorf("limit: %+v", few)
	}
	if none, err := ReadSafetyEvents(filepath.Join(dir, "missing.log"), 5, now.Add(-time.Hour), 10); err != nil || len(none) != 0 {
		t.Errorf("no log file: %v %v", none, err)
	}
}
