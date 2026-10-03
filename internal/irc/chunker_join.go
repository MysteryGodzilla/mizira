// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"regexp"
	"strings"
)

// listItem starts a line that must stay on its own: "- a", "* a", "• a", "1. a", "2) a".
var listItem = regexp.MustCompile(`^\s*([-*•]|\d+[.)])\s`)

// SetJoinLines makes the chunker join a reply's lines into as few IRC messages as fit, so a short
// answer is one message rather than one per line. List items and code blocks keep their own lines.
func (c *Chunker) SetJoinLines(join bool) { c.joinLines = join }

// writeJoined buffers content and sends every finished group of lines; the last group stays
// buffered, since the next write may still continue it.
func (c *Chunker) writeJoined(content string) {
	c.buffer.WriteString(content)
	c.drainJoined(false)
}

// drainJoined sends the buffered groups. Unless final, the last one is kept, but cut down to less
// than maxChunkSize so a long paragraph still goes out as it streams.
func (c *Chunker) drainJoined(final bool) {
	groups := lineGroups(c.buffer.String())
	c.buffer.Reset()
	if len(groups) == 0 {
		return
	}
	keep := ""
	if !final {
		keep = groups[len(groups)-1]
		groups = groups[:len(groups)-1]
	}
	for _, g := range groups {
		c.sendSized(joinGroup(g))
	}
	if keep == "" {
		return
	}
	// A raw tail still ending in a newline may yet start a list; leave it whole.
	if !strings.HasSuffix(keep, "\n") {
		for joined := joinGroup(keep); len(joined) >= c.maxChunkSize; joined = joinGroup(keep) {
			cut := strings.LastIndexByte(joined[:c.maxChunkSize], ' ')
			if cut <= 0 {
				cut = c.maxChunkSize
			}
			c.emit(joined[:cut])
			keep = strings.TrimLeft(joined[cut:], " ")
		}
	}
	c.buffer.WriteString(keep)
}

// sendSized emits one joined group, split at spaces to fit maxChunkSize.
func (c *Chunker) sendSized(s string) {
	for len(s) > c.maxChunkSize {
		cut := strings.LastIndexByte(s[:c.maxChunkSize], ' ')
		if cut <= 0 {
			cut = c.maxChunkSize
		}
		c.emit(s[:cut])
		s = strings.TrimLeft(s[cut:], " ")
	}
	if strings.TrimSpace(s) != "" {
		c.emit(s)
	}
}

// lineGroups splits raw text into groups of lines that may be joined. A list item, or a line in a
// ``` code block, is a group of its own, and so is the line after a list item.
func lineGroups(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.SplitAfter(text, "\n")
	var groups []string
	cur := ""
	inCode, prevList := false, false
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		fence := strings.HasPrefix(trimmed, "```")
		alone := inCode || fence || listItem.MatchString(l)
		if (alone || prevList) && strings.TrimSpace(cur) != "" {
			groups = append(groups, cur)
			cur = ""
		}
		cur += l
		if alone {
			groups = append(groups, cur)
			cur = ""
		}
		if fence {
			inCode = !inCode
		}
		prevList = listItem.MatchString(l)
	}
	if cur != "" {
		groups = append(groups, cur)
	}
	return groups
}

// joinGroup cleans each line (CleanReplyLine), then joins them with single spaces.
func joinGroup(g string) string {
	var parts []string
	for _, l := range strings.Split(g, "\n") {
		if l = CleanReplyLine(l); l != "" {
			parts = append(parts, l)
		}
	}
	return strings.Join(parts, " ")
}
