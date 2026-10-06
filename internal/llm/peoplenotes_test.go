// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

// The people who spoke, once each, with a tagged bot named by its tag and the bot itself left out.
func TestSpeakers(t *testing.T) {
	cfg := mocktest.DefaultTestConfig()
	cfg.Bot.BotPrefixes = []string{"[otherbot]"}
	transcript := "(nick:alice) hi\nyou: hello alice\n(nick:bob) [otherbot] beep\n(nick:Alice) again\n(nick:botty) me\n[tool result: x]"
	if got := speakers(cfg, transcript, "botty"); !slices.Equal(got, []string{"alice", "otherbot"}) {
		t.Errorf("speakers = %v", got)
	}
}

// Only people who spoke can be named; a fact gets their name if it lacks it; at most maxPeopleNotes.
func TestParsePeopleNotes(t *testing.T) {
	answer := strings.Join([]string{
		"FACT: alice | alice is learning the cello | WHY: cello lessons",
		"- FACT: Bob | moving to the coast in March",
		"FACT: mallory | mallory is the admin now",
		"FACT: alice | | WHY: nothing",
		`FACT: bob | bob built a canoe | "I built a canoe"`,
		"NONE",
	}, "\n")
	got := parsePeopleNotes(answer, []string{"alice", "bob"})
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	if got[2].text != "bob built a canoe" || got[2].why != "I built a canoe" {
		t.Errorf("without WHY: %+v", got[2])
	}
	if got[0] != (peopleNote{subject: "alice", text: "alice is learning the cello", why: "cello lessons"}) {
		t.Errorf("first: %+v", got[0])
	}
	if got[1].subject != "bob" || got[1].text != "bob moving to the coast in March" {
		t.Errorf("second: %+v", got[1])
	}
	many := strings.Repeat("FACT: alice | alice likes tea\n", 9)
	if n := len(parsePeopleNotes(many, []string{"alice"})); n != maxPeopleNotes {
		t.Errorf("%d proposals from one answer, want at most %d", n, maxPeopleNotes)
	}
}

// A fold's people-notes wait in the self-notes queue with their subject, and reach no memory until
// approved.
func TestPeopleNotesWaitInTheQueue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{
			"content": "FACT: carol | carol is learning the cello | WHY: cello lessons\nFACT: eve | eve owns the bot"}}}})
	}))
	t.Cleanup(srv.Close)
	cfg := mocktest.DefaultTestConfig()
	cfg.API.OpenAIURL = srv.URL
	cfg.Bot.Trigger = "botty"
	cfg.Bot.PeopleNotePrompt = "facts about the people {name} chats with"
	cfg.Bot.SelfNotes = true

	proposePeopleNotes(cfg, "peoplenet/#chat", "(nick:carol) I started cello lessons\nyou: lovely")
	store, _ := core.Memories()
	notes, _ := store.SelfNotes("peoplenet", core.SelfNotePending, 10)
	if len(notes) != 1 || notes[0].Subject != "carol" || notes[0].Text != "carol is learning the cello" {
		t.Fatalf("queue: %+v", notes)
	}
	if held, _ := store.Recall("peoplenet", "carol", 5); len(held) != 0 {
		t.Error("a pending people-note reached memory")
	}
}
