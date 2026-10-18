// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/alexschlessinger/pollytool/messages"
	"github.com/alexschlessinger/pollytool/tools"

	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
	mocktest "B4reMetal/metald/internal/testing"
)

// announcer writes a plugin whose --announce prints out, or fails when out is empty.
func announcer(t *testing.T, out string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "tool.sh")
	body := "#!/bin/sh\n[ -n '" + out + "' ] && echo '" + out + "' && exit 0\nexit 1\n"
	if runtime.GOOS == "windows" {
		p = filepath.Join(t.TempDir(), "tool.cmd")
		body = "@echo off\r\nif not \"" + out + "\"==\"\" (echo " + out + "& exit /b 0)\r\nexit /b 1\r\n"
	}
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func startTool(name string) []string {
	ctx := mocktest.NewMockContext()
	cfg := ctx.GetConfig()
	cfg.Bot.ShowToolActions = true
	h := newCallbackHandler(ctx, irc.NewChunker(make(chan string, 10), 350), cfg)
	h.onToolStart([]messages.ChatMessageToolCall{{Name: name}})
	h.onToolStart([]messages.ChatMessageToolCall{{Name: name}})
	return ctx.Actions
}

func TestOnToolStart_UsesTheToolsAnnouncement(t *testing.T) {
	core.SetAnnouncer("drawtest__picture", announcer(t, "action: is drawing a picture"))
	got := startTool("drawtest__picture")
	if len(got) != 1 || got[0] != "is drawing a picture" {
		t.Fatalf("actions = %q, want one tool announcement", got)
	}
}

func TestOnToolStart_FailedAnnouncementFallsBackToCalling(t *testing.T) {
	core.SetAnnouncer("brokentest__picture", announcer(t, ""))
	core.SetAnnouncer("plaintest__picture", announcer(t, "no action prefix"))
	for name, want := range map[string]string{"brokentest__picture": "calling picture", "plaintest__picture": "calling picture"} {
		if got := startTool(name); len(got) != 1 || got[0] != want {
			t.Errorf("%s: actions = %q, want %q", name, got, want)
		}
	}
}

type namedTool struct {
	tools.Tool
	name string
}

func (n namedTool) GetName() string { return n.name }

// An admin-only tool called for someone who isn't an admin only refuses, so nothing is announced.
func TestOnToolStart_AdminOnlyToolIsNotAnnouncedForOthers(t *testing.T) {
	core.SetAnnouncer("admintest__picture", announcer(t, "action: is drawing a picture"))
	for _, admin := range []bool{false, true} {
		sys := mocktest.NewMockSystem()
		sys.ToolRegistry.Register(irc.NewAdminOnlyTool(namedTool{name: "admintest__picture"}))
		ctx := mocktest.NewMockContext().WithSystem(sys).WithAdmin(admin)
		cfg := ctx.GetConfig()
		cfg.Bot.ShowToolActions = true
		h := newCallbackHandler(ctx, irc.NewChunker(make(chan string, 10), 350), cfg)
		h.onToolStart([]messages.ChatMessageToolCall{{Name: "admintest__picture"}})
		if want := map[bool]int{false: 0, true: 1}[admin]; len(ctx.Actions) != want {
			t.Errorf("admin=%v: actions = %q, want %d", admin, ctx.Actions, want)
		}
	}
}

// A reply that talks about the picture but leaves out its link still gets the link posted.
func TestOnComplete_ReplyWithoutTheToolURLGetsIt(t *testing.T) {
	ctx := mocktest.NewMockContext()
	output := make(chan string, 10)
	chunker := irc.NewChunker(output, 350)
	h := newCallbackHandler(ctx, chunker, ctx.GetConfig())

	h.onToolEnd(messages.ChatMessageToolCall{Name: "imagegen__picture"}, "url: https://files.example.com/cat.png", time.Millisecond, nil)
	h.onContent("here you go, a sleepy cat!")
	h.onComplete(&messages.ChatMessage{})
	chunker.Flush()
	close(output)

	var got []string
	for line := range output {
		got = append(got, line)
	}
	if len(got) != 2 || got[1] != "https://files.example.com/cat.png" {
		t.Fatalf("want the reply then the link, got %q", got)
	}
}

// Another bot's call to a people-only tool only refuses, so nothing is announced.
func TestOnToolStart_PeopleOnlyToolIsNotAnnouncedForBots(t *testing.T) {
	core.SetAnnouncer("botstest__picture", announcer(t, "action: is drawing a picture"))
	sys := mocktest.NewMockSystem()
	sys.ToolRegistry.Register(irc.NewPeopleOnlyTool(namedTool{name: "botstest__picture"}))
	ctx := mocktest.NewMockContext().WithSystem(sys)
	ctx.BotLine = true
	cfg := ctx.GetConfig()
	cfg.Bot.ShowToolActions = true
	h := newCallbackHandler(ctx, irc.NewChunker(make(chan string, 10), 350), cfg)
	h.onToolStart([]messages.ChatMessageToolCall{{Name: "botstest__picture"}})
	if len(ctx.Actions) != 0 {
		t.Fatalf("announced for a bot: %q", ctx.Actions)
	}
}
