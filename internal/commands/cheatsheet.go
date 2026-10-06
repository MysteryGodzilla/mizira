// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"regexp"
	"slices"
)

// The console's IRC command cheatsheet. Which commands exist and which are admin-only come from the
// registry, so they can't go stale; what each one does is written here, and a test fails when a
// registered command has no entry.

// CommandHelp describes one command for the cheatsheet. Usage lines and text write commands with
// "+", which Cheatsheet replaces with the configured prefix.
type CommandHelp struct {
	Group   string
	Usage   []string
	Text    string
	Feature string // the console feature it needs ("work", "radio"), if any
}

// CheatGroups is the order groups are shown in.
var CheatGroups = []string{"Basics", "Conversation", "Memory", "People and safety", "Running her", "Settings and tools", "Background work", "Radio"}

var commandHelp = map[string]CommandHelp{
	"+help":    {Group: "Basics", Usage: []string{"+help"}, Text: "List the commands you can use."},
	"+version": {Group: "Basics", Usage: []string{"+version"}, Text: "Show her version."},
	"+stats":   {Group: "Basics", Usage: []string{"+stats"}, Text: "This conversation's size, its limit, and token use."},

	"+reset": {Group: "Conversation", Usage: []string{"+reset"},
		Text: "Clear this channel's conversation, any +prompt persona and any model switch. The recap and memories stay."},
	"+recap": {Group: "Conversation", Usage: []string{"+recap", "+recap clear", "+recap fold"},
		Text: "Show the channel's recap (the summary of older chat), delete it, or fold the older conversation into it now (keeps the last 6 turns)."},
	"+prompt": {Group: "Conversation", Usage: []string{"+prompt", "+prompt <text>"},
		Text: "Run a temporary persona in this channel; every tool is off until +reset. No text shows what's running."},

	"+remember": {Group: "Memory", Usage: []string{"+remember <fact about you>", "+remember <nick>: <fact>"},
		Text: "Save a fact, through the same safety checks as her own memory tool. The reply names the saved id."},
	"+recall": {Group: "Memory", Usage: []string{"+recall", "+recall <nick>"},
		Text: "What she remembers about someone (you by default)."},
	"+memories": {Group: "Memory", Usage: []string{"+memories", "+memories about <nick>", "+memories room", "+memories forget <id>", "+memories clear <nick>", "+memories clear"},
		Text: "List memories, someone's memories or room memory (about her and the channel); forget one, or clear someone's. Your own only, unless admin; clearing everything is admin-only. Locked memories are kept."},
	"+forget": {Group: "Memory", Usage: []string{"+forget <id>"},
		Text: "Forget one memory by its id, as +memories forget does."},
	"+selfnotes": {Group: "Memory", Usage: []string{"+selfnotes", "+selfnotes approve <id> [new wording]", "+selfnotes deny <id>"},
		Text: "Notes she proposed from folded chat, about herself and the people who spoke: list them, approve (optionally reworded) to save as a memory, or deny."},

	"+ignore": {Group: "People and safety", Usage: []string{"+ignore <nick> [duration] [reason]", "+ignore list"},
		Text: "Stop answering someone for a while (1h by default), or list who is ignored, for how long, and why."},
	"+unignore": {Group: "People and safety", Usage: []string{"+unignore <nick>"}, Text: "Answer someone again."},
	"+screen": {Group: "People and safety", Usage: []string{"+screen <nick>", "+screen list"},
		Text: "Check everything someone says, and her replies to them, and keep them out of the chat log and recap; list who is screened."},
	"+unscreen": {Group: "People and safety", Usage: []string{"+unscreen <nick>"}, Text: "Stop screening someone."},
	"+suspicion": {Group: "People and safety", Usage: []string{"+suspicion", "+suspicion <nick>"},
		Text: "Suspicion scores (they fade over time), highest first, or one person's."},
	"+bots": {Group: "People and safety", Usage: []string{"+bots list", "+bots add nick|prefix <value>", "+bots remove nick|prefix <value>"},
		Text: "Who counts as another bot: by nick, or by the tag on their lines such as [metalai]."},
	"+admins": {Group: "People and safety", Usage: []string{"+admins", "+admins add <hostmask>", "+admins remove <hostmask>"},
		Text: "List, add or remove admins (nick!ident@host, wildcards allowed)."},

	"+pause":  {Group: "Running her", Usage: []string{"+pause"}, Text: "Start nothing new; replies in progress finish. Kept across restarts until +resume."},
	"+stop":   {Group: "Running her", Usage: []string{"+stop"}, Text: "Cancel everything now and start nothing new. Kept across restarts until +resume."},
	"+resume": {Group: "Running her", Usage: []string{"+resume"}, Text: "Carry on after +pause or +stop."},

	"+set": {Group: "Settings and tools", Usage: []string{"+set <key> <value>"},
		Text: "Change a setting; kept across restarts. Changing model or prompt clears this channel's history."},
	"+get":     {Group: "Settings and tools", Usage: []string{"+get <key>"}, Text: "Show a setting."},
	"+models":  {Group: "Settings and tools", Usage: []string{"+models", "+models <name>"}, Text: "List the models the model server offers, or switch to one."},
	"+backend": {Group: "Settings and tools", Usage: []string{"+backend"}, Text: "Check that the model server answers."},
	"+tools": {Group: "Settings and tools", Usage: []string{"+tools", "+tools load <spec>", "+tools rm <pattern>", "+tools restrict <pattern>", "+tools unrestrict <pattern>"},
		Text: "List her tools; load or unload one; make tools admin-only, or lift that. Listing is open to all, the rest is admin-only."},

	"+task": {Group: "Background work", Usage: []string{"+task <what to do>", "+task list", "+task <id>", "+task result <id>", "+task cancel <id>", "+task pause", "+task resume"},
		Text: "Start a background task, see one or its result, cancel it (its owner or an admin), or pause/resume starting work (admin).", Feature: "work"},
	"+tasks": {Group: "Background work", Usage: []string{"+tasks"}, Text: "List background work.", Feature: "work"},
	"+goal": {Group: "Background work", Usage: []string{"+goal <objective> [done when: <criteria>]", "+goal status <id>", "+goal nudge <id> <hint>", "+goal accept <id>", "+goal cancel <id>"},
		Text: "Work on something in rounds until a reviewer agrees it's done; check it, steer it, accept one she proposed, or cancel it.", Feature: "work"},
	"+schedule": {Group: "Background work", Usage: []string{"+schedule in <30m|2h|1d> <what to do>", "+schedule every <2h> <what to do>", "+schedule daily <HH:MM> <what to do>", "+schedule cancel <id>"},
		Text: "Run something once later, or on repeat (repeats are admin-only; times are UTC).", Feature: "work"},

	"+np":   {Group: "Radio", Usage: []string{"+np"}, Text: "What the web radio is playing.", Feature: "radio"},
	"+skip": {Group: "Radio", Usage: []string{"+skip"}, Text: "Skip the radio's current track (one skip per 20 seconds, for everyone).", Feature: "radio"},
}

// CheatEntry is one command on the cheatsheet.
type CheatEntry struct {
	Name    string
	Admin   bool
	Group   string
	Usage   []string
	Text    string
	Feature string
}

var plusCommand = regexp.MustCompile(`\+([a-z])`)

// Cheatsheet lists every registered command with a name, in CheatGroups order then by name, written
// with prefix. A command missing from commandHelp is listed under "Other" without a description.
func Cheatsheet(r *Registry, prefix string) []CheatEntry {
	withPrefix := func(s string) string { return plusCommand.ReplaceAllString(s, prefix+"$1") }
	var out []CheatEntry
	for _, cmd := range r.All() {
		name := cmd.Name()
		if name == "" {
			continue
		}
		h, ok := commandHelp[name]
		if !ok {
			h = CommandHelp{Group: "Other", Usage: []string{name}}
		}
		e := CheatEntry{Name: withPrefix(name), Admin: cmd.AdminOnly(), Group: h.Group, Text: withPrefix(h.Text), Feature: h.Feature}
		for _, u := range h.Usage {
			e.Usage = append(e.Usage, withPrefix(u))
		}
		out = append(out, e)
	}
	order := func(g string) int {
		if i := slices.Index(CheatGroups, g); i >= 0 {
			return i
		}
		return len(CheatGroups)
	}
	slices.SortStableFunc(out, func(a, b CheatEntry) int {
		if d := order(a.Group) - order(b.Group); d != 0 {
			return d
		}
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return out
}

// MissingHelp names registered commands that have no cheatsheet entry.
func MissingHelp(r *Registry) []string {
	var out []string
	for _, cmd := range r.All() {
		if _, ok := commandHelp[cmd.Name()]; cmd.Name() != "" && !ok {
			out = append(out, cmd.Name())
		}
	}
	return out
}
