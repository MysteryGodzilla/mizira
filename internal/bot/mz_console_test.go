// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package bot

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/alexschlessinger/pollytool/messages"

	"B4reMetal/metald/internal/admin"
	"B4reMetal/metald/internal/commands"
	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/irc"
	"B4reMetal/metald/internal/llm"
	mocktest "B4reMetal/metald/internal/testing"
)

func TestConsoleThinking(t *testing.T) {
	for effort, on := range map[string]bool{"": false, "off": false, "none": false, "None": false, "low": true, "high": true} {
		c := console{cfg: &config.Configuration{Model: &config.ModelConfig{ThinkingEffort: effort}}}
		if c.Thinking() != on {
			t.Errorf("thinkingeffort %q: Thinking() = %v, want %v", effort, !on, on)
		}
	}
}

func toolConsole(t *testing.T, tools ...string) console {
	t.Helper()
	original := commands.OverridesPath
	commands.OverridesPath = filepath.Join(t.TempDir(), "config-overrides.json")
	t.Cleanup(func() { commands.OverridesPath = original })
	cfg := mocktest.DefaultTestConfig()
	cfg.Bot.Tools = tools
	cfg.Bot.PluginLib = filepath.Join(t.TempDir(), "plugins", "lib")
	commands.ApplyOverrides(cfg)
	sys := mocktest.NewMockSystem()
	irc.RegisterIRCTools(sys.ToolRegistry)
	sys.ToolRegistry.RegisterNative("task__delegate", llm.NewDelegateTool) // as NewSystem does
	for _, spec := range tools {
		if _, err := sys.ToolRegistry.LoadToolAuto(spec); err != nil {
			t.Fatal(err)
		}
	}
	return console{cfg: cfg, sys: sys}
}

func TestSwitchingAToolLoadsItNowAndAfterARestart(t *testing.T) {
	c := toolConsole(t, "irc__slap")
	if err := c.SwitchTool("history__search", true, "console:token"); err != nil {
		t.Fatal(err)
	}
	if err := c.SwitchTool("irc__slap", false, "console:token"); err != nil {
		t.Fatal(err)
	}
	if got := c.Tools(); !slices.Equal(got, []string{"history__search"}) {
		t.Errorf("loaded now: %v", got)
	}
	cat := c.ToolCatalog()
	slap := cat[slices.IndexFunc(cat, func(v admin.ToolView) bool { return v.Spec == "irc__slap" })]
	if slap.Loaded || !slap.InConfig || !slap.Switched {
		t.Errorf("irc__slap in the catalog: %+v", slap)
	}

	restarted := mocktest.DefaultTestConfig()
	restarted.Bot.Tools = []string{"irc__slap"}
	commands.ApplyOverrides(restarted)
	if !slices.Equal(restarted.Bot.Tools, []string{"history__search"}) {
		t.Errorf("after a restart the list is %v", restarted.Bot.Tools)
	}

	if err := c.ResetToolSwitches("console:token"); err != nil {
		t.Fatal(err)
	}
	if got := c.Tools(); !slices.Equal(got, []string{"irc__slap"}) {
		t.Errorf("after reset: %v", got)
	}
}

func TestBackgroundWorkIsOneSwitch(t *testing.T) {
	c := toolConsole(t)
	if err := c.SwitchTool("task__start", true, "console:token"); err != nil {
		t.Fatal(err)
	}
	got := c.Tools()
	slices.Sort(got)
	want := slices.Sorted(slices.Values(irc.TaskToolset))
	if !slices.Equal(got, want) {
		t.Errorf("work on: %v, want %v", got, want)
	}
	if err := c.SwitchTool("task__start", false, "console:token"); err != nil || len(c.Tools()) != 0 {
		t.Errorf("work off: %v %v", err, c.Tools())
	}
}

func TestOnlyOfferedToolsCanBeSwitched(t *testing.T) {
	c := toolConsole(t)
	for _, spec := range []string{"bash", "/bin/sh", "../plugins/x.py", "task__note"} {
		if err := c.SwitchTool(spec, true, "console:token"); err == nil {
			t.Errorf("%s was switched on", spec)
		}
	}
	if len(c.Tools()) != 0 {
		t.Errorf("loaded: %v", c.Tools())
	}
}

func TestAdminToolsStayRestrictedWhenSwitchedOn(t *testing.T) {
	c := toolConsole(t)
	c.cfg.Bot.AdminTools = []string{"irc__slap"}
	if err := c.SwitchTool("irc__slap", true, "console:token"); err != nil {
		t.Fatal(err)
	}
	tool, _ := c.sys.GetToolRegistry().Get("irc__slap")
	if !irc.IsAdminOnly(tool) {
		t.Error("an admin tool switched on unrestricted")
	}
}

// A fresh conversation is 0 messages: the system prompt isn't one, as the fold doesn't count it.
func TestConversationCountLeavesOutThePrompt(t *testing.T) {
	cfg := mocktest.DefaultTestConfig()
	n := &config.ServerConfig{Name: "net", Channel: "#count"}
	sys := mocktest.NewMockSystem()
	c := console{cfg: cfg, sys: sys, nets: []*config.ServerConfig{n}}
	s, _ := sys.GetSessionStore().Get("net/#count")
	if v := c.Conversations()[0]; v.Messages != 0 || v.Tokens != 0 {
		t.Errorf("fresh: %d messages, %d tokens", v.Messages, v.Tokens)
	}
	s.AddMessage(messages.ChatMessage{Role: messages.MessageRoleUser, Content: "(nick:bob) hi"})
	if v := c.Conversations()[0]; v.Messages != 1 {
		t.Errorf("after one line: %d messages", v.Messages)
	}
}

// Restricting from the console is ~tools restrict: the loaded tool becomes admin-only, and stays so
// after a restart; an unloaded one is refused.
func TestRestrictToolFromTheConsole(t *testing.T) {
	c := toolConsole(t, "irc__slap")
	if err := c.RestrictTool("irc__slap", true, "console:token"); err != nil {
		t.Fatal(err)
	}
	tool, _ := c.sys.GetToolRegistry().Get("irc__slap")
	if !irc.IsAdminOnly(tool) || !slices.Contains(c.cfg.Bot.AdminTools, "irc__slap") {
		t.Errorf("not restricted: admin tools %v", c.cfg.Bot.AdminTools)
	}
	fresh := mocktest.DefaultTestConfig()
	commands.ApplyOverrides(fresh)
	if !slices.Contains(fresh.Bot.AdminTools, "irc__slap") {
		t.Errorf("not kept across a restart: %v", fresh.Bot.AdminTools)
	}
	if err := c.RestrictTool("irc__slap", false, "console:token"); err != nil {
		t.Fatal(err)
	}
	if tool, _ := c.sys.GetToolRegistry().Get("irc__slap"); irc.IsAdminOnly(tool) {
		t.Error("still admin-only after unrestricting")
	}
	if err := c.RestrictTool("history__search", true, "console:token"); err == nil {
		t.Error("restricted a tool that isn't loaded")
	}
}
