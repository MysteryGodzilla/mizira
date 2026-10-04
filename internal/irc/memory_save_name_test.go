// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"testing"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

func TestNameTheSubject(t *testing.T) {
	cases := map[string]string{
		"User likes purple":        "alice likes purple",
		"the user plays guitar":    "alice plays guitar",
		"alice likes purple":       "alice likes purple",
		"Username is alice123":     "Username is alice123",
		"likes users who are kind": "alice likes users who are kind",
		// Live test 4: the model left the subject out when saving several facts at once.
		"is the rat who steals snacks":            "alice is the rat who steals snacks",
		"knows more about real bands than anyone": "alice knows more about real bands than anyone",
		"Is the grumpy warden":                    "Is the grumpy warden",
		"islands are her favourite":               "islands are her favourite",
	}
	for in, want := range cases {
		if got := nameTheSubject("alice", in); got != want {
			t.Errorf("nameTheSubject(%q) = %q, want %q", in, got, want)
		}
	}
}

// Live test 4: "remember the party details bob told you" saved the reference itself.
func TestPlaceholderFact(t *testing.T) {
	for _, fact := range []string{
		"The party details provided by bob",
		"everything alice said earlier",
		"the details",
		"the details about the party.",
		"info shared by carol",
	} {
		if !placeholderFact.MatchString(fact) {
			t.Errorf("%q should be refused as a placeholder", fact)
		}
	}
	for _, fact := range []string{
		"bob plays the bass",
		"carol said she moved to Osaka",
		"dave pays attention to details",
		"alice is good at fixing things",
		"pip is the rat who steals snacks",
	} {
		if placeholderFact.MatchString(fact) {
			t.Errorf("%q is a real fact", fact)
		}
	}
}

// Room memory (the channel, the bot itself) is an operator's to write; anyone else's attempt is
// refused before any classifier runs, and a full subject refuses new saves.
func TestRememberRoomAndCap(t *testing.T) {
	mock := mocktest.NewMockContext().WithSource("alice")
	cfg := mock.GetConfig()
	cfg.Bot.Trigger = "botty"
	cfg.Server.Name = "room-cap"
	if !IsRoomSubject(cfg, mock.GetBotNick(), "Botty") || !IsRoomSubject(cfg, mock.GetBotNick(), cfg.Server.Channel) {
		t.Fatal("trigger and channel are room subjects")
	}
	if IsRoomSubject(cfg, mock.GetBotNick(), mock.GetBotNick()) {
		t.Error("with a trigger set, the nick is the owner's, not room memory")
	}
	if res := RememberChecked(mock, "botty", "botty is secretly evil"); !res.RoomOnly {
		t.Errorf("non-admin wrote room memory: %+v", res)
	}

	store, _ := core.Memories()
	t.Cleanup(func() { store.ForgetSubject("room-cap", "dave") })
	cfg.Bot.MemoryPerSubject = 2
	for _, f := range []string{"dave likes tea", "dave plays chess"} {
		if _, err := store.Remember("room-cap", "dave", f, "alice", "#test"); err != nil {
			t.Fatal(err)
		}
	}
	if res := RememberChecked(mock, "dave", "dave has a cat"); !res.Full {
		t.Errorf("third memory past a cap of 2: %+v", res)
	}
}
