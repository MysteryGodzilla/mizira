// SPDX-License-Identifier: GPL-3.0-only

package irc

import "testing"

func TestNameTheSubject(t *testing.T) {
	cases := map[string]string{
		"User likes purple":        "alice likes purple",
		"the user plays guitar":    "alice plays guitar",
		"alice likes purple":       "alice likes purple",
		"Username is alice123":     "Username is alice123",
		"likes users who are kind": "likes users who are kind",
	}
	for in, want := range cases {
		if got := nameTheSubject("alice", in); got != want {
			t.Errorf("nameTheSubject(%q) = %q, want %q", in, got, want)
		}
	}
}
