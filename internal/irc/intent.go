// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"regexp"
	"strconv"
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
	// A bot that shares its owner's nick tags its lines: "[botty] Mizira remember ...".
	if len(words) > 0 && BotTag(cfg, words[0]) != "" {
		words = words[1:]
	}
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
		// "remember what bob told you" names no fact: the facts are earlier in the chat, and
		// often more than one, so the model saves them itself from the conversation.
		if backReference.MatchString(strings.Join(words[i+1:], " ")) {
			return Intent{}, false
		}
		return Intent{Tool: ClaimTool[ClaimRemember]}, true
	case "forget":
		rest := strings.Join(words[i+1:], " ")
		if m := memoryID.FindStringSubmatch(rest); m != nil {
			id, _ := strconv.Atoi(m[1])
			return Intent{Tool: ClaimTool[ClaimForget], Args: map[string]any{"id": id}, Complete: true}, true
		}
		if forgetMe.MatchString(rest) {
			return Intent{Tool: ClaimTool[ClaimForget], Args: map[string]any{"fact": "everything"}, Complete: true}, true
		}
		if recallWords[bareWord(next)] || strings.HasSuffix(text, "?") {
			return Intent{}, false
		}
		return Intent{Tool: ClaimTool[ClaimForget]}, true
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
		// "with something you think of" hands the choice back: the model picks the object.
		if delegatedObject.MatchString(object) {
			return Intent{Tool: "irc__slap", Args: map[string]any{"nick": target}}, true
		}
		return Intent{Tool: "irc__slap", Args: map[string]any{"nick": target, "object": object}, Complete: true}, true
	}
	return Intent{}, false
}

// memoryID is a memory named by the id +memories shows: "6", "[6]", "#6", "memory 6".
var memoryID = regexp.MustCompile(`(?i)^(?:memory\s+|number\s+)?[\[#(]?(\d{1,9})[\])]?[.!]?$`)

// forgetMe asks to forget everything about the speaker.
var forgetMe = regexp.MustCompile(`(?i)^(?:(?:everything|all)\s+)?(?:about\s+)?me[.!]*$`)

// backReference is a remember request that points at something said earlier instead of saying it.
var backReference = regexp.MustCompile(`(?i)^(that|this|it|those|these|them|all( of)?( that| this| those| it)?|everything)\s*[.!]*$|` +
	`^(those|these|all (of )?(those|these|the)|everything)\b|` +
	`\b(told|said|says|mentioned|wrote|posted|explained|described|shared)\s+(you|us|me|earlier|before|above|just now)\b|` +
	`\b(above|earlier|from before)\s*[.!]*$|` +
	`^the (details|stuff|things|info|information|facts|list|names)\b`)

// delegatedObject is a slap object that leaves the choice to the bot.
var delegatedObject = regexp.MustCompile(`(?i)\b(you (think|choose|pick|want|like|decide)|your choice|whatever|something|anything)\b`)

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
