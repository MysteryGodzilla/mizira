// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"strings"
	"testing"
	"time"
)

func TestMatchMask(t *testing.T) {
	tests := []struct {
		mask, hostmask string
		want           bool
	}{
		{"alice!user@host.example.com", "alice!user@host.example.com", true},
		{"alice!*@host.example.com", "alice!~whatever@host.example.com", true},
		{"alice!*@*.example.com", "alice!u@a.b.example.com", true},
		{"alice!*@*.example.com", "alice!u@example.com", false}, // needs a dot before
		{"alice!*@*", "alice!u@anywhere", true},
		{"alice!*@*", "malice!u@anywhere", false}, // the nick part is not a substring match
		{"alice!*@*", "alice2!u@anywhere", false},
		{"ALICE!*@HOST", "alice!u@host", true},     // case-insensitive
		{"[alice]!*@host", "{alice}!u@host", true}, // RFC 1459: [] equals {}
		{"[a]!*@host", "a!u@host", false},          // [ ] are literal, not a character class
		{"al?ce!*@host", "alice!u@host", true},
		{"al?ce!*@host", "allice!u@host", false},
		{"alice!*@host", "alice!u@host.evil.com", false}, // no implicit trailing wildcard
		{"alice!*@host", "", false},
		{"*!*@cloak/staff/mystery", "anynick!u@cloak/staff/mystery", true},
		{"*!*@cloak/staff/mystery", "anynick!u@cloak/staff/mystery2", false},
	}
	for _, tt := range tests {
		if got := matchMask(tt.mask, tt.hostmask); got != tt.want {
			t.Errorf("matchMask(%q, %q) = %v, want %v", tt.mask, tt.hostmask, got, tt.want)
		}
	}
}

// Hostile input: many stars against a long near-miss must still finish fast (no exponential
// backtracking).
func TestMatchMaskIsFast(t *testing.T) {
	mask := strings.Repeat("*a", 50) + "!*@b"
	hostmask := strings.Repeat("a", 5000) + "!u@c"
	start := time.Now()
	if matchMask(mask, hostmask) {
		t.Fatal("should not match")
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("took %v", d)
	}
}

func TestValidateAdminMask(t *testing.T) {
	tests := []struct {
		mask string
		ok   bool
	}{
		{"alice!user@host.example.com", true},
		{"alice!*@*", true},               // allowed, with a warning
		{"*!*@host.example.com", true},    // allowed, with a warning
		{"*!*@cloak/staff/mystery", true}, // cloaks have slashes
		{"*!*@*", false},                  // everyone
		{"?*!*@*?", false},                // still everyone
		{"alice", false},                  // not a mask
		{"alice!user", false},
		{"alice@host", false},
		{"!user@host", false},
		{"alice!user@", false},
		{"alice!*@host other!*@host", false}, // space
		{"alice!*@a,b", false},               // comma
		{"alice!*@host\n", false},            // control character
	}
	for _, tt := range tests {
		if err := ValidateAdminMask(tt.mask); (err == nil) != tt.ok {
			t.Errorf("ValidateAdminMask(%q) = %v, want ok=%v", tt.mask, err, tt.ok)
		}
	}
}

func TestAdminMaskWarning(t *testing.T) {
	if AdminMaskWarning("alice!user@host.example.com") != "" {
		t.Error("a specific mask needs no warning")
	}
	if AdminMaskWarning("alice!*@*") == "" {
		t.Error("nick from any host must warn")
	}
	if AdminMaskWarning("*!*@host.example.com") == "" {
		t.Error("anyone from a host must warn")
	}
}

func TestCheckAdminWildcards(t *testing.T) {
	admins := []string{"*!*@*", "alice!*@*.example.com"} // the first is refused and must never match
	tests := []struct {
		hostmask string
		want     bool
	}{
		{"alice!u@home.example.com", true},
		{"mallory!u@home.example.com", false},
		{"mallory!u@evil.net", false},
	}
	for _, tt := range tests {
		if got := CheckAdmin(tt.hostmask, admins); got != tt.want {
			t.Errorf("CheckAdmin(%q) = %v, want %v", tt.hostmask, got, tt.want)
		}
	}
}
