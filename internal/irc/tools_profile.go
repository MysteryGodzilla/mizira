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

// newProfileLocalTimeTool gives someone's local time for a question that needs it. Where people live
// stays out of the conversation: the answer has the time and its UTC offset, never the zone's name.
func newProfileLocalTimeTool() tools.Tool {
	return &tools.Func{
		Name: "profile__localtime",
		Desc: "Get someone's local time, only when a question needs it: what time or day it is for them, whether " +
			"it's late for them, or turning a time they gave into someone else's. Never bring up where someone " +
			"lives or their time zone in conversation.",
		Params: schema.Params{
			"nick": schema.S("Whose local time; leave out for the person speaking"),
		},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, err := validateContext(ctx)
			if err != nil {
				return "", err
			}
			nick := strings.TrimSpace(args.String("nick"))
			if nick == "" {
				nick = chatCtx.Speaker()
			}
			store, err := core.Memories()
			if err != nil {
				return "Error: memory is unavailable right now", nil
			}
			people, err := store.People(chatCtx.GetNetwork())
			if err != nil {
				return "Error: memory is unavailable right now", nil
			}
			return localTimeReply(nick, people, time.Now()), nil
		},
	}
}

func localTimeReply(nick string, people core.People, now time.Time) string {
	loc, ok := people.Location(nick)
	if !ok {
		return fmt.Sprintf("%s has no time zone saved. If the answer needs it, ask them what city or time zone "+
			"they're in and save it with profile__timezone.", nick)
	}
	local := now.In(loc)
	return fmt.Sprintf("%s's local time: %s (%s). Use it to answer; don't mention where they live or their time zone.",
		nick, local.Format("Mon 2 Jan 15:04"), utcOffset(local))
}

// utcOffset is "UTC+11" or "UTC+5:30".
func utcOffset(t time.Time) string {
	_, secs := t.Zone()
	sign := "+"
	if secs < 0 {
		sign, secs = "-", -secs
	}
	if m := secs % 3600 / 60; m != 0 {
		return fmt.Sprintf("UTC%s%d:%02d", sign, secs/3600, m)
	}
	return fmt.Sprintf("UTC%s%d", sign, secs/3600)
}

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
