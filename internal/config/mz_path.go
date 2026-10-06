// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package config

// Path is the config file this process was started with, or "" for none.
func Path() string { return getConfigPath() }
