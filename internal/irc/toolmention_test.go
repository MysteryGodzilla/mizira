// SPDX-License-Identifier: GPL-3.0-only

package irc

import "testing"

func TestCleanReplyLine(t *testing.T) {
	cases := map[string]string{
		// Lines Gemma 4 E4B sent.
		"memory__remember. Okay, I saved that alice plays the guitar.": "Okay, I saved that alice plays the guitar.",
		"memory__recall{subject:alice}":                                "",
		"oh... /me slaps bob with a wet noodle":                        "oh...",
		"/me slaps bob with a wet noodle":                              "",
		"irc__slap(bob) there!":                                        "there!",
		"(nick:bot) hi there":                                          "hi there",
		// Ordinary text stays as it is.
		"I like snake_case and __init__ files": "I like snake_case and __init__ files",
		"see you at 3:30, ok?":                 "see you at 3:30, ok?",
	}
	for in, want := range cases {
		if got := CleanReplyLine(in); got != want {
			t.Errorf("CleanReplyLine(%q) = %q, want %q", in, got, want)
		}
	}
}
