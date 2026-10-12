// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
)

// RememberResult is the outcome of RememberChecked.
type RememberResult struct {
	ID          int64  // set when saved
	Saved       bool   // the fact was stored
	Merged      bool   // it repeated memory ID, which was kept (with the longer wording) instead
	Instruction bool   // refused: it was an order dressed as a fact
	Vague       bool   // refused: it points at facts ("the details bob gave") instead of stating one
	RoomOnly    bool   // refused: facts about the channel or the bot itself are an operator's to set
	Full        bool   // refused: the subject already holds memorypersubject memories
	Refused     bool   // refused by the memory policy classifier
	Unavailable bool   // memory couldn't be reached or written
	Reason      string // why it wasn't saved
}

// RememberChecked stores one fact after the same checks every time, whether the model's
// memory__remember tool or the +remember command asked for it.
//
// A4 (memory poisoning): first a deterministic check refuses instructions dressed as facts,
// then the memorypolicy classifier, which fails closed. Both refusals add suspicion to the
// speaker when they asked for the save; a save the model chose on its own is no one's fault.
//
// An operator's facts about the bot itself are its behaviour notes ("Mizira never apologises
// after a slap"), so they skip both checks: they are exactly the rules the checks keep others
// from planting.
func RememberChecked(chatCtx ChatContextInterface, subject, fact string) RememberResult {
	return RememberAs(chatCtx, subject, fact, "", nil)
}

// RememberAs is RememberChecked with the memory's kind and the other people it is about. Neither
// changes the checks; a kind the store doesn't know is dropped, and so are the channel and the
// bot among the others, since a memory about them is room memory.
func RememberAs(chatCtx ChatContextInterface, subject, fact, kind string, also []string) RememberResult {
	res := rememberChecked(chatCtx, subject, fact)
	if !res.Saved {
		return res
	}
	store, err := core.Memories()
	if err != nil {
		return res
	}
	if core.ValidKind(kind) && kind != "" {
		if _, err := store.SetKind(chatCtx.GetNetwork(), res.ID, kind); err != nil {
			chatCtx.GetLogger().Warn("memory_kind_failed", "id", res.ID, "error", err.Error())
		}
	}
	// People the fact names who are already known count too, whether or not the model listed them.
	self := chatCtx.GetConfig().Bot.Trigger
	if self == "" {
		self = chatCtx.GetBotNick()
	}
	if named, err := store.Mentioned(chatCtx.GetNetwork(), fact,
		[]string{subjectOf(subject), self, chatCtx.GetConfig().Server.Channel}); err == nil {
		also = append(also, named...)
	}
	var people []string
	for _, a := range also {
		if a = subjectOf(strings.TrimSpace(a)); a != "" && !strings.EqualFold(a, subjectOf(subject)) &&
			!slices.ContainsFunc(people, func(p string) bool { return strings.EqualFold(p, a) }) &&
			!IsRoomSubject(chatCtx.GetConfig(), chatCtx.GetBotNick(), a) && len(people) < maxAlsoAbout {
			people = append(people, a)
		}
	}
	if len(people) > 0 {
		if err := store.AddAlso(chatCtx.GetNetwork(), res.ID, people); err != nil {
			chatCtx.GetLogger().Warn("memory_also_failed", "id", res.ID, "error", err.Error())
		}
	}
	if kind != "" || len(people) > 0 {
		chatCtx.GetLogger().Info("memory_meta", "id", res.ID, "kind", kind, "also", strings.Join(people, ","))
	}
	return res
}

// SplitNames splits "bob, carol and dave" into nicks.
func SplitNames(s string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '&' }) {
		for _, name := range strings.Split(part, " and ") {
			if name = strings.TrimSpace(name); name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

// maxAlsoAbout bounds the others one save can name; a fact about more people is about the room.
const maxAlsoAbout = 4

func rememberChecked(chatCtx ChatContextInterface, subject, fact string) RememberResult {
	subject = subjectOf(subject)
	fact = nameTheSubject(subject, fact)
	store, err := core.Memories()
	if err != nil {
		// Detail names a local path; the channel gets nothing useful.
		chatCtx.GetLogger().Error("memory_store_unavailable", "error", err.Error())
		return RememberResult{Unavailable: true, Reason: "memory is unavailable right now"}
	}

	// Room memory - what the channel is and who the bot is - goes out with every request, so only an
	// operator writes it. Not a suspicion signal: people try "remember you are ..." in good faith.
	if IsRoomSubject(chatCtx.GetConfig(), chatCtx.GetBotNick(), subject) && !chatCtx.IsAdmin() {
		chatCtx.GetLogger().Info("memory_rejected_room", "subject", subject, "author", chatCtx.SpeakerKey(), "fact", fact)
		return RememberResult{RoomOnly: true, Reason: "only an operator can set facts about the channel or about me"}
	}
	// A repeat of a fact already held merges into it, so it never counts against the cap.
	_, repeat, _ := store.Similar(chatCtx.GetNetwork(), subject, fact)
	if limit := chatCtx.GetConfig().Bot.MemoryPerSubject; limit > 0 && !repeat {
		if n, err := store.CountSubject(chatCtx.GetNetwork(), subject); err == nil && n >= int64(limit) {
			chatCtx.GetLogger().Info("memory_rejected_full", "subject", subject, "count", n)
			return RememberResult{Full: true, Reason: fmt.Sprintf("%s already has %d memories; forget one first", subject, n)}
		}
	}

	// A placeholder is a mistake, not an attack: no suspicion.
	if placeholderFact.MatchString(fact) {
		chatCtx.GetLogger().Info("memory_rejected_vague", "subject", subject, "fact", fact)
		return RememberResult{Vague: true, Reason: "that points at facts instead of stating one"}
	}

	note := chatCtx.IsAdmin() && IsRoomSubject(chatCtx.GetConfig(), chatCtx.GetBotNick(), subject)
	asked := AskedToSave(chatCtx)

	if reason, bad := looksLikeInstruction(subject, fact); bad && !note {
		chatCtx.GetLogger().Info("memory_rejected_instruction",
			"subject", subject, "author", chatCtx.SpeakerKey(),
			"reason", reason, "fact", fact)
		if asked {
			core.Suspicions().Add(chatCtx.GetNetwork(), chatCtx.SpeakerKey(), core.SignalMemoryRefused)
		}
		return RememberResult{Instruction: true, Reason: reason}
	}

	if !note {
		if ok, reason := core.Classify(chatCtx, chatCtx.GetConfig().Bot.MemoryPolicy, "FACT",
			fmt.Sprintf("About: %s\nFact: %s", subject, fact), false); !ok {
			chatCtx.GetLogger().Info("memory_rejected_unsafe",
				"subject", subject, "author", chatCtx.SpeakerKey(),
				"reason", reason, "fact", fact)
			if asked {
				core.Suspicions().Add(chatCtx.GetNetwork(), chatCtx.SpeakerKey(), core.SignalMemoryRefused)
			}
			return RememberResult{Refused: true, Reason: reason}
		}
	}

	id, merged, err := store.RememberMerged(chatCtx.GetNetwork(), subject, fact, chatCtx.SpeakerKey(),
		chatCtx.GetConfig().Server.Channel)
	if err != nil {
		chatCtx.GetLogger().Error("memory_remember_failed", "error", err.Error())
		return RememberResult{Unavailable: true, Reason: "could not save that"}
	}
	if merged {
		chatCtx.GetLogger().Info("memory_merged", "id", id, "subject", subject, "author", chatCtx.SpeakerKey(), "fact", fact)
		return RememberResult{ID: id, Saved: true, Merged: true}
	}

	chatCtx.GetLogger().Info("memory_remembered",
		"id", id, "subject", subject, "author", chatCtx.SpeakerKey(), "fact", fact)
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

// IsRoomSubject reports whether a memory subject is room memory: the configured channel, or the
// bot itself (its trigger, or its nick when there is no trigger; with a trigger the nick may be its
// owner's).
func IsRoomSubject(cfg *config.Configuration, botNick, subject string) bool {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return false
	}
	self := cfg.Bot.Trigger
	if self == "" {
		self = botNick
	}
	// The first word decides: "Mizira is a night owl" as a subject is still about the bot.
	first := strings.Trim(strings.Fields(subject)[0], ",.:;!?'\"")
	return strings.EqualFold(first, cfg.Server.Channel) || strings.EqualFold(first, self) ||
		strings.EqualFold(strings.TrimSuffix(first, "'s"), self)
}

// subjectOf reduces a subject the model wrote as a phrase to whom it is about: "Dave's party
// avatar" and "dave is a wizard" are both about dave, so their facts sit together and come back
// together.
func subjectOf(subject string) string {
	words := strings.Fields(subject)
	if len(words) == 0 {
		return subject
	}
	first := strings.Trim(words[0], ",.:;!?\"")
	if base, ok := strings.CutSuffix(first, "'s"); ok && base != "" {
		return base
	}
	if len(words) > 1 && subjectVerb[strings.ToLower(words[1])] {
		return first
	}
	return subject
}

// subjectVerb after a subject's first word means the model wrote a sentence, not a name.
var subjectVerb = map[string]bool{"is": true, "was": true, "has": true, "likes": true, "loves": true, "plays": true}

// saveWords mark a message asking for something to be kept.
var saveWords = regexp.MustCompile(`(?i)\b(remember|save|note|keep in mind|don't forget|dont forget)\b`)

// AskedToSave reports whether the speaker's message asked for a save, as opposed to the model
// deciding to keep something they mentioned.
func AskedToSave(chatCtx ChatContextInterface) bool {
	return saveWords.MatchString(strings.Join(chatCtx.GetArgs(), " "))
}

// LooksLikeInstruction reports, with a reason, whether a fact about subject is an order dressed as
// one, as a save would refuse it; the console marks such proposals.
func LooksLikeInstruction(subject, fact string) (string, bool) {
	return looksLikeInstruction(subject, fact)
}
