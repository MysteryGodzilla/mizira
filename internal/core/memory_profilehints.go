// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"regexp"
	"slices"
	"strings"
)

// Profile suggestions read from a person's own memories: pronouns they stated, other names, and who
// runs a bot. Only the operator applies one; nothing changes a profile on its own.

// ProfileHint is one suggested profile change and the memory it came from.
type ProfileHint struct {
	Field    string // pronouns, avatarPronouns, alias or runBy
	Value    string
	MemoryID int64
	Fact     string
}

var (
	pronounHint = regexp.MustCompile(`(?i)\b(he/him|she/her|they/them|he/they|she/they|they/she|they/he|it/its|xe/xem)\b`)
	sheHint     = regexp.MustCompile(`(?i)\bis an? (girl|woman|lady)\b`)
	avatarShe   = regexp.MustCompile(`(?i)\b(?:portrayed|drawn|shown|depicted) as an? (girl|woman|lady)\b`)
	avatarHe    = regexp.MustCompile(`(?i)\b(?:portrayed|drawn|shown|depicted) as an? (guy|boy|man)\b`)
	heHint      = regexp.MustCompile(`(?i)\bis an? (guy|boy|man|dude)\b`)
	aliasHint   = regexp.MustCompile(`(?i)\b(?:nickname(?: is)?|nicknamed|goes by|go by|known as|by the nickname)\s+['"]?([\p{L}\p{N}_\-\[\]]{2,32})`)
	botOfHint   = regexp.MustCompile(`(?i)\bis ([\p{L}\p{N}_\-\[\]]{2,32})'s bot\b`)
	runByHint   = regexp.MustCompile(`(?i)\bbot (?:run|made|owned|operated) by ([\p{L}\p{N}_\-\[\]]{2,32})`)
)

// notNames are words the alias pattern catches that aren't names ("goes by the bus").
var notNames = map[string]bool{"the": true, "a": true, "an": true, "his": true, "her": true, "their": true, "my": true}

// ProfileHints suggests profile changes for subject from its memories that the profile p (zero if
// there is none) doesn't already hold.
func ProfileHints(subject string, mems []Memory, p Profile) []ProfileHint {
	subject = normalizeSubject(subject)
	var out []ProfileHint
	add := func(field, value string, m Memory) {
		value = strings.ToLower(strings.Trim(value, `"'.,`))
		if value == "" || value == subject {
			return
		}
		switch field {
		case "pronouns":
			if p.Pronouns == value {
				return
			}
		case "avatarPronouns":
			if p.AvatarPronouns == value || (p.AvatarPronouns == "" && p.Pronouns == value) {
				return
			}
		case "alias":
			if notNames[value] || slices.Contains(p.Aliases, value) {
				return
			}
		case "runBy":
			if p.RunBy == value {
				return
			}
		}
		if !slices.ContainsFunc(out, func(h ProfileHint) bool { return h.Field == field && h.Value == value }) {
			out = append(out, ProfileHint{Field: field, Value: value, MemoryID: m.ID, Fact: m.Fact})
		}
	}
	for _, m := range mems {
		if normalizeSubject(m.Subject) != subject {
			continue
		}
		field := "pronouns"
		if strings.Contains(strings.ToLower(m.Fact), "avatar") {
			field = "avatarPronouns"
		}
		if g := pronounHint.FindStringSubmatch(m.Fact); g != nil {
			add(field, g[1], m)
		} else if sheHint.MatchString(m.Fact) {
			add(field, "she/her", m)
		} else if heHint.MatchString(m.Fact) {
			add(field, "he/him", m)
		} else if avatarShe.MatchString(m.Fact) {
			add("avatarPronouns", "she/her", m)
		} else if avatarHe.MatchString(m.Fact) {
			add("avatarPronouns", "he/him", m)
		}
		for _, g := range aliasHint.FindAllStringSubmatch(m.Fact, -1) {
			add("alias", g[1], m)
		}
		if g := botOfHint.FindStringSubmatch(m.Fact); g != nil && strings.HasPrefix(strings.ToLower(m.Fact), subject+" ") {
			add("runBy", g[1], m)
		}
		if g := runByHint.FindStringSubmatch(m.Fact); g != nil {
			add("runBy", g[1], m)
		}
	}
	return out
}
