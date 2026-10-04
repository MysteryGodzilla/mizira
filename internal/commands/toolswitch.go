// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"slices"
	"sync"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
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
	configTools = slices.Clone(cfg.Bot.Tools)
	configToolsMu.Unlock()
	if len(o.ToolsOn) == 0 && len(o.ToolsOff) == 0 {
		return
	}
	tools := slices.DeleteFunc(slices.Clone(cfg.Bot.Tools), func(t string) bool { return slices.Contains(o.ToolsOff, t) })
	for _, t := range o.ToolsOn {
		if !slices.Contains(tools, t) {
			tools = append(tools, t)
		}
	}
	cfg.Bot.Tools = tools
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

// ClearToolSwitches forgets every tool switch, so the next start uses config.yml's list.
func ClearToolSwitches() {
	overridesMu.Lock()
	defer overridesMu.Unlock()
	o := loadOverrides()
	o.ToolsOn, o.ToolsOff = nil, nil
	saveOverrides(o)
}
