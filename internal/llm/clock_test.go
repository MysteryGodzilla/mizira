// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"testing"
	"time"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

func TestTimeBlock(t *testing.T) {
	ctx := mocktest.NewMockContext().WithSource("bob")
	net := ctx.GetNetwork()
	mem, _ := core.Memories()
	mem.SetTimezone(net, "bob", "Australia/Melbourne", "bob")
	mem.SetTimezone(net, "carol", "America/New_York", "carol")
	t.Cleanup(func() {
		mem.DeleteProfile(net, "bob")
		mem.DeleteProfile(net, "carol")
	})
	now := time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)

	want := "the time now:\n  Thu 15 Oct 2026 03:00 UTC; bob: Thu 15 Oct 14:00 AEDT (Australia/Melbourne); " +
		"carol: Wed 14 Oct 23:00 EDT (America/New_York)"
	if got := timeBlock(ctx, "is carol around?", now); got != want {
		t.Errorf("time block =\n%s\nwant\n%s", got, want)
	}
	other := mocktest.NewMockContext().WithSource("dave")
	if got := timeBlock(other, "hi", now); got != "the time now:\n  Thu 15 Oct 2026 03:00 UTC; dave: time zone not known" {
		t.Errorf("unknown zone = %q", got)
	}
}
