// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alexschlessinger/pollytool/messages"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

// A fold on request keeps the last idleKeepTurns turns, like an idle fold.
func TestFoldNowKeepsRecentTurns(t *testing.T) {
	var calls atomic.Int32
	srv := summaryServer(t, "bob talks a lot", &calls, nil)
	cfg := mocktest.DefaultTestConfig()
	cfg.API.OpenAIURL = srv.URL

	s, _ := foldSession(t, "net/#foldnow")
	for i := 0; i < 5; i++ {
		s.AddMessage(um("(nick:bob) more"))
		s.AddMessage(am("ok"))
	}
	r, err := FoldNow(cfg, s)
	if err != nil {
		t.Fatal(err)
	}
	if r.Folded != 8 || r.Kept != 2*idleKeepTurns {
		t.Errorf("result = %+v, want 8 folded and %d kept", r, 2*idleKeepTurns)
	}
	if len(s.GetHistory()) != 1+2*idleKeepTurns {
		t.Errorf("history = %d messages after the fold", len(s.GetHistory()))
	}
}

func TestFoldNowShortConversation(t *testing.T) {
	var calls atomic.Int32
	srv := summaryServer(t, "recap", &calls, nil)
	cfg := mocktest.DefaultTestConfig()
	cfg.API.OpenAIURL = srv.URL

	s, _ := foldSession(t, "net/#foldshort")
	if _, err := FoldNow(cfg, s); !errors.Is(err, ErrNothingToFold) {
		t.Errorf("err = %v, want ErrNothingToFold", err)
	}
	if calls.Load() != 0 {
		t.Error("called the model with nothing to fold")
	}
}

// The summary waits behind whatever holds the model gate, so a fold never competes with a reply.
func TestFoldWaitsForModelGate(t *testing.T) {
	var calls atomic.Int32
	srv := summaryServer(t, "recap", &calls, nil)
	cfg := mocktest.DefaultTestConfig()
	cfg.API.OpenAIURL = srv.URL
	s, _ := foldSession(t, "net/#foldgate")

	held, release := make(chan struct{}), make(chan struct{})
	go core.WithModelGate(context.Background(), func() { close(held); <-release })
	<-held
	done := make(chan error, 1)
	go func() {
		_, err := fold(cfg, s, "test", func(c []messages.ChatMessage) int { return cutKeepTurns(c, 2) })
		done <- err
	}()
	time.Sleep(200 * time.Millisecond)
	if calls.Load() != 0 {
		t.Fatal("the fold called the model while the gate was held")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Errorf("model calls = %d after the gate opened, want 1", calls.Load())
	}
}
