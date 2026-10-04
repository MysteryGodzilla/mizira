// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"strings"
	"testing"
	"time"
)

func TestCleanSlapObject(t *testing.T) {
	cases := map[string]string{
		"":                          slapDefault,
		"   ":                       slapDefault,
		"his GPU":                   "his GPU",
		"With his keyboard":         "his keyboard",
		"a \x02bold\x02\nnoodle":    "a bold noodle",
		"http://example.com/x.png":  slapDefault,
		"a link to www.example.com": slapDefault,
	}
	for in, want := range cases {
		if got := cleanSlapObject(in); got != want {
			t.Errorf("cleanSlapObject(%q) = %q, want %q", in, got, want)
		}
	}
	if got := cleanSlapObject(strings.Repeat("trout ", 30)); len([]rune(got)) > maxSlapObject {
		t.Errorf("object not capped: %d runes", len([]rune(got)))
	}
}

func TestSlapCooldown(t *testing.T) {
	now := time.Now()
	key := "test-net/bob-" + t.Name()
	if w := slapWait(key, now); w != 0 {
		t.Fatalf("first slap waited %v", w)
	}
	if w := slapWait(key, now.Add(30*time.Second)); w <= 0 {
		t.Fatal("second slap inside the cooldown was allowed")
	}
	if w := slapWait(key, now.Add(slapCooldown+time.Second)); w != 0 {
		t.Fatalf("slap after the cooldown waited %v", w)
	}
}

// Live test 4: asked to choose, the model sent the request back as the object.
func TestEchoedSlapObject(t *testing.T) {
	for _, obj := range []string{"slap mallory with something", "something", "whatever you want", "mallory"} {
		if !echoedSlapObject(obj, "mallory") {
			t.Errorf("%q should be replaced", obj)
		}
	}
	for _, obj := range []string{"a wet noodle", "his GPU", "a large trout"} {
		if echoedSlapObject(obj, "mallory") {
			t.Errorf("%q should be kept", obj)
		}
	}
}
