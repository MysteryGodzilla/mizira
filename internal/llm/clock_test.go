// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"strings"
	"testing"
	"time"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

// A saved time zone never appears beside a message: she brought up where people live.
func TestTimeBlockIsUTCOnly(t *testing.T) {
	ctx := mocktest.NewMockContext().WithSource("bob")
	net := ctx.GetNetwork()
	mem, _ := core.Memories()
	mem.SetTimezone(net, "bob", "Australia/Melbourne", "bob")
	t.Cleanup(func() { mem.DeleteProfile(net, "bob") })
	now := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)

	got := timeBlock(ctx, now)
	if !strings.HasSuffix(got, "\n  Thu 15 Oct 2026 03:00 UTC") || strings.Contains(got, "Melbourne") || strings.Contains(got, "bob") {
		t.Errorf("time block = %q", got)
	}
}
