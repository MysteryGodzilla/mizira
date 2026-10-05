// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/alexschlessinger/pollytool/messages"
	"github.com/alexschlessinger/pollytool/sessions"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

func settingsCfg(t *testing.T) *config.Configuration {
	t.Helper()
	sharedCfg(t) // a private overrides file
	cfg := mocktest.DefaultTestConfig()
	cfg.API.OpenAIKey = "sk-secret-value"
	ApplyOverrides(cfg)
	return cfg
}

func TestSettingsNeverListCredentials(t *testing.T) {
	cfg := settingsCfg(t)
	for _, s := range Settings(cfg) {
		if strings.HasSuffix(s.Key, "key") || strings.Contains(s.Value, "sk-") || strings.Contains(s.Default, "sk-") {
			t.Errorf("listed a credential: %+v", s)
		}
	}
	for _, key := range []string{"openaikey", "anthropickey", "geminikey", "ollamakey"} {
		if _, err := SetSetting(cfg, nil, key, "x", "console:token", quiet); !errors.Is(err, ErrUnknownSetting) {
			t.Errorf("set %s: %v", key, err)
		}
		if _, err := ResetSetting(cfg, nil, key, "console:token", quiet); !errors.Is(err, ErrUnknownSetting) {
			t.Errorf("reset %s: %v", key, err)
		}
	}
	if cfg.API.OpenAIKey != "sk-secret-value" {
		t.Fatal("a credential changed")
	}
}

func TestSetAndResetASetting(t *testing.T) {
	cfg := settingsCfg(t)
	def := cfg.Bot.MaxReplyLines
	if got, err := SetSetting(cfg, nil, "maxreplylines", "7", "console:token", quiet); err != nil || got != "7" || cfg.Bot.MaxReplyLines != 7 {
		t.Fatalf("set: %q %v", got, err)
	}
	if _, err := SetSetting(cfg, nil, "maxreplylines", "999", "console:token", quiet); err == nil || cfg.Bot.MaxReplyLines != 7 {
		t.Errorf("an invalid value was accepted: %v", err)
	}
	i := slices.IndexFunc(Settings(cfg), func(s Setting) bool { return s.Key == "maxreplylines" })
	if s := Settings(cfg)[i]; !s.Overridden || s.Value != "7" {
		t.Errorf("after set: %+v", s)
	}

	fresh := mocktest.DefaultTestConfig()
	ApplyOverrides(fresh)
	if fresh.Bot.MaxReplyLines != 7 {
		t.Errorf("not persisted: %d", fresh.Bot.MaxReplyLines)
	}

	if _, err := ResetSetting(fresh, nil, "maxreplylines", "console:token", quiet); err != nil || fresh.Bot.MaxReplyLines != def {
		t.Fatalf("reset: %v, now %d, want %d", err, fresh.Bot.MaxReplyLines, def)
	}
	again := mocktest.DefaultTestConfig()
	ApplyOverrides(again)
	if again.Bot.MaxReplyLines != def {
		t.Errorf("the reset didn't drop the override: %d", again.Bot.MaxReplyLines)
	}
}

func TestThePromptIsReadOnlyFromTheConsole(t *testing.T) {
	cfg := settingsCfg(t)
	if _, err := SetSetting(cfg, nil, "prompt", "be a pirate", "console:token", quiet); !errors.Is(err, ErrNotEditable) {
		t.Errorf("prompt: %v", err)
	}
}

func TestToolSwitchesLayerOverConfig(t *testing.T) {
	sharedCfg(t)
	cfg := mocktest.DefaultTestConfig()
	cfg.Bot.Tools = []string{"irc__slap", "memory__recall"}
	ApplyOverrides(cfg)

	PersistToolSwitch("history__search", true)
	PersistToolSwitch("irc__slap", false)
	PersistToolSwitch("memory__recall", true) // already in config.yml: nothing to record
	on, off := ToolOverrides()
	if !slices.Equal(on, []string{"history__search"}) || !slices.Equal(off, []string{"irc__slap"}) {
		t.Fatalf("on %v off %v", on, off)
	}

	restarted := mocktest.DefaultTestConfig()
	restarted.Bot.Tools = []string{"irc__slap", "memory__recall"}
	ApplyOverrides(restarted)
	if !slices.Equal(restarted.Bot.Tools, []string{"memory__recall", "history__search"}) {
		t.Errorf("after a restart: %v", restarted.Bot.Tools)
	}
	if !slices.Equal(ConfigTools(), []string{"irc__slap", "memory__recall"}) {
		t.Errorf("config.yml's list: %v", ConfigTools())
	}

	PersistToolSwitch("irc__slap", true) // back to config.yml's state
	if _, off := ToolOverrides(); len(off) != 0 {
		t.Errorf("switching back still recorded: %v", off)
	}
	ClearToolSwitches()
	if on, off := ToolOverrides(); len(on)+len(off) != 0 {
		t.Errorf("after clear: %v %v", on, off)
	}
}

// ~set wipes the conversation only for a new model or prompt; a new prompt reaches it either way.
func TestSetKeepsHistoryExceptModelAndPrompt(t *testing.T) {
	cfg := settingsCfg(t)
	db, err := core.Context()
	if err != nil {
		t.Fatal(err)
	}
	sys := mocktest.NewMockSystem()
	sys.SessionStore = core.NewPersistentSessionStore(db, &sessions.Metadata{SystemPrompt: cfg.Bot.Prompt})
	session, _ := sys.SessionStore.Get("net/#setkeeps")
	session.AddMessage(messages.ChatMessage{Role: messages.MessageRoleUser, Content: "(nick:bob) hi"})
	run := func(args ...string) {
		ctx := mocktest.NewMockContext().WithConfig(cfg).WithSystem(sys).WithSession(session).WithAdmin(true).WithArgs(args...)
		(&SetCommand{}).Execute(ctx)
	}

	run("+set", "maxreplylines", "7")
	if len(session.GetHistory()) != 2 {
		t.Errorf("history after ~set maxreplylines = %d messages, want it kept", len(session.GetHistory()))
	}
	run("+set", "prompt", "you are a test bot, version two")
	h := session.GetHistory()
	if len(h) != 1 || h[0].Content != "you are a test bot, version two" {
		t.Errorf("history after ~set prompt = %+v, want just the new prompt", h)
	}
}
