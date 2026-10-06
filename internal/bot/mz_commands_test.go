// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package bot

import (
	"testing"

	"B4reMetal/metald/internal/commands"
)

// A command added without a cheatsheet entry would show up on the console with no description.
func TestEveryCommandIsOnTheCheatsheet(t *testing.T) {
	if missing := commands.MissingHelp(newCommandRegistry()); len(missing) > 0 {
		t.Errorf("no cheatsheet entry (internal/commands/cheatsheet.go) for: %v", missing)
	}
}
