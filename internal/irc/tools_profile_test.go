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

// The local time comes with its offset, never the zone's name, so it can't say where someone lives.
func TestProfileLocalTimeTool(t *testing.T) {
	mock := mocktest.NewMockContext().WithSource("carol")
	mock.GetConfig().Server.Name = "localtime-tool"
	store, _ := core.Memories()
	store.SetTimezone("localtime-tool", "carol", "Australia/Melbourne", "carol")
	store.SetTimezone("localtime-tool", "dave", "Asia/Kolkata", "dave")
	t.Cleanup(func() {
		store.DeleteProfile("localtime-tool", "carol")
		store.DeleteProfile("localtime-tool", "dave")
	})
	ctx := InjectContext(context.Background(), mock)

	out := runTool(t, newProfileLocalTimeTool(), ctx, tools.Args{})
	if !strings.HasPrefix(out, "carol's local time: ") || !strings.Contains(out, "(UTC+1") || strings.Contains(out, "Melbourne") {
		t.Errorf("own time = %q", out)
	}
	if out := runTool(t, newProfileLocalTimeTool(), ctx, tools.Args{"nick": "dave"}); !strings.Contains(out, "(UTC+5:30)") || strings.Contains(out, "Kolkata") {
		t.Errorf("half-hour zone = %q", out)
	}
	if out := runTool(t, newProfileLocalTimeTool(), ctx, tools.Args{"nick": "erin"}); !strings.Contains(out, "no time zone saved") {
		t.Errorf("unknown = %q", out)
	}
}

func TestProfileLineLeavesOutTheTimeZone(t *testing.T) {
	store, _ := core.Memories()
	store.SetTimezone("tz-line", "carol", "Australia/Melbourne", "carol")
	t.Cleanup(func() { store.DeleteProfile("tz-line", "carol") })
	people, _ := store.People("tz-line")
	if line := ProfileLine(people, "carol"); strings.Contains(line, "Melbourne") || strings.Contains(line, "time zone") {
		t.Errorf("profile line = %q", line)
	}
}
