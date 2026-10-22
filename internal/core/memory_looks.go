// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Look is how someone is drawn, set in the console so every picture of them matches instead of the
// prompt writer describing them afresh each time. Appearance (age, build, face, eyes, skin) goes into
// the picture word for word; Hair and Outfit are the defaults, which a variant named in the request
// replaces ("hori in her business look").
type Look struct {
	Appearance string        `json:"appearance"`
	Hair       string        `json:"hair,omitempty"`
	Outfit     string        `json:"outfit,omitempty"`
	Reference  string        `json:"reference,omitempty"` // a picture of them, kept for later use
	Variants   []LookVariant `json:"variants,omitempty"`
}

// LookVariant swaps the hair, the outfit or both.
type LookVariant struct {
	Name   string `json:"name"`
	Hair   string `json:"hair,omitempty"`
	Outfit string `json:"outfit,omitempty"`
}

const (
	maxLookText     = 400
	maxLookVariants = 8
)

var (
	variantName  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,19}$`)
	referenceURL = regexp.MustCompile(`^https://\S{4,300}$`)
)

// Set reports whether there is a look to draw from.
func (l Look) Set() bool { return l.Appearance != "" }

// For is the hair and outfit for a picture whose text names one of the variants (a whole word), or
// the defaults; variant is the one used, or "".
func (l Look) For(text string) (hair, outfit, variant string) {
	hair, outfit = l.Hair, l.Outfit
	words := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-')
	}) {
		words[w] = true
	}
	for _, v := range l.Variants {
		if words[v.Name] {
			if v.Hair != "" {
				hair = v.Hair
			}
			if v.Outfit != "" {
				outfit = v.Outfit
			}
			return hair, outfit, v.Name
		}
	}
	return hair, outfit, ""
}

// clean trims a look and says why it can't be saved.
func (l Look) clean() (Look, error) {
	text := func(field, s string) (string, error) {
		s = strings.Join(strings.Fields(s), " ")
		if len([]rune(s)) > maxLookText {
			return "", fmt.Errorf("%s: at most %d characters", field, maxLookText)
		}
		if strings.ContainsAny(s, "{}") {
			return "", fmt.Errorf("%s: no { or }", field)
		}
		return s, nil
	}
	var err error
	if l.Appearance, err = text("appearance", l.Appearance); err != nil {
		return l, err
	}
	if l.Hair, err = text("hair", l.Hair); err != nil {
		return l, err
	}
	if l.Outfit, err = text("outfit", l.Outfit); err != nil {
		return l, err
	}
	l.Reference = strings.TrimSpace(l.Reference)
	if l.Reference != "" && !referenceURL.MatchString(l.Reference) {
		return l, errors.New("the reference picture is an https link")
	}
	if l.Appearance == "" && (l.Hair != "" || l.Outfit != "" || len(l.Variants) > 0) {
		return l, errors.New("a look needs its appearance (age, build, face, eyes, skin)")
	}
	if len(l.Variants) > maxLookVariants {
		return l, fmt.Errorf("up to %d variants", maxLookVariants)
	}
	seen := map[string]bool{}
	for i, v := range l.Variants {
		v.Name = strings.ToLower(strings.TrimSpace(v.Name))
		if !variantName.MatchString(v.Name) {
			return l, fmt.Errorf("variant %q: one word, letters and numbers, like business or bun", v.Name)
		}
		if seen[v.Name] {
			return l, fmt.Errorf("variant %s is listed twice", v.Name)
		}
		seen[v.Name] = true
		if v.Hair, err = text("variant "+v.Name+" hair", v.Hair); err != nil {
			return l, err
		}
		if v.Outfit, err = text("variant "+v.Name+" outfit", v.Outfit); err != nil {
			return l, err
		}
		if v.Hair == "" && v.Outfit == "" {
			return l, fmt.Errorf("variant %s changes neither hair nor outfit", v.Name)
		}
		l.Variants[i] = v
	}
	return l, nil
}

func decodeLook(raw string) Look {
	var l Look
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &l)
	}
	return l
}

// SetLook saves how a subject is drawn, starting a profile for them if they have none; an empty look
// removes it.
func (m *MemoryStore) SetLook(network, subject string, look Look, by string) error {
	look, err := look.clean()
	if err != nil {
		return err
	}
	people, err := m.People(network)
	if err != nil {
		return err
	}
	p, ok := people.Get(subject)
	if !ok {
		if err := m.SetProfile(network, Profile{Subject: subject, By: by}); err != nil {
			return err
		}
		p.Subject = normalizeSubject(subject)
	}
	raw := ""
	if look.Set() {
		b, err := json.Marshal(look)
		if err != nil {
			return err
		}
		raw = string(b)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err = m.db.Exec(`UPDATE memory_profiles SET look = ?, by = ? WHERE network = ? AND subject = ?`,
		raw, by, network, p.Subject)
	return err
}
