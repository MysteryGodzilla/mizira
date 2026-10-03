// Copyright (C) 2023-2026 Alex Schlessinger and soulshack contributors
// Modified 2026 by BareMetal
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"bytes"
	"strings"
)

// Chunker handles chunking of content for IRC message limits.
// It buffers content and emits complete lines or chunks when the buffer
// exceeds the maximum chunk size.
type Chunker struct {
	output       chan<- string
	buffer       *bytes.Buffer
	maxChunkSize int

	// maxLines caps how many lines one reply may post (0 = no cap). A small model asked for a
	// long story ignores "keep it short" in its prompt; this makes the limit a guarantee.
	maxLines  int
	sent      int
	truncated bool

	// hold, when set, keeps back a line it returns true for and every line after it, so a reply
	// stays in order while the caller decides whether to Release or DropHeld.
	hold func(line string) bool
	held []string

	joinLines bool // see SetJoinLines
}

// SetHold installs the check that decides which lines to keep back.
func (c *Chunker) SetHold(hold func(line string) bool) { c.hold = hold }

// Holding reports whether lines are being kept back.
func (c *Chunker) Holding() bool { return len(c.held) > 0 }

// Release sends the lines kept back, in order.
func (c *Chunker) Release() {
	held := c.held
	c.held = nil
	for _, line := range held {
		c.send(line)
	}
}

// DropHeld throws away the lines kept back and returns them.
func (c *Chunker) DropHeld() []string {
	held := c.held
	c.held = nil
	return held
}

// SetMaxLines caps how many lines this chunker will emit. Anything after that is dropped.
func (c *Chunker) SetMaxLines(n int) { c.maxLines = n }

// Sent reports how many lines have gone out.
func (c *Chunker) Sent() int { return c.sent }

// Truncated reports whether lines were dropped because of the line cap.
func (c *Chunker) Truncated() bool { return c.truncated }

// emit sends one line, unless the line cap has been reached. A leading speaker tag the model
// copied from the input format is removed first; a line that was only a tag is dropped.
func (c *Chunker) emit(line string) {
	if line = StripSpeakerTags(line); isBlankLine(line) {
		return
	}
	// hold runs on every line, so a second claim behind the first is still seen.
	if (c.hold != nil && c.hold(line)) || c.Holding() {
		c.held = append(c.held, line)
		return
	}
	c.send(line)
}

// send posts one line, unless the line cap has been reached.
func (c *Chunker) send(line string) {
	if c.maxLines > 0 && c.sent >= c.maxLines {
		c.truncated = true
		return
	}
	c.sent++
	c.output <- line
}

// NewChunker creates a new IRC chunker that writes to the given output channel.
func NewChunker(output chan<- string, maxChunkSize int) *Chunker {
	return &Chunker{
		output:       output,
		buffer:       &bytes.Buffer{},
		maxChunkSize: maxChunkSize,
	}
}

// Write adds content to the buffer and emits complete lines immediately.
// If the buffer grows too large, it forces a chunk to be emitted.
func (c *Chunker) Write(content string) {
	if c.joinLines {
		c.writeJoined(content)
		return
	}
	c.buffer.WriteString(content)

	// Emit complete lines immediately
	for {
		line, err := c.buffer.ReadString('\n')
		if err != nil {
			// No more complete lines, put back what we read
			if line != "" {
				c.buffer.WriteString(line)
			}
			break
		}
		// Remove the newline and send
		if line = strings.TrimSuffix(line, "\n"); line != "" {
			c.emit(line)
		}
	}

	// If the buffer is too large, force chunks until it fits. A loop, not a single split: a big
	// block arriving in one write must not leave an oversized remainder for Flush.
	for c.buffer.Len() >= c.maxChunkSize {
		chunk := c.extractBestSplitChunk()
		if chunk == "" {
			break
		}
		c.emit(chunk)
	}
}

func (c *Chunker) extractBestSplitChunk() string {
	if c.buffer.Len() == 0 {
		return ""
	}

	data := c.buffer.Bytes()
	end := min(c.maxChunkSize, len(data))

	// Try to find a space within the allowed range to break cleanly
	if idx := bytes.LastIndexByte(data[:end], ' '); idx > 0 {
		chunk := string(data[:idx])
		c.buffer.Next(idx + 1) // Skip the space itself
		return chunk
	}

	// If no space is found, hard break at maxChunkSize
	chunk := string(data[:end])
	c.buffer.Next(end)
	return chunk
}

// Flush emits any remaining buffer content, still split at maxChunkSize.
func (c *Chunker) Flush() {
	if c.joinLines {
		c.drainJoined(true)
		return
	}
	for c.buffer.Len() > c.maxChunkSize {
		chunk := c.extractBestSplitChunk()
		if chunk == "" {
			break
		}
		c.emit(chunk)
	}
	if c.buffer.Len() > 0 {
		c.emit(c.buffer.String())
		c.buffer.Reset()
	}
}

// Discard drops the buffered content without emitting it, returning how many bytes were thrown
// away.
func (c *Chunker) Discard() int {
	n := c.buffer.Len()
	c.buffer.Reset()
	for _, line := range c.DropHeld() {
		n += len(line)
	}
	return n
}
