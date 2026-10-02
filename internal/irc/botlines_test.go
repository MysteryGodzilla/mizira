// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"testing"
	"time"

	"github.com/lrstanley/girc"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
)

func botConfig(ownPrefix string, botPrefixes ...string) *config.Configuration {
	return &config.Configuration{
		Server: &config.ServerConfig{Name: "botlines-test", Channel: "#chat", Nick: "botnick"},
		Bot:    &config.BotConfig{ResponsePrefix: ownPrefix, BotPrefixes: botPrefixes},
	}
}

func TestClassifyLine(t *testing.T) {
	cfg := botConfig("\x0306🦋 [botnick]\x0F", "[otherbot]")
	tests := []struct {
		name string
		line string
		want LineKind
	}{
		{"plain human", "hey botnick, what's up", HumanLine},
		{"other bot", "[otherbot] hey botnick", BotLine},
		{"other bot, different case", "[OtherBot] hey botnick", BotLine},
		{"other bot, coloured tag", "\x0304[otherbot]\x03 hey botnick", BotLine},
		{"other bot, leading spaces", "   [otherbot] hey", BotLine},
		{"prefix mid-sentence is still human", "lol [otherbot] said hi to botnick", HumanLine},
		{"own line with colours", "\x0306🦋 [botnick]\x0F hello everyone", OwnLine},
		{"own line, colours stripped by a relay", "🦋 [botnick] hello everyone", OwnLine},
		{"human quoting the own tag mid-line", "she said 🦋 [botnick] hi", HumanLine},
		{"empty line", "", HumanLine},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyLine(cfg, "alice", tt.line); got != tt.want {
				t.Errorf("ClassifyLine(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

// Prefixes too short to be a real tag are ignored, so a bad config can't turn the whole channel
// into "bots". The same goes for the bot's own prefix: an empty one marks nothing as own.
func TestClassifyLineIgnoresShortPrefixes(t *testing.T) {
	cfg := botConfig("", "[", "ab")
	for _, line := range []string{"[hi]", "about that", "abc"} {
		if got := ClassifyLine(cfg, "alice", line); got != HumanLine {
			t.Errorf("ClassifyLine(%q) = %v, want HumanLine", line, got)
		}
	}
}

// Bots with their own account are recognised by nick, prefix or not.
func TestClassifyLineByNick(t *testing.T) {
	cfg := botConfig("[botnick]", "[otherbot]")
	cfg.Bot.BotNicks = []string{"CarolBot", "botnick"} // own nick listed by mistake
	tests := []struct {
		name string
		nick string
		line string
		want LineKind
	}{
		{"bot account, no prefix", "carolbot", "hi botnick", BotLine},
		{"bot account, rfc1459 case", "CAROLBOT", "hi", BotLine},
		{"human nick", "alice", "hi botnick", HumanLine},
		{"own nick is never a bot nick (the owner chats on it)", "botnick", "hi all", HumanLine},
		{"own nick with own prefix is still own", "botnick", "[botnick] hello", OwnLine},
		{"no nick", "", "hi", HumanLine},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyLine(cfg, tt.nick, tt.line); got != tt.want {
				t.Errorf("ClassifyLine(%q, %q) = %v, want %v", tt.nick, tt.line, got, tt.want)
			}
		})
	}
}

func TestValidateBotNick(t *testing.T) {
	cfg := botConfig("[botnick]")
	tests := []struct {
		nick string
		ok   bool
	}{
		{"carolbot", true},
		{"BotNick", false},  // configured own nick
		{"botnick_", false}, // live nick after a collision
		{"#chat", false},
		{"alice,bob", false},
		{"", false},
	}
	for _, tt := range tests {
		if err := ValidateBotNick(cfg, tt.nick, "botnick_"); (err == nil) != tt.ok {
			t.Errorf("ValidateBotNick(%q) error = %v, want ok=%v", tt.nick, err, tt.ok)
		}
	}
}

func TestValidateBotPrefix(t *testing.T) {
	cfg := botConfig("[botnick]")
	tests := []struct {
		prefix string
		ok     bool
	}{
		{"[metalai]", true},
		{"\x0304[x]\x03", true},
		{"[a", false},
		{"   ", false},
		{"\x0304\x03", false},
		{"[BOTNICK]", false}, // own prefix
	}
	for _, tt := range tests {
		if err := ValidateBotPrefix(cfg, tt.prefix); (err == nil) != tt.ok {
			t.Errorf("ValidateBotPrefix(%q) error = %v, want ok=%v", tt.prefix, err, tt.ok)
		}
	}
}

func TestTrackLine(t *testing.T) {
	cfg := botConfig("[botnick]", "[otherbot]")
	cfg.Bot.BotNicks = []string{"carolbot"}
	msg := func(text string) *girc.Event {
		return &girc.Event{Command: girc.PRIVMSG, Source: &girc.Source{Name: "alice"}, Params: []string{"#chat", text}}
	}
	// Fill the chain to the limit, then see which lines reset it.
	fill := func() {
		core.BotLoop().HumanSpoke(cfg.Server.Name, "#chat")
		core.BotLoop().Allow(cfg.Server.Name, "#chat", 1, time.Hour)
	}
	capped := func() bool {
		ok, _ := core.BotLoop().Allow(cfg.Server.Name, "#chat", 1, time.Hour)
		return !ok
	}

	tests := []struct {
		name      string
		event     *girc.Event
		wantDrop  bool
		wantReset bool
	}{
		{"own line is dropped", msg("[botnick] hello"), true, false},
		{"human message resets", msg("hi all"), false, true},
		{"bot line doesn't reset", msg("[otherbot] hi all"), false, false},
		{"bot account doesn't reset", &girc.Event{Command: girc.PRIVMSG, Source: &girc.Source{Name: "carolbot"}, Params: []string{"#chat", "hi all"}}, false, false},
		{"bot /me action doesn't reset", msg("\x01ACTION is thinking...\x01"), false, false},
		{"notice doesn't reset", &girc.Event{Command: girc.NOTICE, Source: &girc.Source{Name: "alice"}, Params: []string{"#chat", "hi"}}, false, false},
		{"private message doesn't reset", &girc.Event{Command: girc.PRIVMSG, Source: &girc.Source{Name: "alice"}, Params: []string{"botnick", "hi"}}, false, false},
		{"join is ignored", &girc.Event{Command: girc.JOIN, Source: &girc.Source{Name: "alice"}, Params: []string{"#chat"}}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fill()
			if drop := TrackLine(cfg, tt.event); drop != tt.wantDrop {
				t.Errorf("drop = %v, want %v", drop, tt.wantDrop)
			}
			if reset := !capped(); reset != tt.wantReset {
				t.Errorf("chain reset = %v, want %v", reset, tt.wantReset)
			}
		})
	}
}
