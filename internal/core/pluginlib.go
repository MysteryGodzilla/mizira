// Copyright (C) 2026 BareMetal
// Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"os"
	"path/filepath"
	"strings"
)

// ExportPluginLib prepends dir to PYTHONPATH so every tool the bot runs,
// shipped or custom, can import the shared metald_tools package. Returns
// the absolute path exported, or "" if dir does not exist.
func ExportPluginLib(dir string) string {
	if dir == "" {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return ""
	}
	parts := []string{abs}
	for _, p := range filepath.SplitList(os.Getenv("PYTHONPATH")) {
		if p != "" && p != abs {
			parts = append(parts, p)
		}
	}
	os.Setenv("PYTHONPATH", strings.Join(parts, string(os.PathListSeparator)))
	return abs
}

// ExportModelEnv tells tools which model the bot is using (METALD_MODEL_URL, METALD_MODEL,
// METALD_MODEL_KEY), so their own model calls (safety reviews, vision) use it unless their settings
// name another. Asking a single-model server for a different one makes it unload the bot's model.
func ExportModelEnv(url, model, key string) {
	if _, name, found := strings.Cut(model, "/"); found {
		model = name
	}
	os.Setenv("METALD_MODEL_URL", strings.TrimSuffix(url, "/"))
	os.Setenv("METALD_MODEL", model)
	os.Setenv("METALD_MODEL_KEY", key)
}
