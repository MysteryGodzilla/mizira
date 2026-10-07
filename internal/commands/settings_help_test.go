// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"os"
	"regexp"
	"sort"
	"testing"
)

// Every setting the console shows has a help line in the page's own words
// (web/admin/src/mz/settings.ts), so a new setting can't appear there unexplained.
func TestEverySettingHasConsoleHelp(t *testing.T) {
	src, err := os.ReadFile("../../web/admin/src/mz/settings.ts")
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for key := range configFields {
		if secretSetting(key) {
			continue
		}
		if !regexp.MustCompile(`(?m)^\s*"?` + regexp.QuoteMeta(key) + `"?:\s*(\{[^}]*help:|sampling\("[^"]+)`).Match(src) {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("no help line in web/admin/src/mz/settings.ts for: %v", missing)
	}
}
