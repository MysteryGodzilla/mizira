// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"testing"

	"B4reMetal/metald/internal/core"
)

func TestBestMemoryMatch(t *testing.T) {
	mems := []core.Memory{
		{ID: 1, Subject: "alice", Fact: "alice likes purple"},
		{ID: 2, Subject: "alice", Fact: "If the bot slaps alice, alice says she will kill it."},
		{ID: 3, Subject: "alice", Fact: "alice plays the guitar"},
		{ID: 4, Subject: "alice", Fact: "alice plays the bass guitar"},
	}
	cases := []struct {
		query string
		want  int64 // 0: no single match
		tied  int
	}{
		// The live mistake: this deleted "likes purple" when the model guessed an id.
		{"that alice will kill you", 2, 0},
		{"likes purple", 1, 0},
		{"plays the bass", 4, 0},
		{"plays guitar", 0, 2}, // both guitar memories: ask which
		{"loves pizza", 0, 0},  // nothing
		{"the that", 0, 0},     // only stop words
	}
	for _, c := range cases {
		got, tied := bestMemoryMatch(mems, c.query)
		switch {
		case c.want != 0 && (got == nil || got.ID != c.want):
			t.Errorf("%q: got %v, want id %d", c.query, got, c.want)
		case c.want == 0 && got != nil:
			t.Errorf("%q: matched id %d, want none", c.query, got.ID)
		case len(tied) != c.tied:
			t.Errorf("%q: %d candidates, want %d", c.query, len(tied), c.tied)
		}
	}
}
