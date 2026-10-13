// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"fmt"
	"slices"
	"testing"
)

func TestProfileHints(t *testing.T) {
	mem := func(id int64, subject, fact string) Memory { return Memory{ID: id, Subject: subject, Fact: fact} }
	hints := func(subject string, p Profile, mems ...Memory) []string {
		var out []string
		for _, h := range ProfileHints(subject, mems, p) {
			out = append(out, fmt.Sprintf("%s=%s#%d", h.Field, h.Value, h.MemoryID))
		}
		return out
	}
	for _, c := range []struct {
		name    string
		subject string
		p       Profile
		mems    []Memory
		want    []string
	}{
		{"stated pronouns", "carol", Profile{}, []Memory{mem(1, "carol", "carol uses they/them")}, []string{"pronouns=they/them#1"}},
		{"girl", "alice", Profile{}, []Memory{mem(2, "alice", "Alice is a girl")}, []string{"pronouns=she/her#2"}},
		{"avatar", "bob", Profile{Pronouns: "he/him"}, []Memory{mem(3, "bob", "bob's avatar is a girl with silver hair")},
			[]string{"avatarPronouns=she/her#3"}},
		{"nickname", "alice", Profile{}, []Memory{mem(4, "alice", "Alice's nickname is Ally.")}, []string{"alias=ally#4"}},
		{"nickname quoted", "botty", Profile{}, []Memory{mem(5, "botty", "dave calls botty by the nickname 'bot'")}, []string{"alias=bot#5"}},
		{"bot of", "botty", Profile{}, []Memory{mem(6, "botty", "botty is dave's bot")}, []string{"runBy=dave#6"}},
		{"already held", "carol", Profile{Pronouns: "they/them", Aliases: []string{"cee"}},
			[]Memory{mem(7, "carol", "carol uses they/them"), mem(8, "carol", "carol goes by Cee")}, nil},
		{"someone else's memory", "carol", Profile{}, []Memory{mem(9, "dave", "dave uses he/him and knows carol")}, nil},
		{"not a name", "dave", Profile{}, []Memory{mem(10, "dave", "dave goes by the bus")}, nil},
		{"portrayed as", "dave", Profile{}, []Memory{mem(12, "dave", "dave is portrayed as a girl with blue hair")},
			[]string{"avatarPronouns=she/her#12"}},
		{"what not to call them", "botty", Profile{}, []Memory{mem(13, "botty", "botty gets grumpy when people call her \"baka\"")}, nil},
		{"avatar same as person", "alice", Profile{Pronouns: "she/her"}, []Memory{mem(11, "alice", "alice's avatar is a girl")}, nil},
	} {
		if got := hints(c.subject, c.p, c.mems...); !slices.Equal(got, c.want) {
			t.Errorf("%s: hints = %v, want %v", c.name, got, c.want)
		}
	}
}
