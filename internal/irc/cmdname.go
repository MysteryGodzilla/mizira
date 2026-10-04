// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"strings"
	"unicode"

	"github.com/lrstanley/girc"

	"B4reMetal/metald/internal/config"
)

// CommandWords splits a message into the words a command sees, and reports whether it is a
// command for this bot.
//
// With commandsneedname on, a command must start with the bot's name: "Mizira +memories" or
// "Mizira: +stop". The name is dropped, so the command sees "+memories" first as usual. A bare
// "+memories" is left alone: other bots in the channel (often built on the same code, with the
// same "+" prefix) would answer it too, and one line should not trigger every bot.
func CommandWords(cfg *config.Configuration, botNick, text string) (words []string, isCommand bool) {
	words = strings.Fields(plainText(text))
	prefix := cfg.Bot.CommandPrefix
	if len(words) >= 2 && isBotName(words[0], cfg.Bot.Trigger, botNick) && CanonicalCommand(words[1], prefix) != "" {
		return words[1:], true
	}
	if len(words) >= 1 && CanonicalCommand(words[0], prefix) != "" && !cfg.Bot.CommandsNeedName {
		return words, true
	}
	return words, false
}

// plainText drops IRC formatting codes and invisible characters (zero-width spaces, joiners,
// direction marks) that some clients add when completing a nick, so "Mizira +reset" is read the
// same however it was typed.
func plainText(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, girc.StripRaw(s))
}

// isBotName reports whether word is the bot's name (its trigger, or its nick when there is no
// trigger), allowing the usual "Name:" or "Name," that IRC clients add.
func isBotName(word, trigger, botNick string) bool {
	name := trigger
	if name == "" {
		name = botNick
	}
	word = strings.TrimRight(word, ":,")
	return name != "" && girc.ToRFC1459(word) == girc.ToRFC1459(name)
}
