// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"strings"
	"testing"
)

func TestParseImageStyles(t *testing.T) {
	sha := strings.Repeat("A", 64)
	good := "soyuz | draw | 0.9 | soyuzmult | Soviet cartoon | https://civitai.com/api/download/models/1 | " + sha + "\n" +
		"# comment\n\n" +
		"clay | edit | 1 | Transform the image into a clay sculpture style | clay figure | https://huggingface.co/x/y/resolve/r/c.safetensors | " + sha
	styles, err := ParseImageStyles(good)
	if err != nil || len(styles) != 2 {
		t.Fatalf("styles = %+v, err = %v", styles, err)
	}
	if s := styles[0]; s.Name != "soyuz" || s.Kind != "draw" || s.Strength != 0.9 || s.Trigger != "soyuzmult" ||
		s.Description != "Soviet cartoon" || s.SHA256 != strings.Repeat("a", 64) {
		t.Errorf("soyuz = %+v", s)
	}
	guided, err := ParseImageStyles("angle | edit | 1 | <qma> {prompt} | camera moved | https://huggingface.co/x | " + sha + " | '<view> <height> <distance>'\n" +
		"outfit | edit2 | 1 | Dress the person in image 1 in the {prompt} shown in image 2. | outfit swap | https://huggingface.co/y | " + sha)
	if err != nil || len(guided) != 2 || guided[0].Guide != "'<view> <height> <distance>'" || guided[1].Kind != "edit2" || guided[1].Guide != "" {
		t.Errorf("guided = %+v, err = %v", guided, err)
	}
	for name, bad := range map[string]string{
		"missing fields": "soyuz | draw | 1 | Soviet cartoon",
		"other host":     "x1 | draw | 1 | | d | https://evil.example.com/x | " + sha,
		"too strong":     "x1 | draw | 3 | | d | https://civitai.com/x | " + sha,
		"bad kind":       "x1 | both | 1 | | d | https://civitai.com/x | " + sha,
		"capital name":   "Soyuz | draw | 1 | | d | https://civitai.com/x | " + sha,
		"twice":          good + "\nsoyuz | draw | 1 | | d | https://civitai.com/x | " + sha,
	} {
		if _, err := ParseImageStyles(bad); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	// The PC-98 LoRA needs cfg 2.5 where every other style runs at the usual 1.
	cfg, err := ParseImageStyles("pc98 | draw | 0.8 cfg 2.5 | pc98dither | PC-98 art | https://civitai.com/p | " + sha)
	if err != nil || len(cfg) != 1 || cfg[0].Strength != 0.8 || cfg[0].CFG != 2.5 || cfg[0].Trigger != "pc98dither" || styles[0].CFG != 0 {
		t.Errorf("cfg = %+v, err = %v", cfg, err)
	}
	if _, err := ParseImageStyles("pc98 | draw | 0.8 cfg 40 | | d | https://civitai.com/p | " + sha); err == nil {
		t.Error("cfg 40 accepted")
	}

	// A friend's LoRA on their own host: listed, it's allowed; look-alikes still aren't.
	t.Setenv("IMAGE_STYLE_HOSTS", " Files.Example.com ,")
	if _, err := ParseImageStyles("x1 | draw | 1 | | d | https://files.example.com/u/x.safetensors | " + sha); err != nil {
		t.Errorf("listed host refused: %v", err)
	}
	for _, url := range []string{"https://files.example.com.evil.example/x", "https://files.example.com@evil.example/x",
		"https://civitai.com.evil.example/x"} {
		if _, err := ParseImageStyles("x1 | draw | 1 | | d | " + url + " | " + sha); err == nil {
			t.Errorf("%s: accepted", url)
		}
	}
}
