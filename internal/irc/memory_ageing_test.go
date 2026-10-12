// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

// fakeModel allows every memory and answers a look merge with merged.
func fakeModel(t *testing.T, merged string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		answer := "ALLOW"
		if strings.HasPrefix(req.Messages[0].Content, "merge the looks") {
			answer = merged
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": answer}}}})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func ageingMock(t *testing.T, network, model string) *mocktest.MockChatContext {
	mock := mocktest.NewMockContext().WithSource("bob")
	cfg := mock.GetConfig()
	cfg.Server.Name = network
	cfg.API.OpenAIURL = model
	store, _ := core.Memories()
	t.Cleanup(func() { store.Clear(network) })
	return mock
}

// A new look and the one held become one description; the old one is kept as an earlier version.
func TestNewLookMergesIntoTheHeldOne(t *testing.T) {
	srv := fakeModel(t, "bob's avatar has long silver hair and wears a red scarf.")
	mock := ageingMock(t, "look-merge", srv.URL)
	store, _ := core.Memories()

	first := RememberAs(mock, "bob", "bob's avatar has long silver hair", core.KindLook, nil)
	second := RememberAs(mock, "bob", "bob's avatar wears a red scarf", core.KindLook, nil)
	if !first.Saved || !second.Saved {
		t.Fatalf("saves = %+v %+v", first, second)
	}
	got, _ := store.Recall("look-merge", "bob", 10)
	if len(got) != 1 || got[0].ID != second.ID || got[0].Fact != "bob's avatar has long silver hair and wears a red scarf." {
		t.Errorf("memories = %+v", got)
	}
	if h, _ := store.History("look-merge", "bob", 10); len(h) != 1 || h[0].Fact != "bob's avatar has long silver hair" {
		t.Errorf("history = %+v", h)
	}
}

// With the model unreachable, both looks stay.
func TestLookMergeFailureKeepsBoth(t *testing.T) {
	srv := fakeModel(t, "")
	mock := ageingMock(t, "look-keep", srv.URL)
	store, _ := core.Memories()
	RememberAs(mock, "bob", "bob's avatar has long silver hair", core.KindLook, nil)
	RememberAs(mock, "bob", "bob's avatar wears a red scarf", core.KindLook, nil)
	if got, _ := store.Recall("look-keep", "bob", 10); len(got) != 2 {
		t.Errorf("memories = %+v", got)
	}
}

func TestNegatedLikeReplacesTheOldOne(t *testing.T) {
	srv := fakeModel(t, "")
	mock := ageingMock(t, "like-flip", srv.URL)
	store, _ := core.Memories()
	RememberAs(mock, "bob", "bob likes pineapple on pizza", core.KindLikes, nil)
	RememberAs(mock, "bob", "bob likes trains", core.KindLikes, nil)
	now := RememberAs(mock, "bob", "bob doesn't like pineapple on pizza", core.KindLikes, nil)
	got, _ := store.Recall("like-flip", "bob", 10)
	if len(got) != 2 || (got[0].ID != now.ID && got[1].ID != now.ID) {
		t.Errorf("memories = %+v", got)
	}
	if h, _ := store.History("like-flip", "bob", 10); len(h) != 1 || h[0].Why != "contradicted" {
		t.Errorf("history = %+v", h)
	}
}

func TestRecallByKind(t *testing.T) {
	srv := fakeModel(t, "")
	mock := ageingMock(t, "recall-kind", srv.URL)
	store, _ := core.Memories()
	RememberAs(mock, "bob", "bob's avatar has long silver hair", core.KindLook, nil)
	RememberAs(mock, "bob", "bob likes trains", core.KindLikes, nil)
	looks, err := recallKind(store, "recall-kind", "bob", core.KindLook)
	if err != nil || len(looks) != 1 || !strings.Contains(looks[0].Fact, "silver") {
		t.Errorf("looks = %+v %v", looks, err)
	}
	if all, _ := recallKind(store, "recall-kind", "bob", ""); len(all) != 2 {
		t.Errorf("all = %+v", all)
	}
}

func TestCheckKind(t *testing.T) {
	for _, c := range []struct{ kind, fact, want string }{
		{core.KindIdentity, "Bob's avatar now has a black beret.", core.KindLook},
		{core.KindLikes, "bob likes drawing his avatar in pastel colours", core.KindLook},
		{core.KindIdentity, "Eve prefers to be called 'Eve' and does not want to be called 'Evie'.", core.KindBoundary},
		{"", "dave doesn't want to be called davey", core.KindBoundary},
		{core.KindIdentity, "Eve and Carol went to a concert that got rained out.", ""},
		{core.KindIdentity, "eve's nickname is Ev", core.KindIdentity},
		{core.KindIdentity, "carol uses they/them", core.KindIdentity},
		{core.KindLikes, "bob doesn't like to use Windows", core.KindLikes},
		{core.KindLore, "bob once fell into the duck pond", core.KindLore},
	} {
		if got := CheckKind(c.kind, c.fact); got != c.want {
			t.Errorf("CheckKind(%q, %q) = %q, want %q", c.kind, c.fact, got, c.want)
		}
	}
}
