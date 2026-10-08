// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"slices"
	"sync"
)

var offline struct {
	sync.Mutex
	reasons []string
}

// KeepOffline records why the bot stays off IRC this run (a console-only start, a tool that can't
// load); the console still starts and shows them.
func KeepOffline(reasons ...string) {
	offline.Lock()
	defer offline.Unlock()
	offline.reasons = append(offline.reasons, reasons...)
}

// OfflineReasons is empty when the bot joins IRC as normal.
func OfflineReasons() []string {
	offline.Lock()
	defer offline.Unlock()
	return slices.Clone(offline.reasons)
}
