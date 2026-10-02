// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"testing"
	"time"
)

func newTestBotLoop() (*BotLoopTracker, *time.Time) {
	clock := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	b := NewBotLoopTracker()
	b.now = func() time.Time { return clock }
	return b, &clock
}

func TestBotLoopStopsAtLimit(t *testing.T) {
	b, _ := newTestBotLoop()
	for i := 1; i <= 3; i++ {
		if ok, _ := b.Allow("net", "#chat", 3, 10*time.Minute); !ok {
			t.Fatalf("reply %d should be allowed under a limit of 3", i)
		}
	}
	ok, first := b.Allow("net", "#chat", 3, 10*time.Minute)
	if ok || !first {
		t.Fatalf("4th reply: ok=%v first=%v, want refused and logged", ok, first)
	}
	ok, first = b.Allow("net", "#chat", 3, 10*time.Minute)
	if ok || first {
		t.Fatalf("5th reply: ok=%v first=%v, want refused and not logged again", ok, first)
	}
}

func TestBotLoopHumanResets(t *testing.T) {
	b, _ := newTestBotLoop()
	for range 3 {
		b.Allow("net", "#chat", 3, 10*time.Minute)
	}
	b.HumanSpoke("net", "#CHAT") // channel names are case-insensitive
	if ok, _ := b.Allow("net", "#chat", 3, 10*time.Minute); !ok {
		t.Fatal("a human line should start a fresh chain")
	}
}

func TestBotLoopCooldownResets(t *testing.T) {
	b, clock := newTestBotLoop()
	for range 3 {
		b.Allow("net", "#chat", 3, 10*time.Minute)
	}
	*clock = clock.Add(9 * time.Minute)
	if ok, _ := b.Allow("net", "#chat", 3, 10*time.Minute); ok {
		t.Fatal("still inside the cooldown, the chain must stay capped")
	}
	*clock = clock.Add(2 * time.Minute) // 11 minutes since the last reply
	if ok, _ := b.Allow("net", "#chat", 3, 10*time.Minute); !ok {
		t.Fatal("after the cooldown the bots may chat again")
	}
}

// The cooldown counts from the last reply, so a bot that keeps pinging after the limit can't
// push it back and keep the chain capped forever.
func TestBotLoopCooldownCountsFromLastReply(t *testing.T) {
	b, clock := newTestBotLoop()
	for range 3 {
		b.Allow("net", "#chat", 3, 10*time.Minute)
	}
	for range 5 {
		*clock = clock.Add(time.Minute)
		b.Allow("net", "#chat", 3, 10*time.Minute) // refused pings
	}
	*clock = clock.Add(6 * time.Minute) // 11 minutes since the last reply
	if ok, _ := b.Allow("net", "#chat", 3, 10*time.Minute); !ok {
		t.Fatal("refused pings must not extend the cooldown")
	}
}

func TestBotLoopZeroLimitNeverReplies(t *testing.T) {
	b, _ := newTestBotLoop()
	if ok, _ := b.Allow("net", "#chat", 0, 10*time.Minute); ok {
		t.Fatal("botreplylimit 0 means never reply to bots")
	}
}

func TestBotLoopScopedByNetworkAndChannel(t *testing.T) {
	b, _ := newTestBotLoop()
	b.Allow("net1", "#chat", 1, 10*time.Minute)
	if ok, _ := b.Allow("net2", "#chat", 1, 10*time.Minute); !ok {
		t.Error("another network's chain must not count")
	}
	if ok, _ := b.Allow("net1", "#test", 1, 10*time.Minute); !ok {
		t.Error("another channel's chain must not count")
	}
}
