// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package config

import (
	"slices"
	"sync"
)

// listMu guards the list settings (admins, screening, bots, tools), which ~ commands and the console
// replace while chat requests read them.
var listMu sync.RWMutex

// List returns a list setting as it stands. Lists are replaced, never changed in place, so the
// slice stays valid; don't change it.
func List(p *[]string) []string {
	listMu.RLock()
	defer listMu.RUnlock()
	return *p
}

// SetList replaces a list setting with a copy of v.
func SetList(p *[]string, v []string) {
	v = slices.Clone(v)
	listMu.Lock()
	defer listMu.Unlock()
	*p = v
}
