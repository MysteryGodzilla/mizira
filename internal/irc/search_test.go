// SPDX-License-Identifier: GPL-3.0-only

package irc

import "testing"

// Requests from live chat that the model left unsearched, and lines that only mention searching.
func TestAsksToSearch(t *testing.T) {
	cases := map[string]bool{
		"mizira can you search and tell me about the character remy?":                           true,
		"mizira can you look up the hi-nu gundam":                                               true,
		"mizira yes search it":                                                                  true,
		"mizira websearch it":                                                                   true,
		"mizira now web search gundam unicorn":                                                  true,
		"mizira can you find me a cake recipe":                                                  true,
		"Mizira can you find out what mobile suits she is known to pilot?":                      true,
		"mizira do a search to find out":                                                        true,
		"mizira think really hard about it, then search some mori girls":                        true,
		"mizira search the web for a chocolate cake recipe in german":                           true,
		"mizira look up the characters from robotics;notes":                                     true,
		"mizira google it":                                                                      true,
		"mizira tell me about the gundam statues, search if you need":                           true,
		"but mizira you just websearched did you not see it?":                                   false,
		"mizira did you search it?":                                                             false,
		"mizira why didn't you search for it":                                                   false,
		"mizira remember when I want you to look something up to ALWAYS use the websearch tool": false,
		"mizira search the chat for what bob said":                                              false,
		"mizira search through your memories for bob":                                           false,
		"mizira look up at the stars":                                                           false,
		"mizira do you like google?":                                                            false,
		"mizira tell me about haman karn":                                                       false,
		"mizira what were you searching for?":                                                   false,
	}
	for msg, want := range cases {
		if got := AsksToSearch(msg); got != want {
			t.Errorf("AsksToSearch(%q) = %v, want %v", msg, got, want)
		}
	}
}

// A search ask is forced only when no verb intent claims the message first.
func TestToolIntentSearch(t *testing.T) {
	cfg := botConfig("")
	cfg.Bot.Trigger = "mizira"
	intent, ok := ToolIntent(cfg, "Mizira", "(nick:alice) mizira can you look up the hi-nu gundam", nil)
	if !ok || intent.Tool != "websearch__web_search" || intent.Complete {
		t.Errorf("search ask: %+v %v", intent, ok)
	}
	intent, ok = ToolIntent(cfg, "Mizira", "(nick:alice) Mizira remember that I search for rare cards", nil)
	if !ok || intent.Tool != "memory__remember" {
		t.Errorf("remember with search in it: %+v %v", intent, ok)
	}
}

func TestDetectSearchClaim(t *testing.T) {
	cases := map[string]bool{
		"um... let me look it up for you~":                                        true,
		"i'm using the search tool to look up the hi-nu gundam for you~":          true,
		"I'll search right away!":                                                 true,
		"I'm going to search for that now.":                                       true,
		"i-i'll look it up right now. please wait a moment while i check...":      true,
		"I can look it up for you if you'd like!":                                 false,
		"Should I search for it?":                                                 false,
		"I can't search right now, sorry.":                                        false,
		"I tried to look for her earlier, but I couldn't find anything specific.": false,
		"Would you like me to search for it?":                                     false,
		"Oh, carol! I was just looking up those Gundam mechs with bob earlier.":   false,
		"I searched for that already!":                                            false,
	}
	for line, want := range cases {
		kind, ok := DetectClaim(line)
		if got := ok && kind == ClaimSearch; got != want {
			t.Errorf("DetectClaim(%q) = %q, %v; want search %v", line, kind, ok, want)
		}
	}
}
