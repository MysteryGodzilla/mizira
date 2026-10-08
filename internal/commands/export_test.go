// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

const exportConfig = `# my bot
nick: botty                    # the bot's nick
maxcontext: 6000               # history before folding
sessionduration: 15m
admins:                        # who runs her
  - "alice!*@*"                # me
openaikey: "sk-secret"

prompt: |
  You are botty.
  Be kind.

tool:
  - irc__slap
  - memory__remember
`

func TestExportConfig(t *testing.T) {
	useTempOverrides(t)
	for _, crlf := range []bool{false, true} {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.yml")
		text := exportConfig
		if crlf {
			text = strings.ReplaceAll(text, "\n", "\r\n")
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		saveOverrides(runtimeOverrides{})
		if _, _, err := ExportConfig(path, time.Now()); !errors.Is(err, ErrNothingToExport) {
			t.Fatalf("no overrides: %v", err)
		}

		admins := []string{"alice!*@*", "bob!*@*"}
		saveOverrides(runtimeOverrides{
			Set: map[string]string{"maxcontext": "8000", "sessionduration": "15m", "prompt": "You are botty.\nBe brave.",
				"selfnotes": "false", "openaikey": "sk-other"},
			Admins:   &admins,
			ToolsOff: []string{"irc__slap"}, ToolsOn: []string{"history__search"},
		})
		now := time.Date(2026, 10, 7, 14, 30, 0, 0, time.UTC)
		out, changes, err := ExportConfig(path, now)
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(out) != "config.yml.export-2026-10-07-1430" {
			t.Errorf("written to %s", out)
		}
		data, _ := os.ReadFile(out)
		got := string(data)
		if crlf != strings.Contains(got, "\r\n") {
			t.Errorf("line endings not kept (crlf %v)", crlf)
		}
		got = strings.ReplaceAll(got, "\r\n", "\n")
		for _, want := range []string{
			"# my bot\nnick: botty                    # the bot's nick\n",
			"maxcontext: 8000               # history before folding\n",
			"sessionduration: 15m\n",
			"admins:                        # who runs her\n  - alice!*@*\n  - bob!*@*\n",
			"prompt: |-\n  You are botty.\n  Be brave.\n",
			"tool:\n  - memory__remember\n  - history__search\n",
			"# Added by the console's export, 2026-10-07 14:30\nselfnotes: false\n",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("export lacks %q:\n%s", want, got)
			}
		}
		var parsed map[string]any
		if err := yaml.Unmarshal([]byte(got), &parsed); err != nil || parsed["maxcontext"] != 8000 || parsed["openaikey"] != "sk-other" {
			t.Errorf("parsed: %v %v", parsed, err)
		}
		keys := ""
		for _, c := range changes {
			keys += c.Key + " "
			if c.Key == "openaikey" && (c.From != "(hidden)" || c.To != "(hidden)") {
				t.Errorf("a secret shown: %+v", c)
			}
		}
		if keys != "admins maxcontext openaikey prompt selfnotes tool " {
			t.Errorf("changes: %s", keys)
		}
		if again, _, _ := ExportConfig(path, now); again == out {
			t.Error("a second export overwrote the first")
		}
		if orig, _ := os.ReadFile(path); string(orig) != text {
			t.Error("config.yml itself changed")
		}
	}
}

// A saved list that matches config.yml isn't reported as changed.
func TestListOverridesComparesWithConfig(t *testing.T) {
	useTempOverrides(t)
	path := filepath.Join(t.TempDir(), "config.yml")
	_ = os.WriteFile(path, []byte(exportConfig), 0o600)
	PersistAdmins([]string{"alice!*@*"})
	PersistBots([]string{"[otherbot]"}, nil)
	if got := strings.Join(ListOverrides(path), ","); got != "botprefixes" {
		t.Errorf("changed lists: %s", got)
	}
}

func TestResetAllKeepsRunState(t *testing.T) {
	cfg := settingsCfg(t)
	if _, err := SetSetting(cfg, nil, "maxreplylines", "7", "console:token", quiet); err != nil {
		t.Fatal(err)
	}
	PersistAdmins([]string{"bob!*@*"})
	PersistRunState(1)
	reset, later := ResetAll(cfg, nil, "console:token", quiet)
	if strings.Join(reset, ",") != "maxreplylines" || strings.Join(later, ",") != "admins" {
		t.Errorf("reset %v, on restart %v", reset, later)
	}
	o := loadOverrides()
	if len(o.Set) != 0 || o.Admins != nil || o.RunState == "" {
		t.Errorf("after: %+v", o)
	}
	if left := ListOverrides(filepath.Join(t.TempDir(), "none.yml")); len(left) != 0 {
		t.Errorf("list overrides left: %v", left)
	}
}
