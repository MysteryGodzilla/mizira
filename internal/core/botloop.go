// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"strings"
	"sync"
	"time"
)

// BotLoopTracker counts how many times in a row the bot has replied to other bots in each
// channel.
//
// A10: bots chatting is welcome, but two bots that keep addressing each other would never stop
// on their own. The count resets when a human speaks, or after a quiet cooldown, so bots can
// chat in bursts but never loop.
type BotLoopTracker struct {
	mu     sync.Mutex
	chains map[string]*botChain
	now    func() time.Time // swapped in tests
}

type botChain struct {
	replies   int
	lastReply time.Time
	refused   bool // the limit was already logged for this chain
}

var (
	globalBotLoop     *BotLoopTracker
	globalBotLoopOnce sync.Once
)

// BotLoop returns the process-wide bot loop tracker.
func BotLoop() *BotLoopTracker {
	globalBotLoopOnce.Do(func() {
		globalBotLoop = NewBotLoopTracker()
	})
	return globalBotLoop
}

func NewBotLoopTracker() *BotLoopTracker {
	return &BotLoopTracker{chains: make(map[string]*botChain), now: time.Now}
}

// Allow is asked before replying to a bot line in channel. It counts the reply and returns
// true while the chain is under limit. firstRefusal is true only the first time a chain hits
// the limit, so the caller logs it once instead of on every bot line.
func (b *BotLoopTracker) Allow(network, channel string, limit int, cooldown time.Duration) (ok, firstRefusal bool) {
	key := ScopeKey(network, strings.ToLower(channel))
	now := b.now()

	b.mu.Lock()
	defer b.mu.Unlock()

	c := b.chains[key]
	if c == nil {
		c = &botChain{}
		b.chains[key] = c
	}
	if c.replies > 0 && now.Sub(c.lastReply) >= cooldown {
		*c = botChain{}
	}
	if c.replies >= limit {
		first := !c.refused
		c.refused = true
		return false, first
	}
	c.replies++
	c.lastReply = now
	return true, false
}

// HumanSpoke resets the chain for channel: a person is in the conversation again.
func (b *BotLoopTracker) HumanSpoke(network, channel string) {
	key := ScopeKey(network, strings.ToLower(channel))
	b.mu.Lock()
	defer b.mu.Unlock()
	// Deleting rather than zeroing keeps the map from growing with channels nobody uses.
	delete(b.chains, key)
}
