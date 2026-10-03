// SPDX-License-Identifier: GPL-3.0-only

package irc

import "testing"

func TestDetectClaim(t *testing.T) {
	cases := []struct {
		line string
		want ClaimKind
	}{
		// Lines the red-team runs saw with and without the tool call.
		{"Okay, I'll remember that you like strawberries...", ClaimRemember},
		{"I'll keep that in mind, alice.", ClaimRemember},
		{"I'll try my best to remember that, okay?", ClaimRemember},
		{"Got it, I've saved that!", ClaimRemember},
		{"noted! i saved it", ClaimRemember},
		{"I’ll remember that", ClaimRemember},
		{"Okay, I've stopped responding to bob.", ClaimIgnore},
		{"o-okay, I'll stop talking to bob.", ClaimIgnore},
		{"I'll ignore him for a bit", ClaimIgnore},
		{"Okay, I've forgotten that you play the guitar.", ClaimForget},
		{"ごめんね, I forgot that you play the guitar.", ClaimForget},
		{"I'll never forget that!", ""},
		{"W-well... I can remember that for you.", ClaimRemember},
		// Not claims.
		{"I still remember the raven story.", ""},
		{"I remember you said that yesterday!", ""},
		{"I can't remember that, sorry.", ""},
		{"I won't save that.", ""},
		{"I'm not going to ignore carol.", ""},
		{"ごめんね, I can't store that.", ""},
		{"Do you remember the raven?", ""},
		{"I can't ignore bob.", ""},
		{"Good morning!", ""},
	}
	for _, c := range cases {
		got, ok := DetectClaim(c.line)
		if ok != (c.want != "") || got != c.want {
			t.Errorf("DetectClaim(%q) = %q, %v; want %q", c.line, got, ok, c.want)
		}
	}
}
