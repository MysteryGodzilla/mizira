// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"errors"
	"strings"
	"unicode"

	"github.com/lrstanley/girc"
)

// splitMask splits nick!ident@host. ok is false when the shape is wrong.
func splitMask(mask string) (nick, ident, host string, ok bool) {
	nick, rest, found := strings.Cut(mask, "!")
	if !found {
		return "", "", "", false
	}
	ident, host, found = strings.Cut(rest, "@")
	if !found || nick == "" || ident == "" || host == "" {
		return "", "", "", false
	}
	return nick, ident, host, true
}

// onlyWildcards reports whether a mask part matches anything, e.g. "*" or "?*".
func onlyWildcards(s string) bool {
	return strings.Trim(s, "*?") == ""
}

// ValidateAdminMask checks a mask before it is used to grant admin rights.
//
// Masks are nick!ident@host and may use * (any run of characters) and ? (one character).
// Server cloaks such as "user/staff/name" are fine. Refused: masks that aren't that shape,
// contain spaces, commas or control characters, or leave both the nick and the host as pure
// wildcards, which would make nearly anyone an admin.
func ValidateAdminMask(mask string) error {
	if strings.IndexFunc(mask, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) || r == ',' }) >= 0 {
		return errors.New("a mask can't contain spaces, commas or control characters")
	}
	nick, _, host, ok := splitMask(mask)
	if !ok {
		return errors.New("use the form nick!ident@host (wildcards * and ? allowed)")
	}
	if onlyWildcards(nick) && onlyWildcards(host) {
		return errors.New("too broad: the nick or the host must be specific")
	}
	return nil
}

// AdminMaskWarning explains a mask that is allowed but deserves a second look, or returns "".
func AdminMaskWarning(mask string) string {
	nick, _, host, ok := splitMask(mask)
	if !ok {
		return ""
	}
	if onlyWildcards(host) {
		return "matches this nick from any host: safe only on servers that tie nicks to accounts"
	}
	if onlyWildcards(nick) {
		return "matches anyone connecting from this host"
	}
	return ""
}

// matchMask reports whether hostmask matches mask, where * matches any run of characters and
// ? matches one. Everything else is literal, including [ ] and \, which are legal in nicks (a
// glob library would treat them as special). Case-insensitive, like IRC.
func matchMask(mask, hostmask string) bool {
	p := []rune(girc.ToRFC1459(mask))
	s := []rune(girc.ToRFC1459(hostmask))
	// Standard backtracking wildcard match: remember the last * and retry from there.
	pi, si, star, mark := 0, 0, -1, 0
	for si < len(s) {
		switch {
		case pi < len(p) && (p[pi] == '?' || p[pi] == s[si]):
			pi++
			si++
		case pi < len(p) && p[pi] == '*':
			star, mark = pi, si
			pi++
		case star >= 0:
			pi = star + 1
			mark++
			si = mark
		default:
			return false
		}
	}
	for pi < len(p) && p[pi] == '*' {
		pi++
	}
	return pi == len(p)
}
