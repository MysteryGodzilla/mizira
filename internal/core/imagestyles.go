// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"fmt"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ImageStyle is one named picture style from IMAGE_STYLES, read the same way by plugins/imagegen.py.
type ImageStyle struct {
	Name, Kind, Trigger, Description, URL, SHA256, Guide string
	Strength                                             float64
}

// One style a line: name | draw, edit or edit2 | strength | trigger | description | url | sha256 [| guide].
var styleLine = regexp.MustCompile(`^([a-z0-9]{2,20})\s*\|\s*(draw|edit2|edit)\s*\|\s*([0-9]+(?:\.[0-9]+)?)\s*\|([^|]*)\|([^|]+)\|\s*(https://\S+)\s*\|\s*([0-9a-fA-F]{64})\s*(?:\|([^|]*))?$`)

// styleHosts are where the worker may fetch a style's LoRA from: Civitai, Hugging Face and any in
// IMAGE_STYLE_HOSTS (the worker checks this too, against its own EXTRA_LORA_HOSTS).
func styleHosts() []string {
	hosts := []string{"civitai.com", "huggingface.co"}
	for _, h := range strings.Split(os.Getenv("IMAGE_STYLE_HOSTS"), ",") {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

// ParseImageStyles reads IMAGE_STYLES, skipping blank lines and # comments. A malformed line is an
// error naming its line number; the styles before it are still returned.
func ParseImageStyles(text string) ([]ImageStyle, error) {
	var out []ImageStyle
	seen := map[string]bool{}
	hosts := styleHosts()
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := styleLine.FindStringSubmatch(line)
		if m == nil {
			return out, fmt.Errorf("line %d: want name | draw, edit or edit2 | strength | trigger | description | url | sha256 [| guide]", i+1)
		}
		strength, _ := strconv.ParseFloat(m[3], 64)
		switch {
		case strength <= 0 || strength > 2:
			return out, fmt.Errorf("line %d: strength must be above 0 and at most 2", i+1)
		case !slices.Contains(hosts, urlHost(m[6])):
			return out, fmt.Errorf("line %d: the LoRA must come from %s", i+1, strings.Join(hosts, ", "))
		case seen[m[1]]:
			return out, fmt.Errorf("line %d: %s is listed twice", i+1, m[1])
		}
		seen[m[1]] = true
		out = append(out, ImageStyle{Name: m[1], Kind: m[2], Strength: strength, Trigger: strings.TrimSpace(m[4]),
			Description: strings.TrimSpace(m[5]), URL: m[6], SHA256: strings.ToLower(m[7]), Guide: strings.TrimSpace(m[8])})
	}
	return out, nil
}

func urlHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}
