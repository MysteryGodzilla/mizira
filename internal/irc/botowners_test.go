// SPDX-License-Identifier: GPL-3.0-only

package irc

import "testing"

// A tagged bot's owner is whoever last posted its lines, per network; untagged lines and unknown
// names give nothing.
func TestBotOwner(t *testing.T) {
	cfg := botConfig("[botnick]", "[otherbot]")
	cfg.Server.Name = "owners-test"
	rememberBotOwner(cfg, "carol", "plain chat")
	if got := BotOwner(cfg, "otherbot"); got != "" {
		t.Errorf("owner before any tagged line: %q", got)
	}
	rememberBotOwner(cfg, "bob", "[otherbot] hi")
	rememberBotOwner(cfg, "dave", "[otherbot] hi again")
	for name, want := range map[string]string{"otherbot": "dave", "[OtherBot]": "dave", "carol": "", "": ""} {
		if got := BotOwner(cfg, name); got != want {
			t.Errorf("BotOwner(%q) = %q, want %q", name, got, want)
		}
	}
	other := botConfig("[botnick]", "[otherbot]")
	other.Server.Name = "owners-test-2"
	if got := BotOwner(other, "otherbot"); got != "" {
		t.Errorf("owner leaked across networks: %q", got)
	}
}
