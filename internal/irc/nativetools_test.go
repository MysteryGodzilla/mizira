// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"slices"
	"testing"
)

func TestNativeToolNamesAreTheBotsOwn(t *testing.T) {
	names := NativeToolNames()
	if !slices.IsSorted(names) || !slices.Contains(names, "irc__slap") || !slices.Contains(names, "task__start") {
		t.Errorf("names = %v", names)
	}
	// polly's own "bash" native runs commands; it must never be offered.
	if slices.Contains(names, "bash") {
		t.Error("bash is not one of the bot's tools")
	}
}

func TestNativeToolDescription(t *testing.T) {
	if NativeToolDescription("irc__slap") == "" || NativeToolDescription("bash") != "" {
		t.Error("descriptions come from the bot's own tools only")
	}
}
