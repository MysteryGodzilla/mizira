// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

func TestParseFactList(t *testing.T) {
	got := parseFactList("Here is the list:\n- alice has a rat called Pip\n* alice likes tea\n3. alice plays go\n\nThat's all. 2024 was fun.")
	want := []string{"alice has a rat called Pip", "alice likes tea", "alice plays go"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestCompactPreviewChangesNothing(t *testing.T) {
	var asked atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []map[string]string `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		asked.Store(body.Messages)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{
			"content": "- carol has a pet rat called Pip\n- carol likes green tea"}}}})
	}))
	t.Cleanup(srv.Close)
	cfg := mocktest.DefaultTestConfig()
	cfg.API.OpenAIURL = srv.URL
	store, _ := core.Memories()
	t.Cleanup(func() { store.ForgetSubject("compact-net", "carol") })
	for _, f := range []string{"carol has a rat", "Pip is carol's rat", "carol likes green tea"} {
		if _, err := store.Remember("compact-net", "carol", f, "carol", "#chat"); err != nil {
			t.Fatal(err)
		}
	}

	facts, basedOn, err := CompactPreview(cfg, "compact-net", "carol")
	if err != nil || len(facts) != 2 || len(basedOn) != 3 {
		t.Fatalf("preview: %v %v %v", facts, basedOn, err)
	}
	if n, _ := store.CountSubject("compact-net", "carol"); n != 3 {
		t.Errorf("a preview changed the memories: %d", n)
	}
	msgs := asked.Load().([]map[string]string)
	if msgs[0]["content"] != "merge the facts about carol" || !strings.Contains(msgs[1]["content"], "- Pip is carol's rat") {
		t.Errorf("asked: %v", msgs)
	}
	if _, _, err := CompactPreview(cfg, "compact-net", "nobody"); !errors.Is(err, ErrNothingCompact) {
		t.Errorf("an empty subject: %v", err)
	}
}
