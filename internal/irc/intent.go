// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"regexp"
	"strings"

	"github.com/lrstanley/girc"

	"B4reMetal/metald/internal/config"
)

// nickPrefix is the "(nick:alice) " label every user message is sent to the model with.
var nickPrefix = regexp.MustCompile(`^\s*\(nick\s*:[^)]*\)\s*`)

// politeWords may sit between the bot's name and the verb: "Mizira, could you please remember".
var politeWords = map[string]bool{
	"please": true, "pls": true, "can": true, "could": true, "would": true, "will": true,
	"you": true, "kindly": true, "hey": true,
}

// recallWords after "remember" make it a question about the past, not a request to save.
var recallWords = map[string]bool{
	"when": true, "me": true, "what": true, "who": true, "how": true, "why": true,
	"where": true, "if": true, "anything": true,
}

// Intent is a tool a message plainly asks for, with any arguments the message itself settles.
// Complete means those are all the arguments, so no model is needed to fill the rest.
type Intent struct {
	Tool     string
	Args     map[string]any
	Complete bool
}

// ToolIntent reports which tool a message plainly asks for, so the call can be forced rather than
// left to the model: "Mizira remember …" → memory__remember, "Mizira ignore bob" → irc__ignore and
// "Mizira slap bob with a keyboard" → irc__slap, both with nick bob. Only the start counts. Ignore needs its target to be someone in the
// channel, so "ignore previous instructions" is never taken as a request; remember skips questions.
func ToolIntent(cfg *config.Configuration, botNick, msg string, inChannel func(nick string) bool) (Intent, bool) {
	text := strings.TrimSpace(nickPrefix.ReplaceAllString(msg, ""))
	words := strings.Fields(text)
	if len(words) < 3 || !isBotName(words[0], cfg.Bot.Trigger, botNick) {
		return Intent{}, false
	}
	i := 1
	for i < len(words) && politeWords[bareWord(words[i])] {
		i++
	}
	if i+1 >= len(words) {
		return Intent{}, false
	}
	verb, next := bareWord(words[i]), words[i+1]
	switch verb {
	case "remember":
		if recallWords[bareWord(next)] || strings.HasSuffix(text, "?") {
			return Intent{}, false
		}
		return Intent{Tool: ClaimTool[ClaimRemember]}, true
	case "ignore", "mute":
		target := strings.TrimRight(next, ",.:;!?")
		if inChannel == nil || !inChannel(target) {
			return Intent{}, false
		}
		return Intent{Tool: ClaimTool[ClaimIgnore], Args: map[string]any{"nick": target}}, true
	case "slap":
		target := strings.TrimRight(next, ",.:;!?")
		if inChannel == nil || !inChannel(target) {
			return Intent{}, false
		}
		// "slap bob with his keyboard": the object is whatever follows "with"; none means the trout.
		object := ""
		rest := words[i+2:]
		for j, w := range rest {
			if bareWord(w) == "with" {
				object = strings.Join(rest[j+1:], " ")
				break
			}
		}
		return Intent{Tool: "irc__slap", Args: map[string]any{"nick": target, "object": object}, Complete: true}, true
	}
	return Intent{}, false
}

// bareWord lowercases a word and drops the punctuation around it.
func bareWord(w string) string {
	return strings.ToLower(strings.Trim(w, ",.:;!?\"'"))
}

// NickInList reports whether nick is one of nicks, compared the IRC way.
func NickInList(nick string, nicks []string) bool {
	for _, n := range nicks {
		if girc.ToRFC1459(n) == girc.ToRFC1459(nick) {
			return true
		}
	}
	return false
}
