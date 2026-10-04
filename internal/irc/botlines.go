// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/lrstanley/girc"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
)

// LineKind says who wrote a channel line.
type LineKind int

const (
	// HumanLine is anything that doesn't start with a known bot prefix.
	HumanLine LineKind = iota
	// BotLine comes from a nick in botnicks, or starts with one of the botprefixes: another
	// bot is talking.
	BotLine
	// OwnLine starts with this bot's own responseprefix: it is reading its own words back.
	OwnLine
)

// minBotPrefixLen is the shortest prefix accepted, so nobody can mark ordinary chat as bot
// talk by accident (a prefix like "[" would match half the channel).
const minBotPrefixLen = 3

// ClassifyLine reports whether a line from nick was written by a human, another bot, or
// this bot.
//
// A10: a bot is recognised in one of two ways. Bots with their own account have their own
// nick (botnicks). Bots that share their owner's nick tag their lines with a prefix such as
// "[metalai]" (botprefixes), which is then the only reliable sign. Colours and other
// formatting are stripped first, since a bot may colour its tag.
func ClassifyLine(cfg *config.Configuration, nick, text string) LineKind {
	line := normaliseLine(text)
	if own := normaliseLine(cfg.EffectiveResponsePrefix()); utf8.RuneCountInString(own) >= minBotPrefixLen &&
		strings.HasPrefix(line, own) {
		return OwnLine
	}
	// Our own nick is never a bot nick, even if listed by mistake: the owner chats on it too.
	if nick != "" && !sameNick(nick, cfg.Server.Nick) {
		for _, n := range cfg.Bot.BotNicks {
			if sameNick(nick, n) {
				return BotLine
			}
		}
	}
	for _, p := range cfg.Bot.BotPrefixes {
		if prefix := normaliseLine(p); utf8.RuneCountInString(prefix) >= minBotPrefixLen &&
			strings.HasPrefix(line, prefix) {
			return BotLine
		}
	}
	return HumanLine
}

// BotTag returns the botprefixes entry a line starts with, as configured, or "".
func BotTag(cfg *config.Configuration, text string) string {
	line := normaliseLine(text)
	for _, p := range cfg.Bot.BotPrefixes {
		if prefix := normaliseLine(p); utf8.RuneCountInString(prefix) >= minBotPrefixLen &&
			strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(p)
		}
	}
	return ""
}

// TrackLine runs on every event after the channel gate. It returns true when the event must
// be dropped: a line in this bot's own prefix, so it never answers itself (A10). A human line
// in the channel resets the bot reply chain, since a person has joined the conversation again.
func TrackLine(cfg *config.Configuration, e *girc.Event) (drop bool) {
	if e.Command != girc.PRIVMSG && e.Command != girc.NOTICE {
		return false
	}
	nick := ""
	if e.Source != nil {
		nick = e.Source.Name
	}
	switch ClassifyLine(cfg, nick, e.Last()) {
	case OwnLine:
		return true
	case HumanLine:
		// Only a plain channel message counts as a person speaking. NOTICEs are often automated,
		// and bots post unprefixed /me actions ("is thinking...") between replies; letting those
		// reset the chain would defeat the limit.
		if ok, _ := e.IsCTCP(); ok {
			return false
		}
		if e.Command == girc.PRIVMSG && len(e.Params) > 0 && girc.IsValidChannel(e.Params[0]) {
			// After the channel gate the only channel left is the configured one.
			core.BotLoop().HumanSpoke(cfg.Server.Name, cfg.Server.Channel)
		}
	}
	return false
}

// ValidateBotPrefix checks a prefix before it is added with +botprefix.
func ValidateBotPrefix(cfg *config.Configuration, prefix string) error {
	p := normaliseLine(prefix)
	if utf8.RuneCountInString(p) < minBotPrefixLen {
		return errors.New("a bot prefix needs at least 3 visible characters")
	}
	if own := normaliseLine(cfg.EffectiveResponsePrefix()); own != "" && p == own {
		return errors.New("that's my own prefix; my own lines are always ignored")
	}
	return nil
}

// ValidateBotNick checks a nick before it is added with +bots. currentNick is the bot's live
// nick, which may differ from the configured one.
func ValidateBotNick(cfg *config.Configuration, nick, currentNick string) error {
	if !girc.IsValidNick(nick) {
		return errors.New("that isn't a valid nick")
	}
	if sameNick(nick, cfg.Server.Nick) || sameNick(nick, currentNick) {
		return errors.New("that's my own nick, which my owner chats on too")
	}
	return nil
}

// sameNick compares nicks the way IRC does (case-insensitive, RFC 1459 casemapping).
func sameNick(a, b string) bool {
	return a != "" && girc.ToRFC1459(a) == girc.ToRFC1459(b)
}

// NormaliseBotPrefix is the form a bot prefix is stored in: no formatting, no outer spaces,
// lower case. Matching ignores all three anyway, so storing it this way avoids near-duplicates.
func NormaliseBotPrefix(p string) string { return normaliseLine(p) }

// normaliseLine strips IRC formatting, leading spaces and case, so "\x0306[MetalAI]\x0F hi"
// and "[metalai] hi" compare equal.
func normaliseLine(s string) string {
	return strings.ToLower(strings.TrimSpace(girc.StripRaw(s)))
}
