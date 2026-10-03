// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"regexp"
	"strings"
)

// toolMention is a tool's name written into the reply, with any arguments after it:
// "memory__remember.", "memory__recall{subject:alice}", "irc__slap(bob)". Tool names are always
// "namespace__name", which ordinary chat never is.
var toolMention = regexp.MustCompile(`\b[a-z][a-z0-9]*__[a-z0-9_]+(\s*(\{[^{}]*\}|\([^()]*\)))?[.:]?`)

// meCommand is an IRC "/me" command written into the reply as text.
var meCommand = regexp.MustCompile(`(?i)(^|\s)/me\s`)

// CleanReplyLine readies one reply line for the channel: speaker tags and tool names the model wrote
// out as text are removed, and so is a "/me ..." it wrote out, from there to the end of the line,
// since only tools send actions. It returns "" for a line with nothing left to send.
func CleanReplyLine(line string) string {
	line = StripSpeakerTags(line)
	if at := meCommand.FindStringIndex(line); at != nil {
		line = line[:at[0]]
	}
	line = toolMention.ReplaceAllString(line, "")
	return strings.Join(strings.Fields(line), " ")
}
