// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"slices"
	"testing"

	"B4reMetal/metald/internal/config"
	mocktest "B4reMetal/metald/internal/testing"
)

// After an export lands in config.yml, a switch it agrees with is dropped at startup and stops
// showing as a difference; one config.yml still disagrees with is kept and applied.
func TestApplyOverridesDropsAgreedToolSwitches(t *testing.T) {
	useTempOverrides(t)
	saveOverrides(runtimeOverrides{
		ToolsOn:  []string{"plugins/websearch.py", "plugins/webfetch.py"},
		ToolsOff: []string{"irc__slap", "irc__op"},
	})
	cfg := mocktest.DefaultTestConfig()
	cfg.Bot.Tools = []string{"memory__remember", "irc__op", "plugins/websearch.py"}
	ApplyOverrides(cfg)

	on, off := ToolOverrides()
	if !slices.Equal(on, []string{"plugins/webfetch.py"}) || !slices.Equal(off, []string{"irc__op"}) {
		t.Errorf("switches left: on %q, off %q", on, off)
	}
	want := []string{"memory__remember", "plugins/websearch.py", "plugins/webfetch.py"}
	if got := config.List(&cfg.Bot.Tools); !slices.Equal(got, want) {
		t.Errorf("tools = %q, want %q", got, want)
	}
}
