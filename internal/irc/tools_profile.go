// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alexschlessinger/pollytool/schema"
	"github.com/alexschlessinger/pollytool/tools"

	"B4reMetal/metald/internal/core"
)

// newProfileTimezoneTool lets someone tell the bot their own time zone, so times they give (a
// reminder "at 4pm") are read on their clock. Only operators may set someone else's.
func newProfileTimezoneTool() tools.Tool {
	return &tools.Func{
		Name: "profile__timezone",
		Desc: "Save the time zone someone lives in, when they tell you where they are or what their time zone " +
			"is. Use the IANA name for their city, e.g. Australia/Brisbane, Australia/Melbourne, America/New_York, " +
			"Europe/London. People set their own; only an operator can set someone else's.",
		Params: schema.Params{
			"timezone": schema.S("IANA time zone name, e.g. Australia/Brisbane"),
			"nick":     schema.S("Whose time zone it is; leave out for the person speaking"),
		},
		Required: []string{"timezone"},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, err := validateContext(ctx)
			if err != nil {
				return "", err
			}
			nick := strings.TrimSpace(args.String("nick"))
			if nick == "" {
				nick = chatCtx.Speaker()
			}
			if !strings.EqualFold(nick, chatCtx.Speaker()) && !chatCtx.IsAdmin() {
				return fmt.Sprintf("Refused: only %s or an operator can set %s's time zone.", nick, nick), nil
			}
			tz := strings.TrimSpace(args.String("timezone"))
			if !core.ValidTimezone(tz) {
				return fmt.Sprintf("Not saved: %q isn't a time zone name. Use the IANA name for their city, "+
					"like Australia/Brisbane.", tz), nil
			}
			store, err := core.Memories()
			if err != nil {
				return "Error: memory is unavailable right now", nil
			}
			if err := store.SetTimezone(chatCtx.GetNetwork(), nick, tz, chatCtx.SpeakerKey()); err != nil {
				return "Not saved: " + err.Error(), nil
			}
			loc, _ := time.LoadLocation(tz)
			chatCtx.GetLogger().Info("profile_timezone_set", "nick", nick, "timezone", tz, "by", chatCtx.SpeakerKey())
			return fmt.Sprintf("Saved: %s is on %s (their time now: %s). Say so briefly.",
				nick, tz, time.Now().In(loc).Format("Mon 15:04 MST")), nil
		},
	}
}
