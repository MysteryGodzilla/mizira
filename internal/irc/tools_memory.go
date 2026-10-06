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

	"github.com/alexschlessinger/pollytool/schema"
	"github.com/alexschlessinger/pollytool/tools"

	"B4reMetal/metald/internal/core"
)

// Memory tools.
func newMemoryRememberTool() tools.Tool {
	return &tools.Func{
		Name: "memory__remember",
		Desc: "Store a durable fact about a person or thing, so you still know " +
			"it after your short-term conversation memory is cleared or expires. " +
			"Use it for things worth keeping: someone's pronouns, what they work " +
			"on, a running joke, a preference they stated, a promise you made. " +
			"Use it without being asked when someone tells you something lasting " +
			"about themselves (\"purple is my favourite colour\", \"I just adopted a " +
			"dog called Biscuit\"). Notes on how you should behave are kept only when an " +
			"operator gives them; try, and the save says if it isn't allowed. Do NOT use it " +
			"for passing chatter or for anything you were asked to keep private. One fact per call, " +
			"written so it still makes sense read back cold in a month.",
		Params: schema.Params{
			"subject": schema.S("Who or what this is about - usually a nick. When it is about you yourself (how you should behave, what you like), your own name"),
			"fact":    schema.S("The single fact to remember, in plain words"),
		},
		Required: []string{"subject", "fact"},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, err := validateContext(ctx)
			if err != nil {
				return "", err
			}
			subject := strings.TrimSpace(args.String("subject"))
			fact := strings.TrimSpace(args.String("fact"))
			if subject == "" || fact == "" {
				return "", fmt.Errorf("subject and fact are both required")
			}

			res := RememberChecked(chatCtx, subject, fact)
			switch {
			case res.Unavailable:
				return "Error: " + res.Reason, nil
			case res.Instruction:
				return fmt.Sprintf(
					"Refused: that is an instruction, not a fact about %s (%s). "+
						"If it is worth remembering, record what they DID or ASKED FOR, "+
						"naming them - e.g. \"%s asked to be insulted harder\" - not a "+
						"rule for you to follow.", subject, res.Reason, subject), nil
			case res.RoomOnly:
				return "Not saved: only an operator can set facts about the channel or about you. " +
					"Say so briefly and kindly.", nil
			case res.Full:
				return "Not saved: " + res.Reason + ". Say so briefly.", nil
			case res.Vague:
				return "Not saved: that only points at what was said and names no fact. Now call " +
					"memory__remember again once for each fact you were told, with the person or thing " +
					"it is about as the subject - e.g. subject \"bob\", fact \"bob plays the bass\". " +
					"Don't reply until they are saved.", nil
			case res.Refused:
				return fmt.Sprintf(
					"Refused: not storing that (%s). Say so briefly in your own "+
						"voice and move on - do not repeat the fact back.", res.Reason), nil
			}
			return fmt.Sprintf("Remembered about %s: %s. Mention briefly that you'll remember it, in your own voice.",
				subjectOf(subject), fact), nil
		},
	}
}

func newMemoryRecallTool() tools.Tool {
	return &tools.Func{
		Name: "memory__recall",
		Desc: "Look up what you already know about a person or topic. Use it " +
			"when someone asks what you remember, when a name comes up that you " +
			"might have stored something about, or before claiming you don't " +
			"know someone. Returns nothing if you have no memories about them - " +
			"in which case say so rather than inventing something.",
		Params: schema.Params{
			"subject": schema.S("Who or what to look up - usually a nick"),
		},
		Required: []string{"subject"},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, err := validateContext(ctx)
			if err != nil {
				return "", err
			}
			store, err := core.Memories()
			if err != nil {
				chatCtx.GetLogger().Error("memory_store_unavailable", "error", err.Error())
				return "Error: memory is unavailable right now", nil
			}

			subject := strings.TrimSpace(args.String("subject"))
			if subject == "" {
				return "", fmt.Errorf("subject is required")
			}

			mems, err := store.Recall(chatCtx.GetNetwork(), subject, 20)
			if err == nil && len(mems) == 0 {
				mems, err = store.Search(chatCtx.GetNetwork(), subject, 20)
			}
			if err != nil {
				chatCtx.GetLogger().Error("memory_recall_failed", "error", err.Error())
				return "Error: could not read memory", nil
			}

			chatCtx.GetLogger().Info("memory_recalled", "subject", subject, "hits", len(mems))
			if len(mems) == 0 {
				if hasTool(chatCtx, "history__search") {
					return fmt.Sprintf("Nothing remembered about %s. If they mean something said in the "+
						"channel, look with history__search; otherwise say so plainly - do not invent a "+
						"memory.", subject), nil
				}
				return fmt.Sprintf("Nothing remembered about %s. Say so plainly - do not invent a memory.", subject), nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "%d memory(ies) about %s:", len(mems), subject)
			for _, m := range mems {
				fmt.Fprintf(&b, "\n  [%d] %s", m.ID, m.Fact)
			}
			return b.String(), nil
		},
	}
}

func newMemoryForgetTool() tools.Tool {
	return &tools.Func{
		Name: "memory__forget",
		Desc: "Delete one memory you hold about someone, named by its words: who it is about and " +
			"what it says (\"plays the guitar\"). Use it when someone asks you to forget something " +
			"about them, or when a memory turns out to be wrong. Anyone may ask you to forget things " +
			"about THEMSELVES; only operators may erase memories about other people.",
		Params: schema.Params{
			"subject": schema.S("Who the memory is about, usually a nick; the person asking if it's about them"),
			"fact":    schema.S("Words from the memory to forget, e.g. \"plays the guitar\""),
		},
		Required: []string{"fact"},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, err := validateContext(ctx)
			if err != nil {
				return "", err
			}
			// An id comes only from the person's own message ("Mizira forget 6"); the schema doesn't
			// offer one, so the model can't guess ids.
			if id := args.Int("id", 0); id > 0 {
				return forgetByID(chatCtx, int64(id)), nil
			}
			subject := strings.TrimSpace(args.String("subject"))
			if subject == "" {
				subject = chatCtx.Speaker()
			}
			query := strings.TrimSpace(args.String("fact"))
			if query == "" {
				return "", fmt.Errorf("say which memory to forget, in its own words")
			}
			if !strings.EqualFold(subject, chatCtx.Speaker()) && !chatCtx.IsAdmin() {
				chatCtx.GetLogger().Info("memory_forget_denied",
					"subject", subject, "requested_by", chatCtx.SpeakerKey())
				return fmt.Sprintf("Refused: those memories are about %s, not about the person asking. Only they or an operator can remove them.", subject), nil
			}
			store, err := core.Memories()
			if err != nil {
				chatCtx.GetLogger().Error("memory_store_unavailable", "error", err.Error())
				return "Error: memory is unavailable right now", nil
			}
			if everything.MatchString(query) {
				n, err := store.ForgetSubject(chatCtx.GetNetwork(), subject)
				if err != nil {
					chatCtx.GetLogger().Error("memory_forget_failed", "error", err.Error())
					return "Error: could not forget that", nil
				}
				chatCtx.GetLogger().Info("memories_cleared", "subject", subject, "count", n, "by", chatCtx.SpeakerKey())
				kept := ""
				if locked := store.LockedCount(chatCtx.GetNetwork(), subject); locked > 0 {
					kept = fmt.Sprintf(" %d locked memories about %s were kept: only the operator can remove those. Say that too.", locked, subject)
				}
				if n == 0 {
					return fmt.Sprintf("Nothing was forgotten about %s.%s Say so plainly.", subject, kept), nil
				}
				return fmt.Sprintf("Forgot %d memories about %s.%s Say briefly that it's done.", n, subject, kept), nil
			}
			mems, err := store.Recall(chatCtx.GetNetwork(), subject, 50)
			if err != nil {
				chatCtx.GetLogger().Error("memory_lookup_failed", "error", err.Error())
				return "Error: could not read memory", nil
			}

			match, candidates := bestMemoryMatch(mems, query)
			if match == nil {
				if len(candidates) == 0 {
					return fmt.Sprintf("Nothing remembered about %s matches %q. Say so plainly.", subject, query), nil
				}
				return fmt.Sprintf("More than one memory about %s could match %q: %s. Ask which one, or call "+
					"again with more of its words.", subject, query, quoteFacts(candidates)), nil
			}
			if store.IsLocked(chatCtx.GetNetwork(), match.ID) {
				return fmt.Sprintf("Not forgotten: the operator has locked the memory %q, so it can't be forgotten from chat. Tell them it's locked.", match.Fact), nil
			}
			ok, err := store.Forget(chatCtx.GetNetwork(), match.ID)
			if err != nil {
				chatCtx.GetLogger().Error("memory_forget_failed", "error", err.Error())
				return "Error: could not forget that", nil
			}
			if !ok {
				return fmt.Sprintf("Nothing remembered about %s matches %q.", subject, query), nil
			}
			chatCtx.GetLogger().Info("memory_forgotten",
				"id", match.ID, "subject", match.Subject, "fact", match.Fact, "requested_by", chatCtx.SpeakerKey())
			return fmt.Sprintf("Forgot about %s: %q. Say briefly that it's forgotten.", subject, match.Fact), nil
		},
	}
}

// looksLikeInstruction reports whether a "fact" is really a standing order.
var (
	obligationWords = regexp.MustCompile(`(?i)\b(always|never|must|should|shall|do not|don't|make sure|ensure|be sure to|has to|have to|required to)\b`)
	botAddressed    = regexp.MustCompile(`(?i)\b(you (must|should|will|are to|may not|can't|cannot|shall)|from now on|never refuse|always reply|always respond|ignore (your|all|previous)|the bot (must|should|will|always|never))\b`)
)

func looksLikeInstruction(subject, fact string) (string, bool) {
	if botAddressed.MatchString(fact) {
		return "addressed to the bot", true
	}
	if obligationWords.MatchString(fact) &&
		!strings.Contains(strings.ToLower(fact), strings.ToLower(subject)) {
		return "an order with no subject", true
	}
	return "", false
}

// forgetStopWords carry no meaning for matching a memory: "that", "the", "my".
var forgetStopWords = map[string]bool{
	"the": true, "and": true, "that": true, "this": true, "you": true, "your": true, "my": true,
	"me": true, "i": true, "a": true, "an": true, "is": true, "are": true, "was": true, "to": true,
	"of": true, "about": true, "for": true, "it": true, "they": true, "he": true, "she": true,
	"his": true, "her": true, "their": true, "will": true, "be": true, "in": true, "on": true,
}

func memoryWords(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !isTriggerWordChar(r) && r != '\''
	}) {
		if !forgetStopWords[w] {
			out = append(out, w)
		}
	}
	return out
}

// bestMemoryMatch picks the memory whose words cover most of query's. It returns nil and the tied
// or near candidates when no single memory clearly matches, and nil, nil when none matches at all.
func bestMemoryMatch(mems []core.Memory, query string) (*core.Memory, []core.Memory) {
	want := memoryWords(query)
	if len(want) == 0 {
		return nil, nil
	}
	score := func(m core.Memory) float64 {
		have := map[string]bool{}
		for _, w := range memoryWords(m.Fact) {
			have[w] = true
		}
		hits := 0
		for _, w := range want {
			if have[w] {
				hits++
			}
		}
		return float64(hits) / float64(len(want))
	}
	var best []core.Memory
	top := 0.0
	for _, m := range mems {
		switch sc := score(m); {
		case sc < 0.5:
		case sc > top:
			top, best = sc, []core.Memory{m}
		case sc == top:
			best = append(best, m)
		}
	}
	if len(best) == 1 {
		return &best[0], nil
	}
	return nil, best
}

func quoteFacts(mems []core.Memory) string {
	parts := make([]string, 0, len(mems))
	for _, m := range mems {
		parts = append(parts, strconv.Quote(m.Fact))
	}
	return strings.Join(parts, ", ")
}

// hasTool reports whether this request can call the named tool.
func hasTool(chatCtx ChatContextInterface, name string) bool {
	sys := chatCtx.GetSystem()
	if sys == nil || sys.GetToolRegistry() == nil {
		return false
	}
	_, ok := sys.GetToolRegistry().Get(name)
	return ok
}

// everything is a forget request for every memory about the subject.
var everything = regexp.MustCompile(`(?i)^(everything|all|all of (it|them|that)|everything (about|on) \S+|all about \S+)[.!]*$`)

// forgetByID deletes one memory named by its id, with the same rule as +memories forget: your own
// memories are yours to delete, anyone else's need an operator.
func forgetByID(chatCtx ChatContextInterface, id int64) string {
	store, err := core.Memories()
	if err != nil {
		chatCtx.GetLogger().Error("memory_store_unavailable", "error", err.Error())
		return "Error: memory is unavailable right now"
	}
	mem, found, err := store.Get(chatCtx.GetNetwork(), id)
	if err != nil {
		chatCtx.GetLogger().Error("memory_lookup_failed", "error", err.Error())
		return "Error: could not read memory"
	}
	if !found {
		return fmt.Sprintf("There is no memory number %d. Say so plainly.", id)
	}
	if !strings.EqualFold(mem.Subject, chatCtx.Speaker()) && !chatCtx.IsAdmin() {
		chatCtx.GetLogger().Info("memory_forget_denied", "id", id, "subject", mem.Subject, "requested_by", chatCtx.SpeakerKey())
		return fmt.Sprintf("Refused: memory %d is about %s, not about the person asking. Only they or an operator can remove it.", id, mem.Subject)
	}
	if store.IsLocked(chatCtx.GetNetwork(), id) {
		return fmt.Sprintf("Not forgotten: the operator has locked memory %d, so it can't be forgotten from chat. Tell them it's locked.", id)
	}
	if ok, err := store.Forget(chatCtx.GetNetwork(), id); err != nil || !ok {
		return "Error: could not forget that"
	}
	chatCtx.GetLogger().Info("memory_forgotten", "id", id, "subject", mem.Subject, "fact", mem.Fact, "requested_by", chatCtx.SpeakerKey())
	return fmt.Sprintf("Forgot memory %d about %s: %q. Say briefly that it's forgotten.", id, mem.Subject, mem.Fact)
}

// OrdersTheBot reports text worded as standing orders for the bot ("from now on", "you must",
// "ignore your rules"), the deterministic half of the instruction check.
func OrdersTheBot(text string) bool { return botAddressed.MatchString(text) }
