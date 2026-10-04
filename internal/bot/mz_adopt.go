// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package bot

import (
	"log/slog"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
)

// adoptUnscoped moves data saved while the first network had no name under its name, before
// anything reads a conversation, so naming a network keeps its history (the memory and reminder
// moves below in Run are upstream's).
func adoptUnscoped(nets []*config.ServerConfig) {
	if len(nets) == 0 || nets[0].Name == "" {
		return
	}
	name := nets[0].Name
	if db, err := core.Context(); err == nil {
		if n, err := db.AdoptUnscoped(name); err != nil {
			slog.Error("context_migration_failed", "error", err.Error())
		} else if n > 0 {
			slog.Info("context_assigned_to_network", "network", name, "rows", n)
		}
	}
	if n := core.Ignores().AdoptUnscoped(name); n > 0 {
		slog.Info("ignores_assigned_to_network", "network", name, "count", n)
	}
	if store, err := core.Memories(); err == nil {
		if n, err := store.AdoptUnscopedSelfNotes(name); err != nil {
			slog.Error("selfnote_migration_failed", "error", err.Error())
		} else if n > 0 {
			slog.Info("selfnotes_assigned_to_network", "network", name, "count", n)
		}
	}
}
