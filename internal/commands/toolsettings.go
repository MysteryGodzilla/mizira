// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"B4reMetal/metald/internal/core"
)

// Tool settings (env: in config.yml) the console may change. A plugin reads its settings when it
// starts, and each call starts it afresh, so a change applies from the next call. Only these keys:
// credentials, hosts the bot dials and safety policies stay in config.yml.

// ToolSetting is one tool setting the console may change.
type ToolSetting struct {
	Key   string
	Group string
	Kind  string // "number", "text" or "lines"
	Help  string
}

var ToolSettings = []ToolSetting{
	{"IMAGE_GEN_PER_HOUR", "Pictures", "number", "pictures one person may have in an hour (0: no cap)"},
	{"IMAGE_GEN_PER_DAY", "Pictures", "number", "pictures one person may have in 24 hours (0: no cap)"},
	{"IMAGE_GEN_DAY_TOTAL", "Pictures", "number", "pictures everyone together may have in 24 hours (0: no cap)"},
	{"IMAGE_GEN_NEGATIVE", "Pictures", "text", "kept out of every picture, comma-separated"},
	{"IMAGE_GEN_ANNOUNCE", "Pictures", "text", "her /me when she starts a picture"},
	{"IMAGE_GEN_ANNOUNCE_EDIT", "Pictures", "text", "her /me when she changes an earlier picture"},
	{"IMAGE_GEN_ANNOUNCE_COLD", "Pictures", "text", "her /me when the image server has to start up first"},
	{"IMAGE_GEN_EXPIRE", "Pictures", "text", "how long pictures stay up, e.g. 30d (empty: the host's default)"},
	{"IMAGE_EDIT_HOSTS", "Pictures", "text", "hosts whose pictures can be edited, comma-separated (empty: no editing)"},
	{"IMAGE_STYLES", "Pictures", "lines", "named styles, one a line: name | draw or edit | strength | trigger | description | LoRA url | sha256"},
	{"VISION_ANNOUNCE", "Looking at pictures", "text", "her /me when she starts looking at a picture someone linked"},
}

// ToolSettingValue is a tool setting's value now and config.yml's.
type ToolSettingValue struct {
	ToolSetting
	Value, Default string
	Overridden     bool
}

const (
	maxToolSetting      = 500
	maxToolSettingLines = 12000
)

var (
	toolDefaultsMu sync.Mutex
	toolDefaults   = map[string]string{} // config.yml's values, before any override
)

var (
	expireValue = regexp.MustCompile(`^([0-9]+[a-z]+)?$`)
	hostName    = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
)

func toolSetting(key string) (ToolSetting, bool) {
	for _, s := range ToolSettings {
		if s.Key == key {
			return s, true
		}
	}
	return ToolSetting{}, false
}

// applyToolEnvOverrides notes config.yml's values, then puts the console's changes in the environment
// the plugins inherit.
func applyToolEnvOverrides(o runtimeOverrides) {
	toolDefaultsMu.Lock()
	defer toolDefaultsMu.Unlock()
	for _, s := range ToolSettings {
		toolDefaults[s.Key] = os.Getenv(s.Key)
	}
	for key, value := range o.Env {
		if _, ok := toolSetting(key); !ok {
			core.GetLogger().Warn("override_unknown_key", "key", key)
			continue
		}
		os.Setenv(key, value)
		core.GetLogger().Info("override_applied", "key", key, "value", value)
	}
}

// ToolSettingValues lists the tool settings with their values now.
func ToolSettingValues() []ToolSettingValue {
	overridesMu.Lock()
	o := loadOverrides()
	overridesMu.Unlock()
	toolDefaultsMu.Lock()
	defer toolDefaultsMu.Unlock()
	out := make([]ToolSettingValue, 0, len(ToolSettings))
	for _, s := range ToolSettings {
		_, changed := o.Env[s.Key]
		out = append(out, ToolSettingValue{ToolSetting: s, Value: os.Getenv(s.Key), Default: toolDefaults[s.Key], Overridden: changed})
	}
	return out
}

// checkToolSetting returns the value to store, or why it can't be.
func checkToolSetting(s ToolSetting, value string) (string, error) {
	value = strings.TrimSpace(value)
	if s.Kind == "lines" {
		return checkToolLines(s, value)
	}
	if len(value) > maxToolSetting {
		return "", fmt.Errorf("at most %d characters", maxToolSetting)
	}
	if strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("one line, no control characters")
	}
	switch {
	case s.Kind == "number":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > 10000 {
			return "", fmt.Errorf("a whole number from 0 to 10000")
		}
		return strconv.Itoa(n), nil
	case s.Key == "IMAGE_GEN_EXPIRE":
		if !expireValue.MatchString(strings.ToLower(value)) {
			return "", fmt.Errorf("a number and a unit, e.g. 30d or 12h, or empty")
		}
		return strings.ToLower(value), nil
	case s.Key == "IMAGE_EDIT_HOSTS":
		var hosts []string
		for _, h := range strings.Split(strings.ToLower(value), ",") {
			if h = strings.TrimSpace(h); h == "" {
				continue
			}
			if !hostName.MatchString(h) {
				return "", fmt.Errorf("%q isn't a host name like files.example.com", h)
			}
			hosts = append(hosts, h)
		}
		return strings.Join(hosts, ","), nil
	}
	return value, nil
}

// checkToolLines checks a several-line setting: no control characters but line breaks, and for
// IMAGE_STYLES every line a valid style.
func checkToolLines(s ToolSetting, value string) (string, error) {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	if len(value) > maxToolSettingLines {
		return "", fmt.Errorf("at most %d characters", maxToolSettingLines)
	}
	if strings.IndexFunc(value, func(r rune) bool { return r != '\n' && unicode.IsControl(r) }) >= 0 {
		return "", fmt.Errorf("no control characters")
	}
	if s.Key == "IMAGE_STYLES" {
		if _, err := core.ParseImageStyles(value); err != nil {
			return "", err
		}
	}
	return value, nil
}

// SetToolSetting changes a tool setting now and keeps it across restarts.
func SetToolSetting(key, value string) (string, error) {
	s, ok := toolSetting(key)
	if !ok {
		return "", fmt.Errorf("%s isn't a setting the console can change", key)
	}
	value, err := checkToolSetting(s, value)
	if err != nil {
		return "", fmt.Errorf("%s: %w", key, err)
	}
	overridesMu.Lock()
	defer overridesMu.Unlock()
	o := loadOverrides()
	if o.Env == nil {
		o.Env = map[string]string{}
	}
	o.Env[key] = value
	saveOverrides(o)
	os.Setenv(key, value)
	return value, nil
}

// ResetToolSetting puts a tool setting back to config.yml's value.
func ResetToolSetting(key string) (string, error) {
	if _, ok := toolSetting(key); !ok {
		return "", fmt.Errorf("%s isn't a setting the console can change", key)
	}
	overridesMu.Lock()
	o := loadOverrides()
	if _, ok := o.Env[key]; ok {
		delete(o.Env, key)
		saveOverrides(o)
	}
	overridesMu.Unlock()
	toolDefaultsMu.Lock()
	value := toolDefaults[key]
	toolDefaultsMu.Unlock()
	if value == "" {
		os.Unsetenv(key)
	} else {
		os.Setenv(key, value)
	}
	return value, nil
}
