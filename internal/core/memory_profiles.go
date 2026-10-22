// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
)

// A profile is what is set about someone rather than remembered: their pronouns, their avatar's
// when those differ, other names they go by, and for a bot, who runs it. Only the console sets one.

// Profile is one person's or bot's profile.
type Profile struct {
	Subject        string
	Bot            bool
	RunBy          string // the nick that runs this bot
	Pronouns       string
	AvatarPronouns string // only when the avatar's differ from the person's
	Aliases        []string
	Timezone       string // an IANA name, e.g. Australia/Brisbane; "" when not known
	Look           Look   // how they are drawn; set with SetLook, which SetProfile leaves alone
	By             string
	Updated        time.Time
}

const (
	maxPronouns = 24
	maxAliases  = 5
	maxName     = 32
)

func createProfiles(db *sql.DB) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS memory_profiles (
		network         TEXT NOT NULL,
		subject         TEXT NOT NULL,
		bot             INTEGER NOT NULL DEFAULT 0,
		run_by          TEXT NOT NULL DEFAULT '',
		pronouns        TEXT NOT NULL DEFAULT '',
		avatar_pronouns TEXT NOT NULL DEFAULT '',
		aliases         TEXT NOT NULL DEFAULT '',
		by              TEXT NOT NULL DEFAULT '',
		updated         INTEGER NOT NULL,
		PRIMARY KEY (network, subject)
	);`)
	if err != nil {
		return err
	}
	if !hasColumn(db, "memory_profiles", "timezone") {
		if _, err = db.Exec(`ALTER TABLE memory_profiles ADD COLUMN timezone TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	if !hasColumn(db, "memory_profiles", "look") {
		_, err = db.Exec(`ALTER TABLE memory_profiles ADD COLUMN look TEXT NOT NULL DEFAULT ''`)
	}
	return err
}

// ValidTimezone reports whether tz is an IANA time zone name Go knows, like "Australia/Melbourne".
// "Local" and "" are refused: the bot's own zone means nothing to the person.
func ValidTimezone(tz string) bool {
	if tz == "" || strings.EqualFold(tz, "local") {
		return false
	}
	_, err := time.LoadLocation(tz)
	return err == nil
}

// validName accepts one word that could be a nick or a name: no spaces or punctuation that IRC
// wouldn't put in a nick.
func validName(s string) bool {
	if s == "" || len(s) > maxName {
		return false
	}
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) || strings.ContainsRune(",!@*?:;\"'", r) {
			return false
		}
	}
	return true
}

// validPronouns accepts short text like "she/her" or "he/they".
func validPronouns(s string) bool {
	if len(s) > maxPronouns {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || strings.ContainsRune("{}<>[]", r) {
			return false
		}
	}
	return true
}

// SetProfile saves a profile, replacing the subject's old one. A bot run by someone is a bot.
func (m *MemoryStore) SetProfile(network string, p Profile) error {
	p.Subject = normalizeSubject(p.Subject)
	p.RunBy = normalizeSubject(p.RunBy)
	p.Pronouns = strings.TrimSpace(p.Pronouns)
	p.AvatarPronouns = strings.TrimSpace(p.AvatarPronouns)
	p.Timezone = strings.TrimSpace(p.Timezone)
	p.Aliases = CleanAlso(p.Subject, p.Aliases)
	switch {
	case !validName(p.Subject):
		return errors.New("a profile is for one nick or name")
	case p.RunBy != "" && (!validName(p.RunBy) || p.RunBy == p.Subject):
		return errors.New("run by takes the one nick that runs the bot")
	case !validPronouns(p.Pronouns) || !validPronouns(p.AvatarPronouns):
		return fmt.Errorf("pronouns are short, like she/her (up to %d characters)", maxPronouns)
	case len(p.Aliases) > maxAliases:
		return fmt.Errorf("up to %d other names", maxAliases)
	case p.Timezone != "" && !ValidTimezone(p.Timezone):
		return fmt.Errorf("%q isn't a time zone name; use one like Australia/Brisbane or America/New_York", p.Timezone)
	}
	for _, a := range p.Aliases {
		if !validName(a) {
			return fmt.Errorf("%q isn't one name", a)
		}
	}
	if p.AvatarPronouns == p.Pronouns {
		p.AvatarPronouns = ""
	}
	if p.RunBy != "" {
		p.Bot = true
	}
	others, err := m.Profiles(network)
	if err != nil {
		return err
	}
	for _, o := range others {
		if o.Subject == p.Subject {
			continue
		}
		for _, a := range p.Aliases {
			if a == o.Subject || slices.Contains(o.Aliases, a) {
				return fmt.Errorf("%q already names %s", a, o.Subject)
			}
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err = m.db.Exec(`INSERT INTO memory_profiles
		(network, subject, bot, run_by, pronouns, avatar_pronouns, aliases, timezone, by, updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(network, subject) DO UPDATE SET bot = excluded.bot, run_by = excluded.run_by,
		  pronouns = excluded.pronouns, avatar_pronouns = excluded.avatar_pronouns,
		  aliases = excluded.aliases, timezone = excluded.timezone, by = excluded.by, updated = excluded.updated`,
		network, p.Subject, p.Bot, p.RunBy, p.Pronouns, p.AvatarPronouns, strings.Join(p.Aliases, ","),
		p.Timezone, p.By, time.Now().Unix())
	return err
}

// DeleteProfile removes a subject's profile. False if there wasn't one.
func (m *MemoryStore) DeleteProfile(network, subject string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res, err := m.db.Exec(`DELETE FROM memory_profiles WHERE network = ? AND subject = ?`,
		network, normalizeSubject(subject))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Profiles lists a network's profiles by subject.
func (m *MemoryStore) Profiles(network string) ([]Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rows, err := m.db.Query(`SELECT subject, bot, run_by, pronouns, avatar_pronouns, aliases, timezone, look, by, updated
		FROM memory_profiles WHERE network = ? ORDER BY subject`, network)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Profile
	for rows.Next() {
		var p Profile
		var aliases, look string
		var updated int64
		if err := rows.Scan(&p.Subject, &p.Bot, &p.RunBy, &p.Pronouns, &p.AvatarPronouns, &aliases, &p.Timezone, &look, &p.By, &updated); err != nil {
			return nil, err
		}
		p.Look = decodeLook(look)
		if aliases != "" {
			p.Aliases = strings.Split(aliases, ",")
		}
		p.Updated = time.Unix(updated, 0)
		out = append(out, p)
	}
	return out, rows.Err()
}

// People is a network's profiles, looked up by subject or by any other name they go by.
type People struct {
	bySubject map[string]Profile
	byName    map[string]string
}

// People loads a network's profiles for lookups.
func (m *MemoryStore) People(network string) (People, error) {
	list, err := m.Profiles(network)
	if err != nil {
		return People{}, err
	}
	p := People{bySubject: map[string]Profile{}, byName: map[string]string{}}
	for _, pr := range list {
		p.bySubject[pr.Subject] = pr
		p.byName[pr.Subject] = pr.Subject
		for _, a := range pr.Aliases {
			p.byName[a] = pr.Subject
		}
	}
	return p, nil
}

// Resolve returns the subject a name stands for: the subject itself, or whose alias it is.
func (p People) Resolve(name string) string {
	name = normalizeSubject(name)
	if s, ok := p.byName[name]; ok {
		return s
	}
	return name
}

// Get returns a subject's profile.
func (p People) Get(subject string) (Profile, bool) {
	pr, ok := p.bySubject[normalizeSubject(subject)]
	return pr, ok
}

// Bots lists the bots subject runs.
func (p People) Bots(subject string) []string {
	subject = normalizeSubject(subject)
	var out []string
	for _, pr := range p.bySubject {
		if pr.RunBy == subject {
			out = append(out, pr.Subject)
		}
	}
	slices.Sort(out)
	return out
}

// Names lists every name a profile answers to, mapped to its subject.
func (p People) Names() map[string]string { return p.byName }

// SetTimezone sets one subject's time zone, keeping the rest of their profile (or starting one).
func (m *MemoryStore) SetTimezone(network, subject, tz, by string) error {
	people, err := m.People(network)
	if err != nil {
		return err
	}
	p, ok := people.Get(subject)
	if !ok {
		p = Profile{Subject: subject}
	}
	p.Timezone, p.By = tz, by
	return m.SetProfile(network, p)
}

// Location is a subject's time zone, if their profile has one.
func (p People) Location(subject string) (*time.Location, bool) {
	pr, ok := p.Get(subject)
	if !ok || pr.Timezone == "" {
		return nil, false
	}
	loc, err := time.LoadLocation(pr.Timezone)
	return loc, err == nil
}
