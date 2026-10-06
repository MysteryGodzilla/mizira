// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package bot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/alexschlessinger/pollytool/tools"

	"B4reMetal/metald/internal/admin"
	"B4reMetal/metald/internal/commands"
	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// toolSwitchMu keeps two switches of the same tool from interleaving their load and unload.
var toolSwitchMu sync.Mutex

// The tool list's entries the console offers: the bot's own tools (background work as one switch,
// task__start, which brings the rest of irc.TaskToolset), the plugins next to pluginlib and in
// custom-plugins, and anything else config.yml lists. Never a path typed into the page.
func (c console) toolSpecs() []string {
	var specs []string
	for _, name := range irc.NativeToolNames() {
		if name == "task__start" || !slices.Contains(irc.TaskToolset, name) {
			specs = append(specs, name)
		}
	}
	plugins := filepath.Dir(c.cfg.Bot.PluginLib)
	for _, dir := range []string{plugins, filepath.Join(filepath.Dir(plugins), "custom-plugins")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if ext := filepath.Ext(e.Name()); !e.IsDir() && (ext == ".py" || ext == ".sh") {
				specs = append(specs, filepath.ToSlash(filepath.Join(dir, e.Name())))
			}
		}
	}
	for _, spec := range append(commands.ConfigTools(), c.cfg.Bot.Tools...) {
		if !slices.ContainsFunc(specs, func(s string) bool { return sameSpec(s, spec) }) {
			specs = append(specs, spec)
		}
	}
	return specs
}

// sameSpec compares tool list entries, so "plugins/x.py" and "plugins\x.py" are one plugin.
func sameSpec(a, b string) bool { return filepath.Clean(a) == filepath.Clean(b) }

// specTools are the loaded tools that came from spec.
func specTools(reg *tools.ToolRegistry, spec string) []tools.Tool {
	var out []tools.Tool
	for _, t := range reg.All() {
		switch {
		case spec == "task__start" && slices.Contains(irc.TaskToolset, t.GetName()),
			t.GetName() == spec,
			t.GetSource() != "builtin" && sameSpec(t.GetSource(), spec):
			out = append(out, t)
		}
	}
	return out
}

func (c console) ToolCatalog() []admin.ToolView {
	reg := c.sys.GetToolRegistry()
	inConfig := commands.ConfigTools()
	on, off := commands.ToolOverrides()
	out := []admin.ToolView{}
	for _, spec := range c.toolSpecs() {
		v := admin.ToolView{Spec: spec, Kind: specKind(spec), Names: []string{},
			InConfig: slices.ContainsFunc(inConfig, func(s string) bool { return sameSpec(s, spec) }),
			Switched: slices.Contains(on, spec) || slices.Contains(off, spec)}
		for _, t := range specTools(reg, spec) {
			v.Loaded = true
			v.Names = append(v.Names, t.GetName())
			v.AdminOnly = v.AdminOnly || irc.IsAdminOnly(t)
			if v.Description == "" || t.GetName() == spec {
				if s := t.GetSchema(); s != nil {
					v.Description = s.Description()
				}
			}
		}
		if v.Description == "" {
			v.Description = irc.NativeToolDescription(spec)
		}
		slices.Sort(v.Names)
		out = append(out, v)
	}
	return out
}

func specKind(spec string) string {
	switch {
	case spec == "task__start":
		return "work"
	case strings.HasSuffix(strings.ToLower(spec), ".json"):
		return "mcp"
	case strings.Contains(spec, "/") || strings.Contains(spec, `\`):
		return "plugin"
	}
	return "native"
}

var errUnknownTool = errors.New("not a tool this console offers")

// SwitchTool loads or unloads one tool list entry now and records it, so it stays that way after
// a restart. A plugin's required settings are checked before it is loaded.
func (c console) SwitchTool(spec string, on bool, by string) error {
	if !slices.Contains(c.toolSpecs(), spec) {
		return errUnknownTool
	}
	toolSwitchMu.Lock()
	defer toolSwitchMu.Unlock()
	reg := c.sys.GetToolRegistry()
	loaded := specTools(reg, spec)
	switch {
	case on && len(loaded) > 0, !on && len(loaded) == 0:
		// already so; still record it, so a restart agrees
	case on:
		if err := c.loadSpec(reg, spec); err != nil {
			return err
		}
	default:
		for _, t := range loaded {
			reg.Remove(t.GetName())
		}
	}
	commands.PersistToolSwitch(spec, on)
	core.GetLogger().Info("tool_switched", "tool", spec, "on", on, "by", by)
	return nil
}

func (c console) loadSpec(reg *tools.ToolRegistry, spec string) error {
	if specKind(spec) == "plugin" {
		meta, err := core.ReadShellToolMeta(spec)
		if err != nil {
			return fmt.Errorf("couldn't read the plugin: %w", err)
		}
		if missing := core.MissingEnv(meta.Requires, nil); len(missing) > 0 {
			return fmt.Errorf("needs %s set under env: in config.yml", strings.Join(missing, ", "))
		}
	}
	adminTools := map[string]bool{}
	for _, name := range c.cfg.Bot.AdminTools {
		adminTools[name] = true
	}
	for _, s := range withTaskToolset([]string{spec}) {
		if err := loadToolSpec(reg, s, adminTools); err != nil {
			for _, t := range specTools(reg, spec) {
				reg.Remove(t.GetName())
			}
			return fmt.Errorf("couldn't load %s: %w", s, err)
		}
	}
	return nil
}

// ResetToolSwitches puts the loaded tools back to config.yml's list and forgets the switches.
func (c console) ResetToolSwitches(by string) error {
	on, off := commands.ToolOverrides()
	var failed []string
	for _, spec := range on {
		if err := c.SwitchTool(spec, false, by); err != nil {
			failed = append(failed, spec)
		}
	}
	for _, spec := range off {
		if err := c.SwitchTool(spec, true, by); err != nil {
			failed = append(failed, spec)
		}
	}
	commands.ClearToolSwitches()
	if len(failed) > 0 {
		return fmt.Errorf("these didn't switch back and need a restart: %s", strings.Join(failed, ", "))
	}
	return nil
}

func (c console) ExportConfig() (admin.ExportView, error) {
	path := config.Path()
	if path == "" {
		return admin.ExportView{}, errors.New("not started with a config file")
	}
	out, changes, err := commands.ExportConfig(path, time.Now())
	if errors.Is(err, commands.ErrNothingToExport) {
		return admin.ExportView{}, admin.ErrNothingToExport
	}
	v := admin.ExportView{Path: out, Changes: []admin.ExportChange{}}
	for _, ch := range changes {
		v.Changes = append(v.Changes, admin.ExportChange{Key: ch.Key, From: ch.From, To: ch.To})
	}
	return v, err
}

func (c console) ResetAll(by string) (admin.ResetAllView, error) {
	reset, later := commands.ResetAll(c.cfg, c.sys, by, core.GetLogger())
	if err := c.ResetToolSwitches(by); err != nil {
		return admin.ResetAllView{}, err
	}
	return admin.ResetAllView{Reset: append([]string{}, reset...), OnRestart: append([]string{}, later...)}, nil
}

func (c console) ListOverrides() []string {
	return append([]string{}, commands.ListOverrides(config.Path())...)
}
