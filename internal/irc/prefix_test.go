// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"testing"

	"B4reMetal/metald/internal/config"
)

// Actions go out through withPrefix too, so "* alice slaps bob" on a shared account still shows it
// was the bot.
func TestWithPrefix(t *testing.T) {
	cfg := &config.Configuration{Bot: &config.BotConfig{ResponsePrefix: "[bot]"}, Server: &config.ServerConfig{}}
	if got := (ChatContext{Config: cfg}).withPrefix("slaps bob"); got != "[bot] slaps bob" {
		t.Errorf("got %q", got)
	}
	cfg.Bot.ResponsePrefix = ""
	if got := (ChatContext{Config: cfg}).withPrefix("slaps bob"); got != "slaps bob" {
		t.Errorf("no prefix configured: got %q", got)
	}
}
