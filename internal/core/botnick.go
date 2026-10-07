// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"strings"
	"sync"
)

// The bot's current nick on each network, which differs from the configured one when that was taken
// and it fell back to an alternate.
var botNicks sync.Map // network -> nick

// NoteBotNick records the nick the bot is using on a network now.
func NoteBotNick(network, nick string) {
	if nick != "" {
		botNicks.Store(network, nick)
	}
}

// IsCurrentBotNick reports whether nick is the bot's current nick on any network.
func IsCurrentBotNick(nick string) bool {
	found := false
	botNicks.Range(func(_, v any) bool {
		found = strings.EqualFold(v.(string), nick)
		return !found
	})
	return found
}
