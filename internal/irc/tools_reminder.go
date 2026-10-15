// Copyright (C) 2026 BareMetal
// Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/alexschlessinger/pollytool/schema"
	"github.com/alexschlessinger/pollytool/tools"

	"B4reMetal/metald/internal/core"
)

// newIrcRemindTool lets the bot schedule a message to be said later in the channel.
func newIrcRemindTool() tools.Tool {
	return &tools.Func{
		Name: "irc__remind",
		Desc: fmt.Sprintf(
			"Schedule a message to be posted in the channel later, pinging "+
				"someone. Use when asked to remind a person about something "+
				"at a later time. The delay must be between 1 minute and %d "+
				"minutes (%d days) - this is a short-term nudge, not a "+
				"calendar, so refuse anything further out instead of "+
				"silently shortening it. THIS TOOL IS THE ONLY WAY TO "+
				"ACTUALLY SET A REMINDER: saying 'i'll remind you' without "+
				"calling it means nothing will ever be sent. Call it first, "+
				"then confirm.",
			int(core.MaxReminderDelay.Minutes()),
			int(core.MaxReminderDelay.Hours()/24),
		),
		Params: schema.Params{
			"nick":    schema.S("The nickname to remind (who gets pinged when it fires)"),
			"minutes": schema.Int("For \"in 20 minutes\" or \"in 2 hours\": how many minutes from now. Leave out when a clock time is given"),
			"at": schema.S("For a clock time like \"4pm\" or \"16:30\": the time exactly as the person said it, " +
				"in THEIR time zone; the tool converts it. Leave out when minutes is given"),
			"day":  schema.S("With at: \"today\", \"tomorrow\", a weekday, or YYYY-MM-DD. Leave out for the next time that clock time comes"),
			"text": schema.S("What to say to them when it fires"),
		},
		Required: []string{"nick", "text"},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, err := validateContext(ctx)
			if err != nil {
				return "", err
			}

			nick := strings.TrimSpace(args.String("nick"))
			if nick == "" {
				return "", fmt.Errorf("nick must be a non-empty string")
			}
			text := strings.TrimSpace(args.String("text"))
			if text == "" {
				return "", fmt.Errorf("text must be a non-empty string")
			}

			if wakesBot(chatCtx, nick, text) {
				return "Refused: a reminder can't address you; you'd answer your own message. " +
					"Remind a person instead.", nil
			}

			minutes := args.Int("minutes", 0)
			d := time.Duration(minutes) * time.Minute
			local := ""
			if at := strings.TrimSpace(args.String("at")); at != "" {
				loc, tz, ok := askerZone(chatCtx)
				if !ok {
					return fmt.Sprintf("Not set: %s's time zone isn't known, so %q can't be worked out. Ask them "+
						"where they are (a city is enough), save it with profile__timezone, then set the reminder.",
						chatCtx.Speaker(), at), nil
				}
				due, err := clockTime(time.Now(), loc, at, strings.TrimSpace(args.String("day")))
				if err != nil {
					return "Not set: " + err.Error() + ". Ask them to say the time another way.", nil
				}
				d = time.Until(due)
				minutes = int(d.Round(time.Minute).Minutes())
				local = fmt.Sprintf(" (%s %s)", due.Format("Mon 2 Jan 15:04"), tz)
			}

			// Out-of-range is refused rather than clamped.
			if d < core.MinReminderDelay {
				return fmt.Sprintf(
					"Refused: %d minutes is too soon - the minimum is %d minute(s). Just say it now instead.",
					minutes, int(core.MinReminderDelay.Minutes())), nil
			}
			if d > core.MaxReminderDelay {
				return fmt.Sprintf(
					"Refused: %d minutes is too far out - the maximum is %d minutes (%d days). "+
						"Tell the user you don't do reminders that far ahead.",
					minutes,
					int(core.MaxReminderDelay.Minutes()),
					int(core.MaxReminderDelay.Hours()/24)), nil
			}

			channel := chatCtx.GetConfig().Server.Channel
			r, err := core.Reminders().Add(chatCtx.GetNetwork(), nick, channel, text, chatCtx.GetSource(), d)
			if err != nil {
				return fmt.Sprintf("Refused: %s", err.Error()), nil
			}

			chatCtx.GetLogger().Info("reminder_set",
				"id", r.ID, "nick", r.Nick, "minutes", minutes,
				"due", r.Due.UTC(), "set_by", r.SetBy, "text", r.Text)

			return fmt.Sprintf(
				"Reminder %s set for %s in %d minute(s), at %s UTC%s. It will be posted in the channel. "+
					"Confirm it briefly in your own voice.",
				r.ID, r.Nick, minutes, r.Due.UTC().Format("15:04"), local), nil
		},
	}
}

// wakesBot reports whether the posted reminder would trigger the bot, which lets a reminder re-arm itself in a loop.
// The bot's own nick alone is fine: with a trigger set, the bot may share a nick with its operator.
func wakesBot(ctx ChatContextInterface, nick, text string) bool {
	trigger := ctx.GetConfig().Bot.Trigger
	if trigger == "" {
		trigger = ctx.GetBotNick()
	}
	// The text is checked on its own too: "eve: botty ..." opens with another name, which Mizira's
	// addressing reads as talk to eve, but the text alone would still wake the bot.
	return CheckAddressed(nick+": "+text, trigger) || CheckAddressed(text, trigger)
}

// newIrcRemindersTool lets the bot answer "what reminders are pending" and
// cancel one, without an operator having to read the json file.
func newIrcRemindersTool() tools.Tool {
	return &tools.Func{
		Name: "irc__reminders",
		Desc: "List the reminders currently scheduled, or cancel one by its id. " +
			"Use when someone asks what's pending or wants a reminder called off.",
		Params: schema.Params{
			"cancel_id": schema.S("Optional: the id of a reminder to cancel. Omit to just list them."),
		},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, err := validateContext(ctx)
			if err != nil {
				return "", err
			}

			if id := strings.TrimSpace(args.String("cancel_id")); id != "" {
				if core.Reminders().Cancel(chatCtx.GetNetwork(), id) {
					chatCtx.GetLogger().Info("reminder_cancelled", "id", id, "by", chatCtx.GetSource())
					return fmt.Sprintf("Cancelled reminder %s", id), nil
				}
				return fmt.Sprintf("No reminder with id %s", id), nil
			}

			pending := core.Reminders().List(chatCtx.GetNetwork())
			if len(pending) == 0 {
				return "No reminders scheduled", nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "%d reminder(s) pending:", len(pending))
			for _, r := range pending {
				fmt.Fprintf(&b, "\n  %s - %s in %s: %s",
					r.ID, r.Nick, time.Until(r.Due).Round(time.Minute), r.Text)
			}
			return b.String(), nil
		},
	}
}

// askerZone is the time zone of whoever asked, from their profile.
func askerZone(chatCtx ChatContextInterface) (*time.Location, string, bool) {
	store, err := core.Memories()
	if err != nil {
		return nil, "", false
	}
	people, err := store.People(chatCtx.GetNetwork())
	if err != nil {
		return nil, "", false
	}
	loc, ok := people.Location(chatCtx.Speaker())
	if !ok {
		return nil, "", false
	}
	return loc, loc.String(), true
}

var clockRe = regexp.MustCompile(`(?i)^(\d{1,2})(?:[:.](\d{2}))?\s*(am|pm)?$`)

// clockTime is when a clock time ("4pm", "16:30", "9.15 am") next comes in loc, on day if given
// ("today", "tomorrow", a weekday, or YYYY-MM-DD). Without a day, a time already past today is
// tomorrow.
func clockTime(now time.Time, loc *time.Location, at, day string) (time.Time, error) {
	m := clockRe.FindStringSubmatch(strings.TrimSpace(at))
	if m == nil {
		return time.Time{}, fmt.Errorf("%q isn't a clock time", at)
	}
	hour, _ := strconv.Atoi(m[1])
	minute := 0
	if m[2] != "" {
		minute, _ = strconv.Atoi(m[2])
	}
	switch strings.ToLower(m[3]) {
	case "am":
		if hour == 12 {
			hour = 0
		}
	case "pm":
		if hour < 12 {
			hour += 12
		}
	}
	if said, _ := strconv.Atoi(m[1]); hour > 23 || minute > 59 || (m[3] != "" && (said < 1 || said > 12)) {
		return time.Time{}, fmt.Errorf("%q isn't a clock time", at)
	}
	here := now.In(loc)
	date := time.Date(here.Year(), here.Month(), here.Day(), hour, minute, 0, 0, loc)
	switch d := strings.ToLower(strings.TrimSpace(day)); {
	case d == "" || d == "today":
		if d == "" && !date.After(here) {
			date = date.AddDate(0, 0, 1)
		}
	case d == "tomorrow":
		date = date.AddDate(0, 0, 1)
	default:
		if t, err := time.ParseInLocation("2006-01-02", d, loc); err == nil {
			date = time.Date(t.Year(), t.Month(), t.Day(), hour, minute, 0, 0, loc)
			break
		}
		wd, ok := weekdays[strings.TrimSuffix(d, "s")]
		if !ok {
			return time.Time{}, fmt.Errorf("%q isn't a day", day)
		}
		ahead := (int(wd) - int(here.Weekday()) + 7) % 7
		date = date.AddDate(0, 0, ahead)
		if !date.After(here) {
			date = date.AddDate(0, 0, 7)
		}
	}
	if !date.After(here) {
		return time.Time{}, fmt.Errorf("%s %s has already passed", day, at)
	}
	return date, nil
}

var weekdays = map[string]time.Weekday{
	"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday,
	"thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday,
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}
