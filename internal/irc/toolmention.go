// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"regexp"
	"strings"
)

// toolMention is a tool's name written into the reply, with any arguments after it:
// "memory__remember.", "memory__recall{subject:alice}", "irc__slap(bob)". Tool names are always
// "namespace__name", which ordinary chat never is.
// Gemma writes a call as "call:name{...}", so a "call" just before the name goes with it.
var toolMention = regexp.MustCompile(`(\bcall\s*:?\s*)?\b[a-z][a-z0-9]*__[a-z0-9_]+(\s*(\{[^{}]*\}|\([^()]*\)))?[.:]?`)

// callToken is a tool-call marker from the model's chat template, such as Gemma's "<|tool_call>",
// "<tool_call|>" or the "<|"|>" it quotes arguments with.
var callToken = regexp.MustCompile(`<\|?(tool_call|tool_response|tool|"|')\|?>`)

// strayCall is the "call" of a tool call left at the start of a line once the rest of it was
// parsed out: "call I'm sorry...". A reply that means the word says "call me", "call him" and so
// on, in lower case, so only a "call" before a capital or a non-Latin letter is dropped.
var strayCall = regexp.MustCompile(`^call:?\s+([A-Z]|[^\x00-\x7F])`)

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
	line = callToken.ReplaceAllString(line, "")
	line = toolMention.ReplaceAllString(line, "")
	line = strings.TrimSpace(line)
	if m := strayCall.FindStringSubmatchIndex(line); m != nil {
		line = line[m[2]:]
	}
	return strings.Join(strings.Fields(line), " ")
}
