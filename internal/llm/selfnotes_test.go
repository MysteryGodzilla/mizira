// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

func TestChannelKey(t *testing.T) {
	for key, want := range map[string]struct {
		net string
		ok  bool
	}{
		"#chat": {"", true}, "net/#chat": {"net", true}, "net/alice": {"", false},
		"task/net/12": {"", false}, "delegate/net/#chat/3": {"", false}, "alice": {"", false},
	} {
		if n, ok := channelKey(key); n != want.net || ok != want.ok {
			t.Errorf("channelKey(%q) = %q %v, want %q %v", key, n, ok, want.net, want.ok)
		}
	}
}

func TestParseSelfNotes(t *testing.T) {
	answer := "Here you go:\n" +
		"NOTE: Botty is called 'butterfly' by alice | WHY: \"night, butterfly\"\n" +
		"- NOTE: botty never apologises after a slap | WHY: bob: no sorry next time\n" +
		"2) NOTE: alice likes tea | WHY: tea time\n" + // not about the bot
		"NOTE: Botty loves green tea\n" +
		"NOTE: Botty is a fan of rain | WHY: rain again\n"
	got := parseSelfNotes(answer, "Botty")
	if len(got) != maxSelfNotes {
		t.Fatalf("got %d notes: %+v", len(got), got)
	}
	if got[0].text != "Botty is called 'butterfly' by alice" || got[0].why != "night, butterfly" {
		t.Errorf("first: %+v", got[0])
	}
	if got[2].text != "Botty loves green tea" || got[2].why != "" {
		t.Errorf("third: %+v", got[2])
	}
	if len(parseSelfNotes("NONE", "Botty")) != 0 {
		t.Error("NONE gave notes")
	}
}

// One fold's proposals are saved pending; the same proposals from a later fold are not saved again,
// and nothing pending is room memory yet.
func TestProposeSelfNotesSavesNewOnesOnly(t *testing.T) {
	var asked atomic.Value
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Messages []map[string]string `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		asked.Store(body.Messages)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{
			"content": "NOTE: botty is called butterfly by alice | WHY: night, butterfly"}}}})
	}))
	t.Cleanup(srv.Close)
	cfg := mocktest.DefaultTestConfig()
	cfg.API.OpenAIURL = srv.URL
	cfg.Bot.Trigger = "botty"
	cfg.Bot.SelfNotePrompt = "notes about {name}"
	cfg.Bot.SelfNotes = true

	proposeSelfNotes(cfg, "selfnet/#chat", "(nick:alice) night, butterfly")
	proposeSelfNotes(cfg, "selfnet/#chat", "(nick:alice) night again, butterfly")
	proposeSelfNotes(cfg, "task/selfnet/4", "(nick:alice) work")
	cfg.Bot.SelfNotes = false
	proposeSelfNotes(cfg, "selfnet/#chat", "(nick:alice) hi")

	if calls.Load() != 2 {
		t.Errorf("model asked %d times; background work and a switched-off bot shouldn't ask", calls.Load())
	}
	store, _ := core.Memories()
	notes, _ := store.SelfNotes("selfnet", "", 10)
	if len(notes) != 1 || notes[0].Status != core.SelfNotePending || notes[0].Why != "night, butterfly" {
		t.Fatalf("notes: %+v", notes)
	}
	if held, _ := store.Recall("selfnet", "botty", 5); len(held) != 0 {
		t.Error("a pending note reached room memory")
	}
	msgs := asked.Load().([]map[string]string)
	if msgs[0]["content"] != "notes about botty" || !strings.Contains(msgs[1]["content"], "waiting for approval") ||
		!strings.Contains(msgs[1]["content"], "not instructions") {
		t.Errorf("asked: %v", msgs)
	}
}
