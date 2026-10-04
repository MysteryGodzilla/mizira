// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func sharedCfg(t *testing.T) *config.Configuration {
	t.Helper()
	original := OverridesPath
	OverridesPath = filepath.Join(t.TempDir(), "config-overrides.json")
	t.Cleanup(func() { OverridesPath = original })
	return &config.Configuration{Bot: &config.BotConfig{Admins: []string{"alice!*@*"}}, Model: &config.ModelConfig{}}
}

func TestIgnoreNickRefusesAdminsAndTheBot(t *testing.T) {
	cfg := sharedCfg(t)
	if _, err := IgnoreNick(cfg, "net", "Mizira", "Alice", time.Hour, "", "console:token", "", quiet); !errors.Is(err, ErrIsAdmin) {
		t.Errorf("admin: %v", err)
	}
	if _, err := IgnoreNick(cfg, "net", "Mizira", "mizira", time.Hour, "", "console:token", "", quiet); !errors.Is(err, ErrIsSelf) {
		t.Errorf("self: %v", err)
	}
	until, err := IgnoreNick(cfg, "net", "Mizira", "mallory", time.Hour, "spam", "console:token", "", quiet)
	if err != nil || time.Until(until) < 59*time.Minute || !core.Ignores().IsIgnored("net", "mallory") {
		t.Fatalf("ignore: %v %v", until, err)
	}
	if !UnignoreNick("net", "mallory", "console:token", quiet) || UnignoreNick("net", "mallory", "console:token", quiet) {
		t.Error("unignore should succeed once")
	}
}

func TestScreenNickSharesTheCommandsRules(t *testing.T) {
	cfg := sharedCfg(t)
	if _, err := ScreenNick(cfg, nil, "Mizira", "alice", "console:token", quiet); !errors.Is(err, ErrIsAdmin) {
		t.Errorf("admin: %v", err)
	}
	if _, err := ScreenNick(cfg, nil, "Mizira", "eve", "console:token", quiet); err != nil {
		t.Fatal(err)
	}
	if _, err := ScreenNick(cfg, nil, "Mizira", "EVE", "console:token", quiet); !errors.Is(err, ErrAlreadyScreened) {
		t.Errorf("twice: %v", err)
	}
	fresh := &config.Configuration{Bot: &config.BotConfig{}, Model: &config.ModelConfig{}}
	ApplyOverrides(fresh)
	if len(fresh.Bot.ScreenNicks) != 1 || len(fresh.Bot.FilterNicks) != 1 {
		t.Errorf("not persisted: %v %v", fresh.Bot.ScreenNicks, fresh.Bot.FilterNicks)
	}
	if !UnscreenNick(cfg, "eve", "console:token", quiet) || UnscreenNick(cfg, "eve", "console:token", quiet) {
		t.Error("unscreen should succeed once")
	}
}

// The console and the commands change the same lists from different goroutines.
func TestScreeningFromTwoWritersKeepsEveryNick(t *testing.T) {
	cfg := sharedCfg(t)
	nicks := []string{"bob", "carol", "dave", "eve", "mallory", "trent", "peggy", "victor"}
	var wg sync.WaitGroup
	for _, n := range nicks {
		wg.Go(func() { _, _ = ScreenNick(cfg, nil, "Mizira", n, "console:token", quiet) })
	}
	wg.Wait()
	if len(cfg.Bot.ScreenNicks) != len(nicks) || len(cfg.Bot.FilterNicks) != len(nicks) {
		t.Errorf("lost a nick: %v", cfg.Bot.ScreenNicks)
	}
}

func TestOperatorMemoriesKeepTheCapAndMerge(t *testing.T) {
	cfg := sharedCfg(t)
	cfg.Server = &config.ServerConfig{Channel: "#test"}
	cfg.Bot.MemoryPerSubject = 2
	store, _ := core.Memories()
	t.Cleanup(func() { store.ForgetSubject("console-net", "carol") })

	id, merged, err := OperatorRemember(cfg, "console-net", "carol", "carol has a rat called Pip", "console:token", quiet)
	if err != nil || merged {
		t.Fatalf("add: %v %v", err, merged)
	}
	if again, merged, _ := OperatorRemember(cfg, "console-net", "carol", "Pip is carol's rat", "console:token", quiet); !merged || again != id {
		t.Errorf("repeat: id %d merged %v", again, merged)
	}
	if _, _, err := OperatorRemember(cfg, "console-net", "carol", "carol plays go", "console:token", quiet); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OperatorRemember(cfg, "console-net", "carol", "carol lives in Osaka", "console:token", quiet); err == nil {
		t.Error("a third fact past a cap of 2 was saved")
	}
	if err := EditMemory("console-net", id, "carol has a pet rat called Pip", "console:token", quiet); err != nil {
		t.Fatal(err)
	}
	if err := EditMemory("other-net", id, "x", "console:token", quiet); !errors.Is(err, ErrNoSuchMemory) {
		t.Errorf("edit on another network: %v", err)
	}
	if err := ForgetMemory("console-net", id, "console:token", quiet); err != nil {
		t.Fatal(err)
	}
	if err := ForgetMemory("console-net", id, "console:token", quiet); !errors.Is(err, ErrNoSuchMemory) {
		t.Errorf("forget twice: %v", err)
	}
}
