// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"errors"
	"strings"
	"testing"
	"time"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

func TestShortDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{2 * time.Hour: "2h", 90 * time.Minute: "1h30m", 45 * time.Minute: "45m",
		30 * time.Second: "30s", 48 * time.Hour: "48h", 90 * time.Second: "1m30s"} {
		if got := shortDuration(d); got != want {
			t.Errorf("%v -> %q, want %q", d, got, want)
		}
	}
}

// Only an admin clears a score; it's gone afterwards.
func TestSuspicionClear(t *testing.T) {
	core.Suspicions().Add("", "mallory", core.SignalScreenDenied)
	t.Cleanup(func() { core.Suspicions().Clear("", "mallory") })

	ctx := mocktest.NewMockContext().WithArgs("+suspicion", "clear", "mallory")
	(&SuspicionCommand{}).Execute(ctx)
	if core.Suspicions().Score("", "mallory") == 0 || !strings.Contains(ctx.LastReply(), "only an admin") {
		t.Errorf("a non-admin cleared it: %q", ctx.LastReply())
	}
	ctx = mocktest.NewMockContext().WithAdmin(true).WithArgs("+suspicion", "clear", "mallory")
	(&SuspicionCommand{}).Execute(ctx)
	if core.Suspicions().Score("", "mallory") != 0 || !strings.Contains(ctx.LastReply(), "cleared") {
		t.Errorf("not cleared: %q", ctx.LastReply())
	}
}

// The bot can't be ignored under its configured nick, the alternate it is using now, or its trigger.
func TestCannotIgnoreTheBotByAnyName(t *testing.T) {
	cfg := sharedCfg(t)
	cfg.Server = &config.ServerConfig{Nick: "botty"}
	cfg.Bot.Trigger = "Bot"
	core.NoteBotNick("ignore-self", "botty_")
	for _, nick := range []string{"botty", "Botty_", "bot"} {
		if _, err := IgnoreNick(cfg, "ignore-self", "someone-else", nick, time.Hour, "", "alice", "", quiet); !errors.Is(err, ErrIsSelf) {
			t.Errorf("ignoring %s: %v", nick, err)
		}
	}
}
