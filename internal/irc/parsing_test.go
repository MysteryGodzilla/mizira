// Copyright (C) 2023-2026 Alex Schlessinger and soulshack contributors
// Modified 2026 by BareMetal
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"testing"
)

func TestCheckAddressed(t *testing.T) {
	tests := []struct {
		name    string
		message string
		nick    string
		want    bool
	}{
		// Calling on the bot.
		{"name first with colon", "bot: hello", "bot", true},
		{"name first with space", "bot hello", "bot", true},
		{"name first with comma", "bot, hello", "bot", true},
		{"at-mention", "@bot hi", "bot", true},
		{"greeting before", "hey bot, how are you", "bot", true},
		{"greeting with punctuation", "hey, bot!", "bot", true},
		{"thanks before", "thank you bot", "bot", true},
		{"good morning", "Good morning Bot", "bot", true},
		{"question ending in name", "What do you think bot?", "bot", true},
		{"praise ending in name", "nice one bot", "bot", true},
		{"set off by commas", "ok so, bot, what now?", "bot", true},
		{"after another bot's tag", "[otherbot] bot, it's me", "bot", true},
		{"just the name", "bot", "bot", true},
		{"multi-word trigger after filler", "yo hey bot wake up", "hey bot", true},
		// Only talking about it.
		{"subject mid-line", "Hey alice did you see what bot did", "bot", false},
		{"object at the end", "alice say hi to bot", "bot", false},
		{"question about it", "who the fuck is bot", "bot", false},
		{"addressed to someone else", "alice: who is bot", "bot", false},
		{"addressed to someone else with comma", "greg, bot is a fork", "bot", false},
		{"other bot's tag then someone else", "[otherbot] greg, bot is my sister", "bot", false},
		{"mid-line plain", "i think bot is cute", "bot", false},
		{"possessive", "bot's code is neat", "bot", false},
		{"statement about it", "bot is my little-sister fork", "bot", false},
		{"statement after a tag", "[otherbot] Bot is Monica Everett", "bot", false},
		{"question with is", "bot is this right?", "bot", true},
		{"comma then is", "bot, is it raining", "bot", true},
		{"who's at the end", "who's bot", "bot", false},
		{"embedded in longer word", "botter hello", "bot", false},
		{"embedded mid message", "hey heybot there", "bot", false},
		{"all caps longer word", "ROBOTS hello", "bot", false},
		{"multi-word trigger embedded", "xhey botx wake up", "hey bot", false},
		{"empty message", "", "bot", false},
		{"empty nick", "bot: hello", "", true}, // empty trigger matches everything
		{"case insensitive trigger", "bot: hello", "Bot", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckAddressed(tt.message, tt.nick)
			if got != tt.want {
				t.Errorf("CheckAddressed(%q, %q) = %v, want %v", tt.message, tt.nick, got, tt.want)
			}
		})
	}
}

func TestCheckAdmin_EmptyList(t *testing.T) {
	// An empty admin list means nobody is admin; a missing config line must not grant access.
	got := CheckAdmin("anyone!user@host.com", []string{})
	if got {
		t.Error("CheckAdmin with empty list must return false (nobody is admin)")
	}
	if CheckAdmin("anyone!user@host.com", nil) {
		t.Error("CheckAdmin with nil list must return false (nobody is admin)")
	}
}

func TestCheckAdmin_ExactMatch(t *testing.T) {
	admins := []string{"admin!user@trusted.host"}

	tests := []struct {
		name     string
		hostmask string
		want     bool
	}{
		{"exact match", "admin!user@trusted.host", true},
		{"different nick", "other!user@trusted.host", false},
		{"different user", "admin!other@trusted.host", false},
		{"different host", "admin!user@other.host", false},
		{"partial match", "admin!user@trusted", false},
		{"empty hostmask", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckAdmin(tt.hostmask, admins)
			if got != tt.want {
				t.Errorf("CheckAdmin(%q, admins) = %v, want %v", tt.hostmask, got, tt.want)
			}
		})
	}
}

func TestCheckAdmin_MultipleAdmins(t *testing.T) {
	admins := []string{
		"admin1!user@host1.com",
		"admin2!user@host2.com",
		"admin3!user@host3.com",
	}

	tests := []struct {
		hostmask string
		want     bool
	}{
		{"admin1!user@host1.com", true},
		{"admin2!user@host2.com", true},
		{"admin3!user@host3.com", true},
		{"admin4!user@host4.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.hostmask, func(t *testing.T) {
			got := CheckAdmin(tt.hostmask, admins)
			if got != tt.want {
				t.Errorf("CheckAdmin(%q) = %v, want %v", tt.hostmask, got, tt.want)
			}
		})
	}
}

func TestCheckPrivate(t *testing.T) {
	tests := []struct {
		target string
		want   bool
	}{
		{"#channel", false},
		{"#test", false},
		{"##double", false},
		{"nickname", true},
		{"user123", true},
		{"", true}, // Empty is technically not a channel
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got := CheckPrivate(tt.target)
			if got != tt.want {
				t.Errorf("CheckPrivate(%q) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}

func TestValidateHostmask_Valid(t *testing.T) {
	tests := []struct {
		name     string
		hostmask string
	}{
		{"basic", "nick!user@host.com"},
		{"with tilde prefix", "nick!~user@host.com"},
		{"with hyphen in nick", "nick-name!user@host.com"},
		{"with underscore in user", "nick!user_name@host.com"},
		{"with dot in user", "nick!user.name@host.com"},
		{"ipv4 host", "nick!user@192.168.1.1"},
		{"ipv6 host", "nick!user@::1"},
		{"ipv6 full", "nick!user@2001:db8::1"},
		{"subdomain", "nick!user@sub.domain.example.com"},
		{"special nick chars", "[nick]!user@host.com"},
		{"backslash nick", "nick\\name!user@host.com"},
		{"caret nick", "nick^name!user@host.com"},
		{"backtick nick", "`nick!user@host.com"},
		{"pipe nick", "nick|away!user@host.com"},
		{"curly nick", "{nick}!user@host.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHostmask(tt.hostmask)
			if err != nil {
				t.Errorf("ValidateHostmask(%q) = %v, want nil", tt.hostmask, err)
			}
		})
	}
}

func TestValidateHostmask_Invalid(t *testing.T) {
	tests := []struct {
		name     string
		hostmask string
		wantErr  string
	}{
		{"empty", "", "cannot be empty"},
		{"no exclamation", "nick@host.com", "must contain '!'"},
		{"no at sign", "nick!userhost.com", "must contain '@'"},
		{"wrong order", "nick@user!host.com", "'!' must come before '@'"},
		{"empty nick", "!user@host.com", "nick cannot be empty"},
		{"empty user", "nick!@host.com", "user cannot be empty"},
		{"empty host", "nick!user@", "host cannot be empty"},
		{"nick starts with digit", "1nick!user@host.com", "invalid nick"},
		{"nick starts with hyphen", "-nick!user@host.com", "invalid nick"},
		{"nick with space", "nick name!user@host.com", "invalid nick"},
		{"user with space", "nick!user name@host.com", "invalid user"},
		{"invalid host", "nick!user@host..com", "invalid host"},
		{"host with space", "nick!user@host name.com", "invalid host"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHostmask(tt.hostmask)
			if err == nil {
				t.Errorf("ValidateHostmask(%q) = nil, want error containing %q", tt.hostmask, tt.wantErr)
				return
			}
			if !contains(err.Error(), tt.wantErr) {
				t.Errorf("ValidateHostmask(%q) error = %q, want error containing %q", tt.hostmask, err.Error(), tt.wantErr)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// A line opening with someone else's name is talking to them, whatever follows.
func TestCheckAddressedAmong(t *testing.T) {
	others := func(w string) bool { return w == "otherbot" || w == "alice" }
	cases := map[string]bool{
		"otherbot who's bot":                            false,
		"otherbot make a song about alice stalking bot": false,
		"alice what do you think bot?":                  false,
		"bot what do you think?":                        true,
		"hey bot, what does otherbot say?":              true,
		"so what do you think bot?":                     true,
	}
	for msg, want := range cases {
		if got := CheckAddressedAmong(msg, "bot", others); got != want {
			t.Errorf("CheckAddressedAmong(%q) = %v, want %v", msg, got, want)
		}
	}
}
