// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"maps"
	"slices"
)

// NativeToolNames are the bot's own tools, sorted: what the operator console offers to switch on
// besides plugins.
func NativeToolNames() []string {
	return slices.Sorted(maps.Keys(nativeFactories))
}

// NativeToolDescription is what a tool of the bot's own tells the model it does, loaded or not.
func NativeToolDescription(name string) string {
	f, ok := nativeFactories[name]
	if !ok {
		return ""
	}
	if s := f().GetSchema(); s != nil {
		return s.Description()
	}
	return ""
}
