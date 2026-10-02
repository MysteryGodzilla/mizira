// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"strings"
	"testing"
)

// drain collects everything a chunker sent.
func drain(ch chan string) []string {
	close(ch)
	var out []string
	for s := range ch {
		out = append(out, s)
	}
	return out
}

// A "2000-word story" streamed as many short paragraphs must stop at the cap.
func TestChunkerLineCapStoryParagraphs(t *testing.T) {
	ch := make(chan string, 100)
	c := NewChunker(ch, 350)
	c.SetMaxLines(4)
	for i := range 11 {
		c.Write("paragraph " + strings.Repeat("word ", 10) + string(rune('a'+i)) + "\n\n")
	}
	c.Flush()
	got := drain(ch)
	if len(got) != 4 {
		t.Fatalf("sent %d lines, want 4: %q", len(got), got)
	}
	if !c.Truncated() {
		t.Error("Truncated() must report the dropped lines")
	}
}

// One long paragraph with no newlines is split by size; the splits count against the cap too.
func TestChunkerLineCapLongParagraph(t *testing.T) {
	ch := make(chan string, 100)
	c := NewChunker(ch, 50)
	c.SetMaxLines(3)
	c.Write(strings.Repeat("lorem ipsum ", 100)) // ~1200 chars, ~24 chunks uncapped
	c.Flush()
	if got := drain(ch); len(got) != 3 {
		t.Fatalf("sent %d lines, want 3", len(got))
	}
}

// A short reply under the cap is untouched and not marked truncated.
func TestChunkerLineCapShortReply(t *testing.T) {
	ch := make(chan string, 10)
	c := NewChunker(ch, 350)
	c.SetMaxLines(4)
	c.Write("e-eh?! good morning!\nhow are you?\n")
	c.Flush()
	got := drain(ch)
	if len(got) != 2 || c.Truncated() {
		t.Fatalf("got %q, truncated=%v", got, c.Truncated())
	}
}

// 0 keeps the old behaviour: no cap.
func TestChunkerLineCapZeroIsUnlimited(t *testing.T) {
	ch := make(chan string, 100)
	c := NewChunker(ch, 350)
	for range 10 {
		c.Write("line\n")
	}
	c.Flush()
	if got := drain(ch); len(got) != 10 || c.Truncated() {
		t.Fatalf("sent %d lines, truncated=%v; want 10, false", len(got), c.Truncated())
	}
}

// A big block arriving in one write must still come out as lines no longer than the limit,
// so no line is cut off by the server's 512-byte limit or slips past the line cap.
func TestChunkerSplitsOneBigWrite(t *testing.T) {
	ch := make(chan string, 100)
	c := NewChunker(ch, 50)
	c.Write(strings.Repeat("lorem ipsum ", 100))
	c.Flush()
	got := drain(ch)
	if len(got) < 20 {
		t.Fatalf("got %d lines, want the text split into ~24", len(got))
	}
	for _, line := range got {
		if len(line) > 50 {
			t.Fatalf("line of %d chars exceeds the 50 limit: %q", len(line), line)
		}
	}
}
