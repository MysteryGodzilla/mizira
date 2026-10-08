// Copyright (C) 2026 BareMetal
// Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"regexp"
	"strings"
	"unicode"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// minLeakRun is how many consecutive significant words shared with the system prompt count as
// reproducing it.
const minLeakRun = 8

// significantWords reduces text to lowercase word tokens, dropping
// punctuation and case so that reformatting does not evade the check.
func significantWords(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// leaksSystemPrompt reports whether reply reproduces a run of the prompt.
func leaksSystemPrompt(reply, prompt string) bool {
	rw := significantWords(reply)
	pw := significantWords(prompt)
	if len(rw) < minLeakRun || len(pw) < minLeakRun {
		return false
	}

	seen := make(map[string]struct{}, len(pw))
	for i := 0; i+minLeakRun <= len(pw); i++ {
		seen[strings.Join(pw[i:i+minLeakRun], " ")] = struct{}{}
	}
	for i := 0; i+minLeakRun <= len(rw); i++ {
		if _, ok := seen[strings.Join(rw[i:i+minLeakRun], " ")]; ok {
			return true
		}
	}
	return false
}

// ScreenOutgoing decides whether a completed reply may be posted.
func ScreenOutgoing(ctx irc.ChatContextInterface, reply string) (bool, string) {
	cfg := ctx.GetConfig()
	if !outboundScreened(ctx) {
		return true, ""
	}
	if strings.TrimSpace(reply) == "" {
		return true, ""
	}

	// Deterministic first, and not negotiable by a classifier that is having
	// an off day.
	if leaksSystemPrompt(reply, cfg.Bot.Prompt) {
		return false, "reproduces the system prompt"
	}

	allowed, reason := core.Classify(ctx, cfg.Bot.ReplyScreenPolicy, "reply", neutralNicks(ctx, reply), false)
	return allowed, reason
}

// neutralName stands in for nicks in a reply under review.
const neutralName = "Sam"

// neutralNicks swaps the nicks of the speaker, the channel's users and known bots for a plain name before the
// reply screen sees them: a nick such as "BareMetal" or "rootkit" reads like infrastructure or a
// threat, and saying someone's name is never what the screen is for.
func neutralNicks(ctx irc.ChatContextInterface, reply string) string {
	nicks := []string{ctx.GetSource()}
	for _, u := range ctx.GetChannelUsers(ctx.GetConfig().Server.Channel) {
		nicks = append(nicks, u.Nick)
	}
	// Other bots' names too, nick or line tag: "[metalai]" read as a model name once.
	nicks = append(nicks, config.List(&ctx.GetConfig().Bot.BotNicks)...)
	for _, p := range config.List(&ctx.GetConfig().Bot.BotPrefixes) {
		nicks = append(nicks, strings.TrimFunc(p, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }))
	}
	var alts []string
	for _, n := range nicks {
		if len(n) >= 3 {
			alts = append(alts, regexp.QuoteMeta(n))
		}
	}
	if len(alts) == 0 {
		return reply
	}
	re, err := regexp.Compile(`(?i)(^|[^\w])(` + strings.Join(alts, "|") + `)([^\w]|$)`)
	if err != nil {
		return reply
	}
	return re.ReplaceAllString(reply, "${1}"+neutralName+"${3}")
}
