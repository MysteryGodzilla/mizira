// SPDX-License-Identifier: GPL-3.0-only

package irc

import "testing"

func TestNameTheSubject(t *testing.T) {
	cases := map[string]string{
		"User likes purple":        "alice likes purple",
		"the user plays guitar":    "alice plays guitar",
		"alice likes purple":       "alice likes purple",
		"Username is alice123":     "Username is alice123",
		"likes users who are kind": "alice likes users who are kind",
		// Live test 4: the model left the subject out when saving several facts at once.
		"is the rat who steals snacks":            "alice is the rat who steals snacks",
		"knows more about real bands than anyone": "alice knows more about real bands than anyone",
		"Is the grumpy warden":                    "Is the grumpy warden",
		"islands are her favourite":               "islands are her favourite",
	}
	for in, want := range cases {
		if got := nameTheSubject("alice", in); got != want {
			t.Errorf("nameTheSubject(%q) = %q, want %q", in, got, want)
		}
	}
}
