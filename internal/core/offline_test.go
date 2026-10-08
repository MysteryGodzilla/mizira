// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"slices"
	"testing"
)

func TestOfflineReasons(t *testing.T) {
	t.Cleanup(func() { offline.reasons = nil })
	if got := OfflineReasons(); len(got) != 0 {
		t.Fatalf("offline before any reason: %q", got)
	}
	KeepOffline("started console-only")
	KeepOffline("websearch.py: missing settings: EXA_API_KEY", "webfetch.py: missing settings: EXA_API_KEY")
	got := OfflineReasons()
	want := []string{"started console-only", "websearch.py: missing settings: EXA_API_KEY",
		"webfetch.py: missing settings: EXA_API_KEY"}
	if !slices.Equal(got, want) {
		t.Fatalf("reasons = %q, want %q", got, want)
	}
	got[0] = "changed"
	if OfflineReasons()[0] != want[0] {
		t.Error("caller could change the stored reasons")
	}
}
