// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"context"
	"strings"
	"testing"

	"github.com/alexschlessinger/pollytool/tools"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

// People set their own time zone; only an operator sets someone else's; made-up zones are refused.
func TestProfileTimezoneTool(t *testing.T) {
	mock := mocktest.NewMockContext().WithSource("carol")
	mock.GetConfig().Server.Name = "tz-tool"
	store, _ := core.Memories()
	t.Cleanup(func() {
		store.DeleteProfile("tz-tool", "carol")
		store.DeleteProfile("tz-tool", "dave")
	})
	ctx := InjectContext(context.Background(), mock)

	if out := runTool(t, newProfileTimezoneTool(), ctx, tools.Args{"timezone": "Australia/Brisbane"}); !strings.HasPrefix(out, "Saved: carol is on Australia/Brisbane") {
		t.Errorf("own zone = %q", out)
	}
	if out := runTool(t, newProfileTimezoneTool(), ctx, tools.Args{"timezone": "Mars/Olympus"}); !strings.HasPrefix(out, "Not saved") {
		t.Errorf("made-up zone = %q", out)
	}
	if out := runTool(t, newProfileTimezoneTool(), ctx, tools.Args{"timezone": "Europe/London", "nick": "dave"}); !strings.HasPrefix(out, "Refused") {
		t.Errorf("someone else's = %q", out)
	}
	admin := InjectContext(context.Background(), mocktest.NewMockContext().WithSource("carol").WithAdmin(true))
	mockAdmin, _ := validateContext(admin)
	mockAdmin.GetConfig().Server.Name = "tz-tool"
	if out := runTool(t, newProfileTimezoneTool(), admin, tools.Args{"timezone": "Europe/London", "nick": "dave"}); !strings.HasPrefix(out, "Saved: dave") {
		t.Errorf("operator setting dave's = %q", out)
	}
	people, _ := store.People("tz-tool")
	if c, _ := people.Get("carol"); c.Timezone != "Australia/Brisbane" {
		t.Errorf("carol = %+v", c)
	}
	if d, _ := people.Get("dave"); d.Timezone != "Europe/London" {
		t.Errorf("dave = %+v", d)
	}
}
