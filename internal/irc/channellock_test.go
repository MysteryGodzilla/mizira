// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/lrstanley/girc"

	"B4reMetal/metald/internal/config"
)

func lockConfig(channel string, partUnlisted bool) *config.Configuration {
	return &config.Configuration{
		Server: &config.ServerConfig{Channel: channel},
		Bot:    &config.BotConfig{PartUnlisted: partUnlisted},
	}
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestChannelAllowed(t *testing.T) {
	tests := []struct {
		name    string
		allowed string
		target  string
		want    bool
	}{
		{"configured channel", "#chat", "#chat", true},
		{"different case", "#chat", "#CHAT", true},
		{"rfc1459 casemapping", "#a[b]", "#A{B}", true},
		{"other channel", "#chat", "#test", false},
		{"lobby the server auto-joins", "#chat", "#lobby", false},
		{"same name, other prefix", "#chat", "&chat", false},
		{"private message to a nick", "#chat", "alice", true},
		{"empty config refuses channels", "", "#chat", false},
		{"empty config still allows a nick", "", "alice", true},
		// Hostile targets that girc doesn't call channels, but a server would deliver to one.
		{"channel list", "#chat", "#test,#chat", false},
		{"channel list, allowed first", "#chat", "#chat,#test", false},
		{"statusmsg to ops", "#chat", "@#test", false},
		{"statusmsg to ops of allowed", "#chat", "@#chat", false},
		{"nick list", "#chat", "alice,bob", false},
		{"trailing space", "#chat", "#chat ", false},
		{"empty target", "#chat", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ChannelAllowed(lockConfig(tt.allowed, false), tt.target); got != tt.want {
				t.Errorf("ChannelAllowed(%q, %q) = %v, want %v", tt.allowed, tt.target, got, tt.want)
			}
		})
	}
}

func TestChannelGateAdmit(t *testing.T) {
	const bot = "botnick"
	msg := func(target string) *girc.Event {
		return &girc.Event{Command: girc.PRIVMSG, Source: &girc.Source{Name: "alice"}, Params: []string{target, "botnick: hi"}}
	}
	tests := []struct {
		name  string
		event *girc.Event
		want  bool
	}{
		{"message in allowed channel", msg("#chat"), true},
		{"message in other channel", msg("#lobby"), false},
		{"private message", msg(bot), true},
		{"notice in other channel", &girc.Event{Command: girc.NOTICE, Source: &girc.Source{Name: "alice"}, Params: []string{"#lobby", "hi"}}, false},
		{"bot auto-joined elsewhere", &girc.Event{Command: girc.JOIN, Source: &girc.Source{Name: bot}, Params: []string{"#lobby"}}, false},
		{"bot joined allowed channel", &girc.Event{Command: girc.JOIN, Source: &girc.Source{Name: bot}, Params: []string{"#chat"}}, true},
		{"op given in other channel", &girc.Event{Command: girc.MODE, Source: &girc.Source{Name: "alice"}, Params: []string{"#lobby", "+o", bot}}, false},
		{"user mode on the bot", &girc.Event{Command: girc.MODE, Source: &girc.Source{Name: bot}, Params: []string{bot, "+i"}}, true},
		{"statusmsg to ops elsewhere", msg("@#lobby"), false},
		{"gated event without a target", &girc.Event{Command: girc.PRIVMSG, Source: &girc.Source{Name: "alice"}}, false},
		// CONNECTED and numerics aren't gated: blocking them would stop the bot joining its channel.
		{"connected", &girc.Event{Command: girc.CONNECTED, Params: []string{"irc.example.com:6667"}}, true},
		{"numeric error", &girc.Event{Command: girc.ERR_NOSUCHCHANNEL, Params: []string{bot, "#chat", "No such channel"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewChannelGate(quietLogger())
			parted := 0
			got := g.Admit(lockConfig("#chat", false), tt.event, bot, func(string) { parted++ })
			if got != tt.want {
				t.Errorf("Admit = %v, want %v", got, tt.want)
			}
			if parted != 0 {
				t.Errorf("parted %d times with partunlisted off, want 0", parted)
			}
		})
	}
}

// With partunlisted on, the bot leaves a channel it was auto-joined to exactly once per
// connection, even if the server keeps putting it back.
func TestChannelGatePartsOncePerConnection(t *testing.T) {
	cfg := lockConfig("#chat", true)
	g := NewChannelGate(quietLogger())
	join := func(who, channel string) *girc.Event {
		return &girc.Event{Command: girc.JOIN, Source: &girc.Source{Name: who}, Params: []string{channel}}
	}
	var parts []string
	part := func(channel string) { parts = append(parts, channel) }

	g.Admit(cfg, join("botnick", "#lobby"), "botnick", part)
	g.Admit(cfg, join("botnick", "#LOBBY"), "botnick", part) // server re-joins it
	g.Admit(cfg, join("alice", "#lobby"), "botnick", part)   // someone else joining isn't ours to answer
	g.Admit(cfg, join("botnick", "#chat"), "botnick", part)  // the allowed channel is never left
	if len(parts) != 1 || parts[0] != "#lobby" {
		t.Fatalf("parts = %v, want exactly one PART of #lobby", parts)
	}

	g.Reset() // reconnect
	g.Admit(cfg, join("botnick", "#lobby"), "botnick", part)
	if len(parts) != 2 {
		t.Fatalf("after reconnect parts = %v, want a second PART", parts)
	}
}

// The send-side checks run before the IRC client is touched. These contexts have no client, so
// reaching it would panic: a blocked call returning cleanly proves nothing was sent.
func TestSendGuardsRefuseOtherChannels(t *testing.T) {
	c := ChatContext{
		Context: context.Background(),
		Config:  lockConfig("#chat", false),
		logger:  quietLogger(),
		event:   &girc.Event{Command: girc.PRIVMSG, Source: &girc.Source{Name: "alice"}, Params: []string{"#lobby", "botnick: hi"}},
	}

	refusals := map[string]func() bool{
		"oper":          func() bool { return c.Oper("#lobby", "alice") },
		"kick":          func() bool { return c.Kick("#lobby", "alice", "bye") },
		"topic":         func() bool { return c.Topic("#lobby", "hi") },
		"join":          func() bool { return c.Join("#lobby") },
		"join with key": func() bool { return c.JoinWithKey("#lobby", "key") },
		"mode":          func() bool { return c.SetMode("#lobby", "+m") },
		"ban":           func() bool { return c.Ban("#lobby", "alice") },
		"unban":         func() bool { return c.Unban("#lobby", "alice") },
		"invite":        func() bool { return c.Invite("#lobby", "alice") },
		"channel list":  func() bool { return c.Topic("#chat,#lobby", "hi") },
	}
	for name, call := range refusals {
		if call() {
			t.Errorf("%s in another channel was allowed", name)
		}
	}

	// These return nothing; not panicking on the nil client is the assertion.
	c.SendAction("#lobby", "waves")
	c.Reply("hello")
	c.ReplyAction("waves")
}
