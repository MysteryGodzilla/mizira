// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package admin

import (
	"net/http"
	"strings"
	"testing"
)

type fakeMizira struct {
	state string
	tools []string
	think bool
	by    []string
}

func (f *fakeMizira) RunState() string { return f.state }
func (f *fakeMizira) SetRunState(state, by string) (string, bool, int) {
	prev := f.state
	if (state == "paused" && prev != "running") || (state == "running" && prev == "running") {
		return prev, false, 0
	}
	f.state, f.by = state, append(f.by, by)
	if state == "stopped" {
		return prev, true, 2
	}
	return prev, true, 0
}
func (f *fakeMizira) Tools() []string { return f.tools }
func (f *fakeMizira) Thinking() bool  { return f.think }

func newMzRig(t *testing.T) (*rig, *fakeMizira) {
	t.Helper()
	m := &fakeMizira{state: "running", tools: []string{"irc__remind", "irc__slap"}}
	r := newRig(t)
	r.srv.WithMizira(m)
	r.ts.Config.Handler = r.srv.Handler()
	return r, m
}

// callFrom is call as if through the reverse proxy, for the client named in X-Forwarded-For.
func (r *rig) callFrom(t *testing.T, client, method, path, token string) int {
	t.Helper()
	req, _ := http.NewRequest(method, r.ts.URL+path, nil)
	req.Header.Set("X-Forwarded-For", client)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestCheckListen(t *testing.T) {
	for addr, ok := range map[string]bool{
		"127.0.0.1:8425": true, "localhost:8425": true, "[::1]:8425": true, "198.51.100.7:8425": true,
		"0.0.0.0:8425": false, ":8425": false, "[::]:8425": false, "8425": false,
	} {
		if err := CheckListen(addr); (err == nil) != ok {
			t.Errorf("CheckListen(%q) = %v, want ok=%v", addr, err, ok)
		}
	}
}

func TestRunRefusesEveryInterface(t *testing.T) {
	r := newRig(t)
	if err := r.srv.Run(t.Context(), ":0"); err == nil || !strings.Contains(err.Error(), "every interface") {
		t.Fatalf("Run(:0) = %v", err)
	}
}

func TestMiziraEndpointsNeedTheToken(t *testing.T) {
	r, m := newMzRig(t)
	for _, c := range [][2]string{{"GET", "/api/v1/features"}, {"GET", "/api/v1/mizira/state"}, {"PUT", "/api/v1/mizira/state"}} {
		if code, _ := r.call(t, c[0], c[1], "wrong", `{"state":"stopped"}`); code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", c[0], c[1], code)
		}
	}
	if m.state != "running" {
		t.Fatal("a refused call changed the run state")
	}
}

func TestWithoutMiziraTheEndpointsAreAbsent(t *testing.T) {
	r := newRig(t)
	if code, _ := r.call(t, "GET", "/api/v1/features", "s3cret", ""); code != http.StatusNotFound {
		t.Errorf("features on upstream's page = %d, want 404", code)
	}
}

func TestFeatures(t *testing.T) {
	r, m := newMzRig(t)
	m.think = true
	_, out := r.call(t, "GET", "/api/v1/features", "s3cret", "")
	got := map[string]bool{}
	for _, f := range out["features"].([]any) {
		f := f.(map[string]any)
		got[f["id"].(string)] = f["active"].(bool)
	}
	want := map[string]bool{"work": false, "reminders": true, "gpu": false, "radio": false, "thinking": true}
	if asJSON(got) != asJSON(want) {
		t.Errorf("features = %v, want %v", got, want)
	}
}

func TestPauseStopResumeFromTheConsole(t *testing.T) {
	r, m := newMzRig(t)
	put := func(state string) (int, map[string]any) {
		return r.call(t, "PUT", "/api/v1/mizira/state", "s3cret", `{"state":"`+state+`"}`)
	}
	if code, out := put("paused"); code != http.StatusOK || out["was"] != "running" {
		t.Fatalf("pause = %d %v", code, out)
	}
	if code, _ := put("paused"); code != http.StatusConflict {
		t.Errorf("pausing twice = %d, want 409", code)
	}
	if code, out := put("stopped"); code != http.StatusOK || out["cancelled"] != float64(2) {
		t.Errorf("stop = %d %v", code, out)
	}
	if code, _ := put("paused"); code != http.StatusConflict || m.state != "stopped" {
		t.Errorf("pause after stop = %d, state %s; pausing must not weaken a stop", code, m.state)
	}
	if code, _ := put("running"); code != http.StatusOK {
		t.Errorf("resume = %d", code)
	}
	if code, _ := put("sideways"); code != http.StatusBadRequest {
		t.Errorf("unknown state = %d, want 400", code)
	}
	if _, out := r.call(t, "GET", "/api/v1/mizira/state", "s3cret", ""); out["state"] != "running" {
		t.Errorf("state = %v", out)
	}
	if len(m.by) == 0 || m.by[0] != "console:token" {
		t.Errorf("changes recorded as by %v", m.by)
	}
}

func TestWrongTokensLockOutTheClient(t *testing.T) {
	r, _ := newMzRig(t)
	for i := range authFailures {
		if code := r.callFrom(t, "198.51.100.9", "GET", "/api/v1/status", "guess"); code != http.StatusUnauthorized {
			t.Fatalf("wrong token %d = %d, want 401", i, code)
		}
	}
	if code := r.callFrom(t, "198.51.100.9", "GET", "/api/v1/status", "s3cret"); code != http.StatusTooManyRequests {
		t.Errorf("right token from a locked-out client = %d, want 429", code)
	}
	if code := r.callFrom(t, "198.51.100.10", "GET", "/api/v1/status", "s3cret"); code != http.StatusOK {
		t.Errorf("another client = %d, want 200", code)
	}
	// From this machine with no proxy in between: whoever it is can read config.yml anyway.
	if code, _ := r.call(t, "GET", "/api/v1/status", "s3cret", ""); code != http.StatusOK {
		t.Errorf("local call = %d, want 200", code)
	}
}

func TestNoTokenIsNotAFailure(t *testing.T) {
	r, _ := newMzRig(t)
	for range authFailures * 2 {
		r.callFrom(t, "198.51.100.9", "GET", "/api/v1/whoami", "")
	}
	if code := r.callFrom(t, "198.51.100.9", "GET", "/api/v1/status", "s3cret"); code != http.StatusOK {
		t.Errorf("after proxy checks without a token = %d, want 200", code)
	}
}

func TestARightTokenClearsEarlierMistakes(t *testing.T) {
	r, _ := newMzRig(t)
	for range 3 {
		for range authFailures - 1 {
			r.callFrom(t, "198.51.100.9", "GET", "/api/v1/status", "typo")
		}
		if code := r.callFrom(t, "198.51.100.9", "GET", "/api/v1/status", "s3cret"); code != http.StatusOK {
			t.Fatalf("right token after %d typos = %d", authFailures-1, code)
		}
	}
}
