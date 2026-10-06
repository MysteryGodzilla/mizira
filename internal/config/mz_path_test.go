// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package config

import "testing"

func TestPathFromEnvironment(t *testing.T) {
	t.Setenv("METALD_CONFIG", "/tmp/example/config.yml")
	if got := Path(); got != "/tmp/example/config.yml" {
		t.Errorf("Path() = %q", got)
	}
}
