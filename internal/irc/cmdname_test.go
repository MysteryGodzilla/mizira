// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"reflect"
	"testing"

	"B4reMetal/metald/internal/config"
)

func nameConfig(needName bool, trigger string) *config.Configuration {
	return &config.Configuration{Bot: &config.BotConfig{CommandPrefix: "+", Trigger: trigger, CommandsNeedName: needName}}
}

func TestCommandWords(t *testing.T) {
	tests := []struct {
		name     string
		needName bool
		trigger  string
		text     string
		words    []string
		command  bool
	}{
		{"named command", true, "Mizira", "Mizira +memories", []string{"+memories"}, true},
		{"named with colon", true, "Mizira", "Mizira: +stop now", []string{"+stop", "now"}, true},
		{"named with comma, other case", true, "Mizira", "mizira, +recall jeff", []string{"+recall", "jeff"}, true},
		{"bare command refused", true, "Mizira", "+memories", []string{"+memories"}, false},
		{"other bot's name", true, "Mizira", "metalai +memories", []string{"metalai", "+memories"}, false},
		{"name after the command", true, "Mizira", "+memories mizira", []string{"+memories", "mizira"}, false},
		{"name then chat", true, "Mizira", "Mizira hello there", []string{"Mizira", "hello", "there"}, false},
		{"name alone", true, "Mizira", "Mizira", []string{"Mizira"}, false},
		{"no trigger: the nick is the name", true, "", "testbot +help", []string{"+help"}, true},
		{"setting off: bare works", false, "Mizira", "+memories", []string{"+memories"}, true},
		{"setting off: named still works", false, "Mizira", "Mizira +memories", []string{"+memories"}, true},
		{"empty", true, "Mizira", "", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			words, ok := CommandWords(nameConfig(tt.needName, tt.trigger), "testbot", tt.text)
			if ok != tt.command || (len(words) > 0 || len(tt.words) > 0) && !reflect.DeepEqual(words, tt.words) {
				t.Errorf("CommandWords(%q) = %q, %v; want %q, %v", tt.text, words, ok, tt.words, tt.command)
			}
		})
	}
}

// A line that mentions the bot but isn't a named command must not run the "+word" it starts
// with: it is addressed (so it reaches the dispatcher), and GetCommand is what the dispatcher
// trusts.
func TestGetCommandRespectsTheNameRule(t *testing.T) {
	cfg := nameConfig(true, "Mizira")
	args, isCmd := CommandWords(cfg, "testbot", "+memories mizira")
	c := ChatContext{Config: cfg, args: args, isCommand: isCmd}
	if got := c.GetCommand(); got != "" {
		t.Fatalf("GetCommand() = %q, want no command", got)
	}
	args, isCmd = CommandWords(cfg, "testbot", "Mizira +memories")
	c = ChatContext{Config: cfg, args: args, isCommand: isCmd}
	if got := c.GetCommand(); got != "+memories" {
		t.Fatalf("GetCommand() = %q, want +memories", got)
	}
}
