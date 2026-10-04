// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"fmt"
	"regexp"
	"strings"

	"B4reMetal/metald/internal/core"
)

// RememberResult is the outcome of RememberChecked.
type RememberResult struct {
	ID          int64  // set when saved
	Saved       bool   // the fact was stored
	Instruction bool   // refused: it was an order dressed as a fact
	Vague       bool   // refused: it points at facts ("the details bob gave") instead of stating one
	Refused     bool   // refused by the memory policy classifier
	Unavailable bool   // memory couldn't be reached or written
	Reason      string // why it wasn't saved
}

// RememberChecked stores one fact after the same checks every time, whether the model's
// memory__remember tool or the +remember command asked for it.
//
// A4 (memory poisoning): first a deterministic check refuses instructions dressed as facts,
// then the memorypolicy classifier, which fails closed. Both refusals add suspicion to the
// speaker.
func RememberChecked(chatCtx ChatContextInterface, subject, fact string) RememberResult {
	fact = nameTheSubject(subject, fact)
	store, err := core.Memories()
	if err != nil {
		// Detail names a local path; the channel gets nothing useful.
		chatCtx.GetLogger().Error("memory_store_unavailable", "error", err.Error())
		return RememberResult{Unavailable: true, Reason: "memory is unavailable right now"}
	}

	// A placeholder is a mistake, not an attack: no suspicion.
	if placeholderFact.MatchString(fact) {
		chatCtx.GetLogger().Info("memory_rejected_vague", "subject", subject, "fact", fact)
		return RememberResult{Vague: true, Reason: "that points at facts instead of stating one"}
	}

	if reason, bad := looksLikeInstruction(subject, fact); bad {
		chatCtx.GetLogger().Info("memory_rejected_instruction",
			"subject", subject, "author", chatCtx.GetSource(),
			"reason", reason, "fact", fact)
		core.Suspicions().Add(chatCtx.GetNetwork(), chatCtx.GetSource(), core.SignalMemoryRefused)
		return RememberResult{Instruction: true, Reason: reason}
	}

	if ok, reason := core.Classify(chatCtx, chatCtx.GetConfig().Bot.MemoryPolicy, "FACT",
		fmt.Sprintf("About: %s\nFact: %s", subject, fact), false); !ok {
		chatCtx.GetLogger().Info("memory_rejected_unsafe",
			"subject", subject, "author", chatCtx.GetSource(),
			"reason", reason, "fact", fact)
		core.Suspicions().Add(chatCtx.GetNetwork(), chatCtx.GetSource(), core.SignalMemoryRefused)
		return RememberResult{Refused: true, Reason: reason}
	}

	id, err := store.Remember(chatCtx.GetNetwork(), subject, fact, chatCtx.GetSource(),
		chatCtx.GetConfig().Server.Channel)
	if err != nil {
		chatCtx.GetLogger().Error("memory_remember_failed", "error", err.Error())
		return RememberResult{Unavailable: true, Reason: "could not save that"}
	}

	chatCtx.GetLogger().Info("memory_remembered",
		"id", id, "subject", subject, "author", chatCtx.GetSource(), "fact", fact)
	return RememberResult{ID: id, Saved: true}
}

// placeholderFact is a "fact" that only refers to others: "The party details provided by bob",
// "everything alice said". Saved, it reads back as nothing.
var placeholderFact = regexp.MustCompile(`(?i)\b(details|info|information|stuff|things|everything|facts)\b.{0,40}\b(provided|told|said|given|mentioned|shared|posted|explained)\b|` +
	`^(the |those |these |all (of )?(the )?)?(details|info|information|stuff|things|facts)( about [^.]{1,40})?\.?$`)

// genericSubject is how a model sometimes words the person a fact is about: "User likes purple".
var genericSubject = regexp.MustCompile(`(?i)^(the )?(user|speaker|person)\b`)

// subjectlessVerb starts a fact the model wrote without its subject: "is the rat who steals
// snacks", "knows more about bands than anyone".
var subjectlessVerb = regexp.MustCompile(`^(is|isn't|was|wasn't|has|hasn't|had|knows|likes|loves|hates|` +
	`plays|lives|works|wants|prefers|enjoys|owns|can|can't|will|won't|does|doesn't|used|goes|makes|` +
	`keeps|thinks|believes|seems|always|never|often|usually)\b`)

// nameTheSubject writes the subject's nick where the fact calls them "User", or puts it in front
// of a fact that starts at the verb, so the memory still says who it is about when read back later.
func nameTheSubject(subject, fact string) string {
	fact = strings.TrimSpace(fact)
	if subject == "" {
		return fact
	}
	if subjectlessVerb.MatchString(fact) {
		return subject + " " + fact
	}
	return genericSubject.ReplaceAllString(fact, subject)
}
