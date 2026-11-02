// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package admin

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"B4reMetal/metald/internal/core"
)

func newBridgeRig(t *testing.T) (*rig, *fakeMizira) {
	t.Helper()
	r, m := newMzRig(t)
	r.srv.cfg.StatusToken = "status-key"
	r.ts.Config.Handler = r.srv.Handler()
	return r, m
}

func TestBridgeTakesTheStatusTokenAndTheConsoleToken(t *testing.T) {
	r, _ := newBridgeRig(t)
	for _, tok := range []string{"status-key", "s3cret"} {
		if code, _ := r.call(t, "GET", "/api/v1/bridge", tok, ""); code != http.StatusOK {
			t.Errorf("token %q: status %d, want 200", tok, code)
		}
	}
	for _, tok := range []string{"", "wrong"} {
		if code, _ := r.call(t, "GET", "/api/v1/bridge", tok, ""); code != http.StatusUnauthorized {
			t.Errorf("token %q: status %d, want 401", tok, code)
		}
	}
}

func TestStatusTokenOpensNothingElse(t *testing.T) {
	r, _ := newBridgeRig(t)
	for _, path := range []string{"/api/v1/status", "/api/v1/memories?network=net", "/api/v1/people", "/api/v1/tasks"} {
		if code, _ := r.call(t, "GET", path, "status-key", ""); code != http.StatusUnauthorized {
			t.Errorf("%s with the status token: %d, want 401", path, code)
		}
	}
	if code, _ := r.call(t, "PUT", "/api/v1/mizira/state", "status-key", `{"state":"stopped"}`); code != http.StatusUnauthorized {
		t.Errorf("changing her state with the status token: %d, want 401", code)
	}
}

func TestBridgeWithoutStatusTokenTakesOnlyTheConsoleToken(t *testing.T) {
	r, _ := newMzRig(t)
	if code, _ := r.call(t, "GET", "/api/v1/bridge", "", ""); code != http.StatusUnauthorized {
		t.Errorf("no token: %d, want 401", code)
	}
	if code, _ := r.call(t, "GET", "/api/v1/bridge", "s3cret", ""); code != http.StatusOK {
		t.Errorf("console token: %d, want 200", code)
	}
}

func TestBridgeSaysWhatSheIsDoing(t *testing.T) {
	r, m := newBridgeRig(t)
	last := time.Now().Add(-time.Minute)
	r.work.last = last
	r.work.tools = []core.ActiveTool{{Name: "imagegen__picture", Since: time.Now()}}

	_, out := r.call(t, "GET", "/api/v1/bridge", "status-key", "")
	if out["state"] != "working" || out["irc"] != true || out["tasks"] != float64(1) {
		t.Fatalf("bridge = %v, want working, on IRC, one task", out)
	}
	doing, _ := out["doing"].([]any)
	if len(doing) != 1 || doing[0].(map[string]any)["what"] != "drawing a picture" {
		t.Fatalf("doing = %v, want only the picture: a running tool replaces \"chatting\"", doing)
	}
	if out["last_active"] != float64(last.Unix()) {
		t.Errorf("last_active = %v, want %d", out["last_active"], last.Unix())
	}

	r.work.tools = nil
	_, out = r.call(t, "GET", "/api/v1/bridge", "status-key", "")
	doing, _ = out["doing"].([]any)
	if len(doing) != 1 || doing[0].(map[string]any)["what"] != "chatting" {
		t.Fatalf("doing = %v, want chatting for a running request with no tool", doing)
	}

	m.state, m.offline = "paused", []string{"console only"}
	_, out = r.call(t, "GET", "/api/v1/bridge", "status-key", "")
	if out["state"] != "paused" || out["irc"] != false {
		t.Errorf("bridge = %v, want paused and off IRC", out)
	}
}

func TestBridgeIsIdleWithNothingRunning(t *testing.T) {
	r, _ := newBridgeRig(t)
	r.srv.work = idleWork{}
	r.ts.Config.Handler = r.srv.Handler()
	_, out := r.call(t, "GET", "/api/v1/bridge", "status-key", "")
	if out["state"] != "idle" || out["last_active"] != float64(0) {
		t.Errorf("bridge = %v, want idle, never active", out)
	}
}

func TestBridgeShowsNoChatDetails(t *testing.T) {
	r, _ := newBridgeRig(t)
	req, _ := http.NewRequest("GET", r.ts.URL+"/api/v1/bridge", nil)
	req.Header.Set("Authorization", "Bearer status-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	// The rig's in-flight request is alice's in net/#chat, and its task is "write a haiku".
	for _, secret := range []string{"alice", "#chat", "haiku"} {
		if strings.Contains(string(b), secret) {
			t.Errorf("bridge output mentions %q: %s", secret, b)
		}
	}
}

func TestToolLabel(t *testing.T) {
	for name, want := range map[string]string{
		"imagegen__picture": "drawing a picture", "radio__station": "on the radio", "weather__now": "weather",
	} {
		if got := toolLabel(name); got != want {
			t.Errorf("toolLabel(%q) = %q, want %q", name, got, want)
		}
	}
}

type idleWork struct{}

func (idleWork) Inflight() []core.Inflight { return nil }
func (idleWork) Concurrency() int          { return 3 }
func (idleWork) StopRound(int64)           {}
func (idleWork) Tools() []core.ActiveTool  { return nil }
func (idleWork) LastActive() time.Time     { return time.Time{} }
