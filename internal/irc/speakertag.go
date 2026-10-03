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

// StripSpeakerTags removes any leading speaker tags from one reply line.
func StripSpeakerTags(line string) string {
	for {
		loc := speakerTag.FindStringIndex(line)
		if loc == nil || loc[1] == 0 {
			return line
		}
		line = line[loc[1]:]
	}
}

// isBlankLine reports whether a line has nothing left to send.
func isBlankLine(line string) bool { return strings.TrimSpace(line) == "" }
