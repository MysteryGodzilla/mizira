// SPDX-License-Identifier: GPL-3.0-only

package irc

import "testing"

func TestStripSpeakerTags(t *testing.T) {
	cases := map[string]string{
		"(nick:Mizira) あの... that's a secret!": "あの... that's a secret!",
		"(Mizira:mina) Good morning, Mina!":    "Good morning, Mina!",
		"(nick : bob) hi":                      "hi",
		"(nick:metalai) ...um, hello":          "...um, hello",
		"(nick:a) (nick:b) twice":              "twice",
		"[23:47:35] <Mizira> arr":              "arr",
		"<bob> I agree":                        "I agree",
		"(nick:Mizira)":                        "",
		// Left alone: no tag, or ordinary text in brackets.
		"plain reply":                "plain reply",
		"(um...) okay":               "(um...) okay",
		"(note: this one) okay":      "(note: this one) okay",
		"I said (nick:bob) mid-line": "I said (nick:bob) mid-line",
		"3 < 4 and 5 > 2":            "3 < 4 and 5 > 2",
		// A time in brackets has the tag's shape and is dropped; an accepted cost.
		"(12:30) is lunch": "is lunch",
	}
	for in, want := range cases {
		if got := StripSpeakerTags(in); got != want {
			t.Errorf("StripSpeakerTags(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestChunkerStripsSpeakerTags(t *testing.T) {
	out := make(chan string, 10)
	c := NewChunker(out, 400)
	c.SetMaxLines(2)
	c.Write("(nick:Mizira)\n(Mizira:mina) one\n(Mizira:mina) two\nthree\n")
	c.Flush()
	close(out)
	var got []string
	for l := range out {
		got = append(got, l)
	}
	// The tag-only line is dropped without using up the line cap.
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("got %q", got)
	}
}
