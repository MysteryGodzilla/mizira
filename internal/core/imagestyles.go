// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ImageStyle is one named picture style from IMAGE_STYLES, read the same way by plugins/imagegen.py.
type ImageStyle struct {
	Name, Kind, Trigger, Description, URL, SHA256 string
	Strength                                      float64
}

// One style a line: name | draw or edit | strength | trigger | description | url | sha256.
var styleLine = regexp.MustCompile(`^([a-z0-9]{2,20})\s*\|\s*(draw|edit)\s*\|\s*([0-9]+(?:\.[0-9]+)?)\s*\|([^|]*)\|([^|]+)\|\s*(https://\S+)\s*\|\s*([0-9a-fA-F]{64})\s*$`)

// styleHosts are where the worker may fetch a style's LoRA from (the worker checks this too).
var styleHosts = []string{"https://civitai.com/", "https://huggingface.co/"}

// ParseImageStyles reads IMAGE_STYLES, skipping blank lines and # comments. A malformed line is an
// error naming its line number; the styles before it are still returned.
func ParseImageStyles(text string) ([]ImageStyle, error) {
	var out []ImageStyle
	seen := map[string]bool{}
	for i, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := styleLine.FindStringSubmatch(line)
		if m == nil {
			return out, fmt.Errorf("line %d: want name | draw or edit | strength | trigger | description | url | sha256", i+1)
		}
		strength, _ := strconv.ParseFloat(m[3], 64)
		switch {
		case strength <= 0 || strength > 2:
			return out, fmt.Errorf("line %d: strength must be above 0 and at most 2", i+1)
		case !hasAnyPrefix(m[6], styleHosts):
			return out, fmt.Errorf("line %d: the LoRA must come from civitai.com or huggingface.co", i+1)
		case seen[m[1]]:
			return out, fmt.Errorf("line %d: %s is listed twice", i+1, m[1])
		}
		seen[m[1]] = true
		out = append(out, ImageStyle{Name: m[1], Kind: m[2], Strength: strength, Trigger: strings.TrimSpace(m[4]),
			Description: strings.TrimSpace(m[5]), URL: m[6], SHA256: strings.ToLower(m[7])})
	}
	return out, nil
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
