// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import "testing"

func TestCurrentBotNick(t *testing.T) {
	NoteBotNick("nicknet", "Botty_")
	t.Cleanup(func() { botNicks.Delete("nicknet") })
	if !IsCurrentBotNick("botty_") || IsCurrentBotNick("botty") {
		t.Error("current nick not recognised, or the wrong one was")
	}
	NoteBotNick("nicknet", "")
	if !IsCurrentBotNick("Botty_") {
		t.Error("an empty nick replaced the current one")
	}
}
