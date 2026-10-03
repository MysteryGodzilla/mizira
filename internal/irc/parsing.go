// Copyright (C) 2023-2026 Alex Schlessinger and soulshack contributors
// Modified 2026 by BareMetal
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"errors"
	"net"
	"regexp"
	"strings"
	"unicode"
)

// isTriggerWordChar reports whether r is part of a word, so a trigger matches whole words
// only, never inside a longer word.
func isTriggerWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// CheckAddressed reports whether message calls on trigger (case-insensitive, whole words), rather
// than just mentioning it. "bot do this", "hey bot, ...", "what do you think bot?" call on it;
// "did you see what bot did", "alice say hi to bot" and "alice: bot is great" only talk about it.
// A leading tag such as "[otherbot]" is skipped first. An empty trigger matches everything.
func CheckAddressed(message, trigger string) bool {
	if trigger == "" {
		return true
	}
	trig := strings.Fields(strings.ToLower(trigger))
	words := strings.Fields(message)
	if len(trig) == 0 {
		return false
	}
	if len(words) > 0 && strings.HasPrefix(words[0], "[") && strings.HasSuffix(words[0], "]") {
		words = words[1:]
	}
	at := findWords(words, trig)
	if at < 0 {
		return false
	}
	before, after := words[:at], words[at+len(trig):]

	// "alice: ..." or "alice, ..." opens by talking to someone else.
	if len(before) > 0 && strings.ContainsAny(lastRune(before[0]), ":,") && !fillerWords[bareAddressWord(before[0])] {
		return false
	}
	allFiller := true
	for _, w := range before {
		if !fillerWords[bareAddressWord(w)] {
			allFiller = false
			break
		}
	}
	if allFiller {
		return true
	}
	// Called at the end: "what do you think bot?". Not after a word that makes it the object:
	// "say hi to bot", "who is bot".
	if len(after) == 0 || allPunctuation(after) {
		return !objectMarkers[bareAddressWord(before[len(before)-1])]
	}
	// Called mid-line, set off by commas: "ok so, bot, what now?"
	name := words[at+len(trig)-1]
	prev := before[len(before)-1]
	return strings.ContainsAny(lastRune(name), ",:") && strings.ContainsAny(lastRune(prev), ",.!?")
}

// fillerWords may come before the name of someone being called: "hey bot", "thanks bot".
var fillerWords = map[string]bool{
	"": true, "hey": true, "hi": true, "hello": true, "heya": true, "hiya": true, "yo": true, "oi": true,
	"ok": true, "okay": true, "oh": true, "ah": true, "so": true, "well": true, "and": true, "but": true,
	"thanks": true, "thank": true, "you": true, "ty": true, "thx": true, "please": true, "pls": true,
	"sorry": true, "good": true, "morning": true, "night": true, "evening": true, "afternoon": true,
	"gm": true, "gn": true, "welcome": true, "back": true, "dear": true, "lol": true, "haha": true,
}

// objectMarkers before a trailing name make it the object of the sentence, not someone called.
var objectMarkers = map[string]bool{
	"to": true, "at": true, "about": true, "with": true, "for": true, "from": true, "of": true,
	"is": true, "was": true, "are": true, "and": true, "or": true, "than": true, "like": true,
	"by": true, "on": true, "in": true, "tell": true, "ask": true, "told": true, "asked": true,
	"see": true, "saw": true, "meet": true, "met": true, "love": true, "hate": true, "said": true,
	"says": true, "does": true, "did": true, "slap": true, "ignore": true, "ping": true, "the": true,
}

// findWords returns the index in words where want starts, comparing words without punctuation.
func findWords(words, want []string) int {
	for i := 0; i+len(want) <= len(words); i++ {
		match := true
		for j, w := range want {
			if bareAddressWord(words[i+j]) != w {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// bareAddressWord lowercases w and trims punctuation and a leading "@" from both ends.
func bareAddressWord(w string) string {
	return strings.ToLower(strings.TrimFunc(w, func(r rune) bool { return !isTriggerWordChar(r) && r != '\'' }))
}

func lastRune(w string) string {
	r := []rune(w)
	if len(r) == 0 {
		return ""
	}
	return string(r[len(r)-1])
}

func allPunctuation(words []string) bool {
	for _, w := range words {
		if bareAddressWord(w) != "" {
			return false
		}
	}
	return true
}

// CheckAdmin returns true if hostmask matches any admin mask in the list. Masks may use the
// wildcards * and ? (see matchMask). An empty list means nobody is admin, never everybody, and
// a mask ValidateAdminMask refuses (e.g. "*!*@*") never matches.
func CheckAdmin(hostmask string, adminList []string) bool {
	if hostmask == "" {
		return false
	}
	for _, admin := range adminList {
		if ValidateAdminMask(admin) != nil {
			continue
		}
		if matchMask(admin, hostmask) {
			return true
		}
	}
	return false
}

// CheckPrivate returns true if target is not a channel (doesn't start with #).
func CheckPrivate(target string) bool {
	return !strings.HasPrefix(target, "#")
}

// RFC 2812 compliant patterns
var (
	// Nick: starts with letter or special, followed by letters, digits, special, or hyphen
	// Special chars: [\]^_`{|}
	nickRegex = regexp.MustCompile(`^[a-zA-Z\[\]\\^\x60_{|}][a-zA-Z0-9\[\]\\^\x60_{|}\-]*$`)

	// User: alphanumeric with optional ~ prefix, allows -_.
	userRegex = regexp.MustCompile(`^~?[a-zA-Z0-9_.\-]+$`)

	// Hostname: DNS labels (letters, digits, hyphens) separated by dots
	hostnameRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9\-]*[a-zA-Z0-9])?$`)
)

// ValidateHostmask validates an IRC hostmask in the format nick!user@host per RFC 2812.
func ValidateHostmask(hostmask string) error {
	if hostmask == "" {
		return errors.New("hostmask cannot be empty")
	}

	// Find ! and @ positions
	exclamIdx := strings.Index(hostmask, "!")
	atIdx := strings.Index(hostmask, "@")

	if exclamIdx == -1 {
		return errors.New("hostmask must contain '!' (format: nick!user@host)")
	}
	if atIdx == -1 {
		return errors.New("hostmask must contain '@' (format: nick!user@host)")
	}
	if exclamIdx >= atIdx {
		return errors.New("'!' must come before '@' (format: nick!user@host)")
	}

	nick := hostmask[:exclamIdx]
	user := hostmask[exclamIdx+1 : atIdx]
	host := hostmask[atIdx+1:]

	// Validate nick
	if nick == "" {
		return errors.New("nick cannot be empty")
	}
	if len(nick) > 30 {
		return errors.New("nick too long (max 30 characters)")
	}
	if !nickRegex.MatchString(nick) {
		return errors.New("invalid nick: must start with letter or special char, contain only letters, digits, special chars, or hyphens")
	}

	// Validate user
	if user == "" {
		return errors.New("user cannot be empty")
	}
	if len(user) > 64 {
		return errors.New("user too long (max 64 characters)")
	}
	if !userRegex.MatchString(user) {
		return errors.New("invalid user: must be alphanumeric with optional ~ prefix, allows -_.")
	}

	// Validate host
	if host == "" {
		return errors.New("host cannot be empty")
	}
	if len(host) > 253 {
		return errors.New("host too long (max 253 characters)")
	}

	// Check if it's a valid IP address
	if net.ParseIP(host) != nil {
		return nil
	}

	// Check if it's a valid hostname
	if !hostnameRegex.MatchString(host) {
		return errors.New("invalid host: must be a valid hostname or IP address")
	}

	return nil
}

// nickSpoof matches text imitating the "(nick:x)" identity prefix metald
// prepends to every message before handing it to the model.
var nickSpoof = regexp.MustCompile(`(?i)[\(\[]\s*nick\s*:`)

// SanitizeUserMessage neutralises attempts to forge the identity prefix.
func SanitizeUserMessage(msg string) string {
	return nickSpoof.ReplaceAllStringFunc(msg, func(m string) string {
		// "(nick:" -> "(nick :" - still legible, no longer parses as ours.
		return strings.TrimSuffix(m, ":") + " :"
	})
}

// Structural markers that models use to delimit their own reasoning, tool calls, and conversation
// roles.
var injectionFrame = regexp.MustCompile(
	`(?i)</?\s*(think|thinking|thought|reasoning|scratchpad|` +
		`function|function_call|function_results|tool|tool_call|tool_result|` +
		`system|assistant|user|output|answer)\s*>`)

// ChatML-style turn markers: <|im_start|>, <|im_end|>, <|system|> and friends.
var chatMLMarker = regexp.MustCompile(`<\|[^|>]{0,40}\|>`)

// StripInjectionFrames removes pseudo-structural tags from a user message and reports how many it
// removed.
func StripInjectionFrames(msg string) (string, int) {
	count := len(injectionFrame.FindAllString(msg, -1)) +
		len(chatMLMarker.FindAllString(msg, -1))
	if count == 0 {
		return msg, 0
	}

	cleaned := injectionFrame.ReplaceAllString(msg, " ")
	cleaned = chatMLMarker.ReplaceAllString(cleaned, " ")
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return cleaned, count
}
