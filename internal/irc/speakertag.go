// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"regexp"
	"strings"
)

// speakerTag matches a speaker label a model may put at the start of a reply line, copying the
// "(nick:name)" format user messages arrive in: "(nick:Mizira)", "(nick : bob)", "(Mizira:mina)".
// The name halves can't hold spaces, so ordinary asides like "(note: this one)" are left alone.
// It also matches an IRC-log style "[12:34] <bob>" or "<bob>". Either way the line would look like
// someone else talking (A14), so the tag is never sent.
var speakerTag = regexp.MustCompile(`^\s*(?:\([^()\s:]{1,32}\s*:\s*[^()\s:]{1,32}\)|(?:\[\d{1,2}:\d{2}(?::\d{2})?\]\s*)?<[^<>\s]{1,32}>)\s*`)

// speakerWrap matches the opening of a reply wrapped in a speaker label, "(Mizira:that's a
// secret!)": a label with no spaces, then a colon, at the start of the line.
var speakerWrap = regexp.MustCompile(`^\s*\([^()\s:]{1,32}\s*:\s*`)

// StripSpeakerTags removes any leading speaker tags from one reply line, and unwraps a line wrapped
// in one. A wrapper only counts when its bracket closes at the very end of the line, or not at
// all (a long reply split over lines), so "(note: this one) okay" is left alone.
func StripSpeakerTags(line string) string {
	for {
		if loc := speakerTag.FindStringIndex(line); loc != nil && loc[1] > 0 {
			line = line[loc[1]:]
			continue
		}
		loc := speakerWrap.FindStringIndex(line)
		if loc == nil {
			return line
		}
		rest := strings.TrimRight(line[loc[1]:], " \t")
		switch close := strings.IndexByte(rest, ')'); {
		case close == -1:
			line = rest
		case close == len(rest)-1:
			line = rest[:close]
		default:
			return line
		}
	}
}

// isBlankLine reports whether a line has nothing left to send.
func isBlankLine(line string) bool { return strings.TrimSpace(line) == "" }
