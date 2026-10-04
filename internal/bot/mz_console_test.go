// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package bot

import (
	"testing"

	"B4reMetal/metald/internal/config"
)

func TestConsoleThinking(t *testing.T) {
	for effort, on := range map[string]bool{"": false, "off": false, "none": false, "None": false, "low": true, "high": true} {
		c := console{cfg: &config.Configuration{Model: &config.ModelConfig{ThinkingEffort: effort}}}
		if c.Thinking() != on {
			t.Errorf("thinkingeffort %q: Thinking() = %v, want %v", effort, !on, on)
		}
	}
}
