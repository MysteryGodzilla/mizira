// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"fmt"
	"strings"

	"B4reMetal/metald/internal/core"
)

// MemoryLine is one memory as the model reads it: its kind, whose avatar a look is when the avatar's
// pronouns differ from the person's, the fact, and whose word it is when not the person's own.
func MemoryLine(m core.Memory, meta core.MemoryMeta, people core.People) string {
	var b strings.Builder
	if meta.Kind != "" {
		fmt.Fprintf(&b, "[%s] ", meta.Kind)
	}
	if p, ok := people.Get(m.Subject); ok && meta.Kind == core.KindLook && p.AvatarPronouns != "" {
		fmt.Fprintf(&b, "%s's avatar (%s): ", m.Subject, p.AvatarPronouns)
	}
	b.WriteString(m.Fact)
	var notes []string
	if len(meta.Also) > 0 {
		notes = append(notes, "also about "+strings.Join(meta.Also, ", "))
	}
	switch core.MemorySource(m.Subject, m.Author) {
	case core.SourceTold:
		notes = append(notes, "per "+core.AuthorNick(m.Author))
	case core.SourceGuessed:
		notes = append(notes, "guessed from chat")
	}
	if len(notes) > 0 {
		fmt.Fprintf(&b, " (%s)", strings.Join(notes, "; "))
	}
	return b.String()
}

// ProfileLine is what a profile says about subject, or "" when there is none or it is empty:
// "bob: he/him; avatar she/her; also called bobby; runs the bot botty".
func ProfileLine(people core.People, subject string) string {
	p, ok := people.Get(subject)
	bots := people.Bots(subject)
	if !ok && len(bots) == 0 {
		return ""
	}
	if !ok {
		p.Subject = strings.ToLower(strings.TrimSpace(subject))
	}
	var parts []string
	switch {
	case p.RunBy != "":
		parts = append(parts, "a bot run by "+p.RunBy)
	case p.Bot:
		parts = append(parts, "a bot")
	}
	if p.Pronouns != "" {
		parts = append(parts, p.Pronouns)
	}
	if p.AvatarPronouns != "" {
		parts = append(parts, "avatar "+p.AvatarPronouns)
	}
	if len(p.Aliases) > 0 {
		parts = append(parts, "also called "+strings.Join(p.Aliases, ", "))
	}
	// The set look, as their look memories are left out: without it she couldn't say what they look
	// like, and described someone else in their picture.
	if p.Look.Set() {
		look := p.Look.Appearance
		if p.Look.Hair != "" {
			look += ", " + p.Look.Hair
		}
		parts = append(parts, "looks like "+look+" (added to pictures for you: name them)")
		if p.Look.Outfit != "" {
			parts = append(parts, "usually wears "+p.Look.Outfit)
		}
		var alts []string
		for _, v := range p.Look.Variants {
			alts = append(alts, v.Name)
		}
		if len(alts) > 0 {
			parts = append(parts, "alt looks (put the alt's name in a picture prompt to use it): "+strings.Join(alts, ", "))
		}
	}
	// No time zone: it says where someone lives, and profile__localtime gives the time when needed.
	for _, bot := range bots {
		parts = append(parts, "runs the bot "+bot)
	}
	if len(parts) == 0 {
		return ""
	}
	return p.Subject + ": " + strings.Join(parts, "; ")
}

// WithoutSetLooks drops the look memories of people whose profile sets their look: the model wrote
// them into picture prompts, and the picture had a second person beside the one the look describes.
func WithoutSetLooks(network string, mems []core.Memory) []core.Memory {
	store, err := core.Memories()
	if err != nil || len(mems) == 0 {
		return mems
	}
	people, err := store.People(network)
	if err != nil {
		return mems
	}
	ids := make([]int64, len(mems))
	for i, m := range mems {
		ids[i] = m.ID
	}
	meta, err := store.Meta(network, ids)
	if err != nil {
		return mems
	}
	out := mems[:0:0]
	for _, m := range mems {
		if p, ok := people.Get(m.Subject); ok && p.Look.Set() && meta[m.ID].Kind == core.KindLook {
			continue
		}
		out = append(out, m)
	}
	return out
}

// MemoryLines renders memories with their kinds, others and profiles looked up once. On a lookup
// error the facts go out bare.
func MemoryLines(network string, mems []core.Memory) []string {
	out := make([]string, len(mems))
	store, err := core.Memories()
	var meta map[int64]core.MemoryMeta
	var people core.People
	if err == nil {
		ids := make([]int64, len(mems))
		for i, m := range mems {
			ids[i] = m.ID
		}
		meta, _ = store.Meta(network, ids)
		people, _ = store.People(network)
	}
	for i, m := range mems {
		out[i] = MemoryLine(m, meta[m.ID], people)
	}
	return out
}
