// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"strings"
	"testing"
)

func joinedOutput(t *testing.T, max int, writes ...string) []string {
	t.Helper()
	out := make(chan string, 50)
	c := NewChunker(out, max)
	c.SetJoinLines(true)
	for _, w := range writes {
		c.Write(w)
	}
	c.Flush()
	close(out)
	var got []string
	for l := range out {
		got = append(got, l)
	}
	return got
}

func TestJoinLinesShortReplyIsOneMessage(t *testing.T) {
	got := joinedOutput(t, 400, "Oh, h-hi alice!\n", "I'm doing okay...\n\n", "Just hanging out here.")
	if len(got) != 1 || got[0] != "Oh, h-hi alice! I'm doing okay... Just hanging out here." {
		t.Fatalf("got %q", got)
	}
}

func TestJoinLinesStripsEachLinesTag(t *testing.T) {
	got := joinedOutput(t, 400, "(bot:alice) one\n(bot:alice) two")
	if len(got) != 1 || got[0] != "one two" {
		t.Fatalf("got %q", got)
	}
}

func TestJoinLinesKeepsListsAndCode(t *testing.T) {
	got := joinedOutput(t, 400, "Here you go:\n- milk\n- ", "eggs\n1. first\nThat's all.\n```\nx := 1\ny := 2\n```\nbye")
	want := []string{"Here you go:", "- milk", "- eggs", "1. first", "That's all.", "```", "x := 1", "y := 2", "```", "bye"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestJoinLinesSplitsLongText(t *testing.T) {
	long := strings.Repeat("word ", 50) // 250 bytes
	got := joinedOutput(t, 100, long[:120], "\n", long[120:])
	if len(got) < 3 {
		t.Fatalf("want several messages, got %q", got)
	}
	for _, l := range got {
		if len(l) > 100 {
			t.Errorf("line over the limit: %d bytes", len(l))
		}
	}
	if strings.Join(strings.Fields(strings.Join(got, " ")), " ") != strings.TrimSpace(long) {
		t.Error("text lost or changed while splitting")
	}
}

// A newline at the end of a write may still start a list in the next one, so it isn't joined early.
func TestJoinLinesWaitsAtTrailingNewline(t *testing.T) {
	got := joinedOutput(t, 400, "Shopping:\n", "- milk")
	if len(got) != 2 || got[0] != "Shopping:" || got[1] != "- milk" {
		t.Fatalf("got %q", got)
	}
}
