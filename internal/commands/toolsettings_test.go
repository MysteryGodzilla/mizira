// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestToolSettingsChangeAndReset(t *testing.T) {
	old := OverridesPath
	OverridesPath = filepath.Join(t.TempDir(), "overrides.json")
	t.Cleanup(func() { OverridesPath = old })
	t.Setenv("IMAGE_GEN_PER_HOUR", "5")
	t.Setenv("IMAGE_EDIT_HOSTS", "")
	applyToolEnvOverrides(runtimeOverrides{})

	if v, err := SetToolSetting("IMAGE_GEN_PER_HOUR", " 8 "); err != nil || v != "8" || os.Getenv("IMAGE_GEN_PER_HOUR") != "8" {
		t.Fatalf("set = %q, %v, env %q", v, err, os.Getenv("IMAGE_GEN_PER_HOUR"))
	}
	if v, err := SetToolSetting("IMAGE_EDIT_HOSTS", "Files.Example.com, ,img.example.org"); err != nil || v != "files.example.com,img.example.org" {
		t.Errorf("hosts = %q, %v", v, err)
	}
	for _, c := range [][2]string{
		{"IMAGE_GEN_PER_HOUR", "lots"}, {"IMAGE_GEN_PER_HOUR", "-1"}, {"IMAGE_GEN_EXPIRE", "forever"},
		{"IMAGE_EDIT_HOSTS", "http://x.example.com/"}, {"IMAGE_GEN_ANNOUNCE", "two\nlines"},
		{"RUNPOD_API_KEY", "x"}, {"IMAGE_PROMPT_POLICY", "anything goes"},
	} {
		if _, err := SetToolSetting(c[0], c[1]); err == nil {
			t.Errorf("%s = %q was accepted", c[0], c[1])
		}
	}

	// Kept across a restart: the next start puts it back in the environment.
	t.Setenv("IMAGE_GEN_PER_HOUR", "5")
	applyToolEnvOverrides(loadOverrides())
	if os.Getenv("IMAGE_GEN_PER_HOUR") != "8" {
		t.Errorf("after restart = %q", os.Getenv("IMAGE_GEN_PER_HOUR"))
	}
	for _, s := range ToolSettingValues() {
		if s.Key == "IMAGE_GEN_PER_HOUR" && (!s.Overridden || s.Default != "5" || s.Value != "8") {
			t.Errorf("listed as %+v", s)
		}
	}

	if v, err := ResetToolSetting("IMAGE_GEN_PER_HOUR"); err != nil || v != "5" || os.Getenv("IMAGE_GEN_PER_HOUR") != "5" {
		t.Errorf("reset = %q, %v", v, err)
	}
	if _, ok := loadOverrides().Env["IMAGE_GEN_PER_HOUR"]; ok {
		t.Error("reset left the override saved")
	}
}

func TestImageStylesSetting(t *testing.T) {
	old := OverridesPath
	OverridesPath = filepath.Join(t.TempDir(), "overrides.json")
	t.Cleanup(func() { OverridesPath = old })
	t.Setenv("IMAGE_STYLES", "")
	applyToolEnvOverrides(runtimeOverrides{})

	sha := strings.Repeat("a", 64)
	styles := "soyuz | draw | 1 | soyuzmult | Soviet cartoon | https://civitai.com/x | " + sha + "\r\n" +
		"clay | edit | 1 | Transform the image into a clay sculpture style | clay figure | https://huggingface.co/y | " + sha
	if v, err := SetToolSetting("IMAGE_STYLES", styles); err != nil || strings.Count(v, "\n") != 1 || strings.Contains(v, "\r") {
		t.Fatalf("set = %q, %v", v, err)
	}
	for _, bad := range []string{"soyuz | draw | 1 | Soviet cartoon", "x1 | draw | 1 | | d | https://evil.example.com/x | " + sha, "a\x07b"} {
		if _, err := SetToolSetting("IMAGE_STYLES", bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}

	conf := "nick: botty\n\nenv:\n  IMAGE_STYLES: \"\"\n  IMAGE_GEN_PER_HOUR: \"5\"\n"
	out, err := setEnvKey(conf, "IMAGE_STYLES", os.Getenv("IMAGE_STYLES"))
	if err != nil {
		t.Fatal(err)
	}
	var got struct{ Env map[string]string }
	if err := yaml.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got.Env["IMAGE_STYLES"] != os.Getenv("IMAGE_STYLES") || got.Env["IMAGE_GEN_PER_HOUR"] != "5" {
		t.Errorf("exported:\n%s", out)
	}
}
