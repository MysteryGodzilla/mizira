// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"strings"
	"sync"

	"B4reMetal/metald/internal/config"
)

// botOwners maps a tagged bot ("network/metalai") to the nick that last posted its lines: a bot
// that shares its owner's nick has no nick of its own to slap or mention.
var botOwners sync.Map

func rememberBotOwner(cfg *config.Configuration, nick, text string) {
	if name := strings.ToLower(botName(BotTag(cfg, text))); name != "" && nick != "" {
		botOwners.Store(cfg.Server.Name+"/"+name, nick)
	}
}

// BotOwner returns the nick whose tagged lines come from the bot called name ("metalai" or
// "[metalai]"), or "" when that bot hasn't spoken since startup.
func BotOwner(cfg *config.Configuration, name string) string {
	name = strings.ToLower(botName(name))
	for _, p := range cfg.Bot.BotPrefixes {
		if strings.ToLower(botName(p)) == name && name != "" {
			if nick, ok := botOwners.Load(cfg.Server.Name + "/" + name); ok {
				return nick.(string)
			}
		}
	}
	return ""
}

// isBotTagName reports whether word names one of the botprefixes bots: "metalai", "[metalai]".
func isBotTagName(cfg *config.Configuration, word string) bool {
	name := strings.ToLower(botName(word))
	for _, p := range cfg.Bot.BotPrefixes {
		if name != "" && strings.ToLower(botName(p)) == name {
			return true
		}
	}
	return false
}
