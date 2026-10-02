// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"log/slog"
	"sync"

	"github.com/lrstanley/girc"

	"B4reMetal/metald/internal/config"
)

// ChannelAllowed reports whether the bot may read from or send to target: the configured
// channel, or a single plain nick (a private message; ignoreprivate decides those separately).
//
// A13: the bot may speak only in its configured channel. Servers can auto-join it elsewhere
// (some networks put everyone in a lobby channel), and every other channel is off-limits.
// This is an allowlist on purpose: targets that are neither, such as "#a,#b" (a list) or
// "@#a" (a message to a channel's ops), would still reach other channels, so they're refused.
func ChannelAllowed(cfg *config.Configuration, target string) bool {
	allowed := cfg.Server.Channel
	// IRC channel names are case-insensitive, with RFC 1459 casemapping ([]\ equal {}|).
	// An empty allowed channel matches nothing: fail closed.
	if allowed != "" && girc.ToRFC1459(target) == girc.ToRFC1459(allowed) {
		return true
	}
	return girc.IsValidNick(target)
}

// gatedEvents are the event types whose first parameter is where the event happened (a channel,
// or the bot's own nick for a private message or user mode). Other events, such as CONNECTED or
// numeric errors, don't carry a target there and aren't gated.
var gatedEvents = map[string]bool{
	girc.PRIVMSG: true,
	girc.NOTICE:  true,
	girc.JOIN:    true,
	girc.MODE:    true,
}

// eventTarget returns where a gated event happened. ok is false for events the gate ignores.
func eventTarget(e *girc.Event) (target string, ok bool) {
	if e == nil || !gatedEvents[e.Command] {
		return "", false
	}
	if len(e.Params) == 0 {
		// A gated event with no target is malformed; give it a target that fails the check.
		return "", true
	}
	return e.Params[0], true
}

// ChannelGate is the first check every IRC event passes, before a session, lock or behaviour
// sees it (A13). Events from other channels are dropped whole, so nothing can be said there.
//
// It remembers which channels it has already logged and left, per connection: a busy channel
// then logs once instead of on every line, and a server that re-joins the bot can't cause a
// PART/JOIN loop.
type ChannelGate struct {
	log    *slog.Logger
	mu     sync.Mutex
	logged map[string]bool
	parted map[string]bool
}

// NewChannelGate makes a gate for one network connection.
func NewChannelGate(log *slog.Logger) *ChannelGate {
	g := &ChannelGate{log: log}
	g.Reset()
	return g
}

// Reset forgets which channels were logged and left. Call it before each (re)connect, so a
// fresh connection that gets auto-joined again is handled again.
func (g *ChannelGate) Reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.logged = map[string]bool{}
	g.parted = map[string]bool{}
}

// Admit reports whether the event may be processed. For an event somewhere the bot isn't
// allowed it returns false, and, when partunlisted is on and the bot itself has just joined
// that channel, calls part once for this connection.
func (g *ChannelGate) Admit(cfg *config.Configuration, e *girc.Event, botNick string, part func(channel string)) bool {
	target, gated := eventTarget(e)
	if !gated || ChannelAllowed(cfg, target) {
		return true
	}

	key := girc.ToRFC1459(target)
	botJoined := e.Command == girc.JOIN && e.Source != nil &&
		girc.ToRFC1459(e.Source.Name) == girc.ToRFC1459(botNick)

	g.mu.Lock()
	firstSeen := !g.logged[key]
	g.logged[key] = true
	leave := cfg.Bot.PartUnlisted && botJoined && girc.IsValidChannel(target) && !g.parted[key]
	if leave {
		g.parted[key] = true
	}
	g.mu.Unlock()

	if firstSeen {
		g.log.Info("channel_not_allowed", "target", target, "event", e.Command, "allowed", cfg.Server.Channel)
	}
	if leave {
		g.log.Info("channel_parting", "channel", target)
		part(target)
	}
	return false
}
