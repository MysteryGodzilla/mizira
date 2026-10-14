// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"regexp"
	"strings"
	"sync"
	"time"
)

// Someone the gatekeeper refuses for harassment twice in a short while is ignored for longer than a
// flood timeout, without waiting for an operator.
const (
	HarassmentLimit  = 2
	HarassmentWindow = 10 * time.Minute
	HarassmentIgnore = time.Hour
	IgnoreByScreen   = "screening" // ignored after repeated harassment
)

// harassmentReason matches the gatekeeper's verdicts that are abuse rather than tricks: its reasons
// are free text ("TARGETED HARASSMENT OR SELF-HARM", "SEXUAL CONTENT").
var harassmentReason = regexp.MustCompile(`(?i)harass|harm|sexual|threat|hate|slur|abus|violen|bully`)

// IsHarassment reports whether a screening reason is abuse.
func IsHarassment(reason string) bool { return harassmentReason.MatchString(reason) }

var harassment = struct {
	sync.Mutex
	seen map[string][]time.Time
}{seen: map[string][]time.Time{}}

// NoteHarassment records a harassment screening and reports how many the speaker has had within
// HarassmentWindow, this one included.
func NoteHarassment(network, nick string, now time.Time) int {
	key := ScopeKey(network, strings.ToLower(nick))
	harassment.Lock()
	defer harassment.Unlock()
	var recent []time.Time
	for _, t := range harassment.seen[key] {
		if now.Sub(t) < HarassmentWindow {
			recent = append(recent, t)
		}
	}
	recent = append(recent, now)
	harassment.seen[key] = recent
	return len(recent)
}

// selfHarm matches a screening reason, or the message itself, that is about suicide or self-harm.
var selfHarm = regexp.MustCompile(`(?i)suicid|self[- ]?harm|kill(ing)? (my|him|her|them)sel|end (it all|my life)|` +
	`blow my brains|want to die|wanna die|hurt(ing)? myself`)

// IsSelfHarm reports whether a refused message is about suicide or self-harm, by the classifier's
// reason or the message's own words, so the refusal can say something better than the usual line.
func IsSelfHarm(reason, msg string) bool {
	return selfHarm.MatchString(reason) || selfHarm.MatchString(msg)
}
