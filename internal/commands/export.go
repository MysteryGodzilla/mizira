// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
)

// Exporting the runtime overrides as a config.yml: config.yml's own text with only the changed keys
// rewritten (comments and layout kept), written beside it and never over it. The file holds the
// secrets config.yml holds, so it stays on the machine; the console only learns its path and which
// keys changed.

// ExportChange is one key an export changed.
type ExportChange struct {
	Key, From, To string
}

var ErrNothingToExport = errors.New("nothing differs from config.yml")

// listOverrides are the list settings the overrides file can hold, with their config.yml keys.
func listOverrides(o runtimeOverrides) map[string]*[]string {
	return map[string]*[]string{"admins": o.Admins, "admintools": o.AdminTools, "screennicks": o.ScreenNicks,
		"filternicks": o.FilterNicks, "botprefixes": o.BotPrefixes, "botnicks": o.BotNicks}
}

// ExportConfig writes config.yml with every runtime override folded in, as configPath.export-<time>.
func ExportConfig(configPath string, now time.Time) (string, []ExportChange, error) {
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return "", nil, err
	}
	crlf := bytes.Contains(raw, []byte("\r\n"))
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	var before map[string]any
	if err := yaml.Unmarshal([]byte(text), &before); err != nil {
		return "", nil, fmt.Errorf("config.yml doesn't parse: %w", err)
	}

	overridesMu.Lock()
	o := loadOverrides()
	overridesMu.Unlock()

	want := map[string]any{}
	var changes []ExportChange
	for _, key := range sortedKeys(o.Set) {
		value := typedScalar(o.Set[key])
		if reflect.DeepEqual(value, before[key]) {
			continue
		}
		want[key] = value
		changes = append(changes, change(key, before[key], value))
	}
	for key, list := range listOverrides(o) {
		if list == nil {
			continue
		}
		value := anyList(*list)
		if reflect.DeepEqual(value, normaliseList(before[key])) {
			continue
		}
		want[key] = value
		changes = append(changes, change(key, before[key], value))
	}
	if len(o.ToolsOn) > 0 || len(o.ToolsOff) > 0 {
		var tools []string
		for _, t := range normaliseList(before["tools"]) {
			if s := fmt.Sprint(t); !slices.Contains(o.ToolsOff, s) {
				tools = append(tools, s)
			}
		}
		for _, t := range o.ToolsOn {
			if !slices.Contains(tools, t) {
				tools = append(tools, t)
			}
		}
		if value := anyList(tools); !reflect.DeepEqual(value, normaliseList(before["tools"])) {
			want["tools"] = value
			changes = append(changes, change("tools", before["tools"], value))
		}
	}
	if len(changes) == 0 {
		return "", nil, ErrNothingToExport
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Key < changes[j].Key })

	var appended []string
	for _, c := range changes {
		rendered, err := renderKey(c.Key, want[c.Key])
		if err != nil {
			return "", nil, err
		}
		var ok bool
		if text, ok = replaceKey(text, c.Key, rendered); !ok {
			appended = append(appended, rendered)
		}
	}
	if len(appended) > 0 {
		text = strings.TrimRight(text, "\n") + "\n\n# Added by the console's export, " + now.Format("2006-01-02 15:04") + "\n" +
			strings.Join(appended, "\n") + "\n"
	}

	// Proof before anything is written: every changed key reads back as intended, every other key
	// exactly as config.yml had it.
	var after map[string]any
	if err := yaml.Unmarshal([]byte(text), &after); err != nil {
		return "", nil, fmt.Errorf("the export doesn't parse: %w", err)
	}
	for key, v := range before {
		if w, changed := want[key]; changed {
			if !reflect.DeepEqual(normalise(after[key]), normalise(w)) {
				return "", nil, fmt.Errorf("the export didn't keep %s", key)
			}
		} else if !reflect.DeepEqual(after[key], v) {
			return "", nil, fmt.Errorf("the export changed %s, which it shouldn't have", key)
		}
	}
	for key, w := range want {
		if !reflect.DeepEqual(normalise(after[key]), normalise(w)) {
			return "", nil, fmt.Errorf("the export didn't keep %s", key)
		}
	}

	if crlf {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	path := configPath + ".export-" + now.Format("2006-01-02-1504")
	for i := 2; fileExists(path); i++ {
		path = fmt.Sprintf("%s.export-%s-%d", configPath, now.Format("2006-01-02-1504"), i)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return "", nil, err
	}
	return path, changes, nil
}

// typedScalar is a +set value as YAML would type it, so "8000" is written as a number and "15m" as
// plain text.
func typedScalar(raw string) any {
	if strings.Contains(raw, "\n") {
		return raw
	}
	var v any
	if err := yaml.Unmarshal([]byte(raw), &v); err == nil {
		switch v.(type) {
		case int, float64, bool:
			return v
		}
	}
	return raw
}

func anyList(list []string) []any {
	out := make([]any, 0, len(list))
	for _, s := range list {
		out = append(out, s)
	}
	return out
}

func normaliseList(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	return []any{}
}

// normalise treats a missing or empty list as the same value.
func normalise(v any) any {
	if l, ok := v.([]any); ok && len(l) == 0 {
		return nil
	}
	return v
}

func change(key string, from, to any) ExportChange {
	show := func(v any) string {
		if v == nil {
			return "(not set)"
		}
		if secretSetting(key) {
			return "(hidden)"
		}
		s := fmt.Sprint(v)
		if l, ok := v.([]any); ok {
			parts := make([]string, len(l))
			for i, x := range l {
				parts[i] = fmt.Sprint(x)
			}
			s = "[" + strings.Join(parts, ", ") + "]"
		}
		s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ⏎ ")
		if r := []rune(s); len(r) > 120 {
			s = string(r[:120]) + "…"
		}
		return s
	}
	return ExportChange{Key: key, From: show(from), To: show(to)}
}

func renderKey(key string, value any) (string, error) {
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(map[string]any{key: value}); err != nil {
		return "", err
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

var trailingComment = regexp.MustCompile(`\s+#[^"']*$`)

// replaceKey swaps a top-level key and its indented block for rendered, keeping the comment on the
// key's line. False if config.yml has no such key.
func replaceKey(text, key, rendered string) (string, bool) {
	lines := strings.Split(text, "\n")
	start := -1
	for i, l := range lines {
		if strings.HasPrefix(l, key+":") && (len(l) == len(key)+1 || l[len(key)+1] == ' ' || l[len(key)+1] == '\t') {
			start = i
			break
		}
	}
	if start < 0 {
		return text, false
	}
	end := start + 1
	for k := start + 1; k < len(lines); k++ {
		l := lines[k]
		if strings.TrimSpace(l) == "" {
			continue
		}
		if l[0] == ' ' || l[0] == '\t' || strings.HasPrefix(l, "- ") {
			end = k + 1
			continue
		}
		break
	}
	newLines := strings.Split(rendered, "\n")
	if c := trailingComment.FindString(strings.TrimPrefix(lines[start], key+":")); c != "" {
		newLines[0] += c
	}
	out := append(append(slices.Clone(lines[:start]), newLines...), lines[end:]...)
	return strings.Join(out, "\n"), true
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// ResetAll puts every +set setting back to config.yml's value now, and drops the list overrides
// (admins, screening, bots, admin-only tools), which go back to config.yml at the next restart. Tool
// switches are reset by the caller. A paused or stopped state is kept: it isn't a setting.
func ResetAll(cfg *config.Configuration, sys core.System, by string, log *slog.Logger) (reset, onRestart []string) {
	overridesMu.Lock()
	o := loadOverrides()
	overridesMu.Unlock()
	for _, key := range sortedKeys(o.Set) {
		if _, err := ResetSetting(cfg, sys, key, by, log); err != nil {
			PersistUnset(key)
			onRestart = append(onRestart, key)
			continue
		}
		reset = append(reset, key)
	}
	overridesMu.Lock()
	o = loadOverrides()
	for key, list := range listOverrides(o) {
		if list != nil {
			onRestart = append(onRestart, key)
		}
	}
	o.Admins, o.AdminTools, o.ScreenNicks, o.FilterNicks, o.BotPrefixes, o.BotNicks = nil, nil, nil, nil, nil, nil
	saveOverrides(o)
	overridesMu.Unlock()
	sort.Strings(onRestart)
	log.Info("config_reset_all", "reset", reset, "on_restart", onRestart, "by", by)
	return reset, onRestart
}

// ListOverrides names the list settings whose saved value differs from config.yml's (at configPath).
func ListOverrides(configPath string) []string {
	overridesMu.Lock()
	o := loadOverrides()
	overridesMu.Unlock()
	var before map[string]any
	if raw, err := os.ReadFile(configPath); err == nil {
		_ = yaml.Unmarshal(raw, &before)
	}
	var out []string
	for key, list := range listOverrides(o) {
		if list != nil && !reflect.DeepEqual(anyList(*list), normaliseList(before[key])) {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}
