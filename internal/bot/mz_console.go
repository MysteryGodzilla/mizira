// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package bot

import (
	"strings"

	"B4reMetal/metald/internal/commands"
	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
)

// console is the bot side of the operator console's own pages (admin.Mizira): every action runs the
// code the matching ~ command runs.
type console struct {
	cfg *config.Configuration
	sys core.System
}

func (c console) RunState() string { return core.State().String() }

func (c console) SetRunState(state, by string) (string, bool, int) {
	to, ok := core.ParseRunState(state)
	if !ok {
		return core.State().String(), false, 0
	}
	change := commands.ChangeRunState(to, by, "", core.GetLogger())
	return change.Previous.String(), change.Changed, change.Cancelled
}

func (c console) Tools() []string {
	var names []string
	if reg := c.sys.GetToolRegistry(); reg != nil {
		for _, t := range reg.All() {
			names = append(names, t.GetName())
		}
	}
	return names
}

// Thinking is off for "off" (nothing sent) and "none" (reasoning turned off at the model).
func (c console) Thinking() bool {
	effort := strings.ToLower(c.cfg.Model.ThinkingEffort)
	return effort != "" && effort != "off" && effort != "none"
}
