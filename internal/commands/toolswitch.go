// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/alexschlessinger/pollytool/tools"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// config.yml's own tool list, before the console's switches are layered on.
var (
	configToolsMu sync.RWMutex
	configTools   []string
)

// ConfigTools is the tool list as config.yml has it.
func ConfigTools() []string {
	configToolsMu.RLock()
	defer configToolsMu.RUnlock()
	return slices.Clone(configTools)
}

// applyToolOverrides layers the console's tool switches over config.yml's list, so a tool switched
// on stays on after a restart (and one switched off stays off).
func applyToolOverrides(cfg *config.Configuration, o runtimeOverrides) {
	if cfg.Bot == nil {
		return
	}
	configToolsMu.Lock()
	configTools = slices.Clone(config.List(&cfg.Bot.Tools))
	configToolsMu.Unlock()
	o.ToolsOn, o.ToolsOff = dropAgreedSwitches(config.List(&cfg.Bot.Tools))
	if len(o.ToolsOn) == 0 && len(o.ToolsOff) == 0 {
		return
	}
	tools := slices.DeleteFunc(slices.Clone(config.List(&cfg.Bot.Tools)), func(t string) bool { return slices.Contains(o.ToolsOff, t) })
	for _, t := range o.ToolsOn {
		if !slices.Contains(tools, t) {
			tools = append(tools, t)
		}
	}
	config.SetList(&cfg.Bot.Tools, tools)
	core.GetLogger().Info("override_applied", "key", "tools", "on", o.ToolsOn, "off", o.ToolsOff)
}

// ToolOverrides are the tool list entries switched on and off relative to config.yml.
func ToolOverrides() (on, off []string) {
	overridesMu.Lock()
	defer overridesMu.Unlock()
	o := loadOverrides()
	return o.ToolsOn, o.ToolsOff
}

// PersistToolSwitch records spec switched on or off. Switching back to what config.yml says
// drops the entry rather than recording it.
func PersistToolSwitch(spec string, on bool) {
	inConfig := slices.Contains(ConfigTools(), spec)
	overridesMu.Lock()
	defer overridesMu.Unlock()
	o := loadOverrides()
	o.ToolsOn = slices.DeleteFunc(o.ToolsOn, func(t string) bool { return t == spec })
	o.ToolsOff = slices.DeleteFunc(o.ToolsOff, func(t string) bool { return t == spec })
	switch {
	case on && !inConfig:
		o.ToolsOn = append(o.ToolsOn, spec)
	case !on && inConfig:
		o.ToolsOff = append(o.ToolsOff, spec)
	}
	saveOverrides(o)
}

// dropAgreedSwitches forgets switches config.yml now agrees with (a switched-on tool since added to
// its list, say by an export), so they stop showing as differences; it returns the ones left.
func dropAgreedSwitches(inConfig []string) (on, off []string) {
	overridesMu.Lock()
	defer overridesMu.Unlock()
	o := loadOverrides()
	on = slices.DeleteFunc(slices.Clone(o.ToolsOn), func(t string) bool { return slices.Contains(inConfig, t) })
	off = slices.DeleteFunc(slices.Clone(o.ToolsOff), func(t string) bool { return !slices.Contains(inConfig, t) })
	if len(on) != len(o.ToolsOn) || len(off) != len(o.ToolsOff) {
		o.ToolsOn, o.ToolsOff = on, off
		saveOverrides(o)
		core.GetLogger().Info("tool_switches_dropped", "reason", "config.yml already agrees")
	}
	return on, off
}

// ClearToolSwitches forgets every tool switch, so the next start uses config.yml's list.
func ClearToolSwitches() {
	overridesMu.Lock()
	defer overridesMu.Unlock()
	o := loadOverrides()
	o.ToolsOn, o.ToolsOff = nil, nil
	saveOverrides(o)
}

// ErrNoToolMatched: the pattern matched no loaded tool.
var ErrNoToolMatched = errors.New("no tools matched")

// RestrictTools makes the loaded tools pattern matches admin-only (or open to everyone again), and
// keeps that across restarts. It's "+tools restrict" and "+tools unrestrict", shared with the
// console. It returns the tools whose state changed.
func RestrictTools(cfg *config.Configuration, registry *tools.ToolRegistry, pattern string, restrict bool, by string, log *slog.Logger) ([]string, error) {
	matches := matchToolNames(registry.All(), pattern)
	if len(matches) == 0 {
		return nil, ErrNoToolMatched
	}
	var changed []string
	configMu.Lock()
	defer configMu.Unlock()
	for _, name := range matches {
		tool, ok := registry.Get(name)
		if !ok || irc.IsAdminOnly(tool) == restrict {
			continue
		}
		if restrict {
			registry.Register(irc.NewAdminOnlyTool(tool))
		} else {
			registry.Register(irc.UnwrapAdminOnly(tool))
		}
		changed = append(changed, name)
	}
	if len(changed) == 0 {
		return nil, nil
	}
	syncAdminToolsConfig(cfg, changed, restrict)
	PersistAdminTools(config.List(&cfg.Bot.AdminTools))
	log.Info("tool_restriction_changed", "tools", strings.Join(changed, ","), "admin_only", restrict, "by", by)
	return changed, nil
}
