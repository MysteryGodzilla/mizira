// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package admin

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

type fakeMizira struct {
	state string
	tools []string
	think bool
	by    []string

	persona  string
	recap    bool
	ignores  []IgnoreView
	screened []string
	scores   []ScoreView
	botNicks []string
	setting  string
	mems     []MemoryView
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

func (f *fakeMizira) Conversations() []ConversationView {
	return []ConversationView{{Network: "net", Channel: "#chat", Messages: 4, Persona: f.persona,
		Recent: []LineView{{Role: "user", Text: "(nick:bob) <b>hi</b>"}}}}
}
func (f *fakeMizira) ResetConversation(network, by string) (ResetView, error) {
	f.by = append(f.by, by)
	cleared := f.persona != ""
	f.persona = ""
	return ResetView{PersonaCleared: cleared}, nil
}
func (f *fakeMizira) ClearRecap(network, by string) (bool, error) {
	had := f.recap
	f.recap = false
	return had, nil
}
func (f *fakeMizira) Ignores() []IgnoreView { return f.ignores }
func (f *fakeMizira) Ignore(network, nick string, d time.Duration, reason, by string) (time.Time, error) {
	if nick == "alice" {
		return time.Time{}, errors.New("alice is an admin; admins are exempt from ignore")
	}
	until := time.Now().Add(d)
	f.ignores = append(f.ignores, IgnoreView{Network: network, Nick: nick, Until: until.Unix(), Kind: "admin", By: by, Reason: reason})
	return until, nil
}
func (f *fakeMizira) Unignore(network, nick, by string) bool {
	for i, e := range f.ignores {
		if e.Network == network && e.Nick == nick {
			f.ignores = slices.Delete(f.ignores, i, i+1)
			return true
		}
	}
	return false
}
func (f *fakeMizira) Screened() ScreenView { return ScreenView{In: f.screened, Out: f.screened} }
func (f *fakeMizira) Screen(network, nick, by string) (int, error) {
	f.screened = append(f.screened, nick)
	return 2, nil
}
func (f *fakeMizira) Unscreen(nick, by string) bool {
	n := len(f.screened)
	f.screened = slices.DeleteFunc(f.screened, func(s string) bool { return s == nick })
	return len(f.screened) < n
}
func (f *fakeMizira) Suspicion() ([]ScoreView, float64) { return f.scores, 3 }
func (f *fakeMizira) ClearSuspicion(network, key, by string) bool {
	n := len(f.scores)
	f.scores = slices.DeleteFunc(f.scores, func(s ScoreView) bool { return s.Network == network && s.Key == key })
	return len(f.scores) < n
}

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
	for _, c := range [][2]string{{"GET", "/api/v1/features"}, {"GET", "/api/v1/mizira/state"}, {"PUT", "/api/v1/mizira/state"},
		{"GET", "/api/v1/conversation"}, {"POST", "/api/v1/conversation/reset"}, {"DELETE", "/api/v1/recap?network=net"},
		{"GET", "/api/v1/people"}, {"POST", "/api/v1/ignores"}, {"DELETE", "/api/v1/ignores?network=net&nick=bob"},
		{"POST", "/api/v1/screens"}, {"DELETE", "/api/v1/screens?nick=bob"}, {"DELETE", "/api/v1/suspicion?network=net&key=bob"},
		{"GET", "/api/v1/bots"}, {"POST", "/api/v1/bots"}, {"DELETE", "/api/v1/bots?kind=nick&value=bob"},
		{"GET", "/api/v1/settings"}, {"PUT", "/api/v1/settings/maxreplylines"}, {"DELETE", "/api/v1/settings/maxreplylines"},
		{"GET", "/api/v1/tools"}, {"PUT", "/api/v1/tools"}, {"DELETE", "/api/v1/tools/switches"},
		{"GET", "/api/v1/memories/subjects?network=net"}, {"GET", "/api/v1/memories?network=net"}, {"POST", "/api/v1/memories"},
		{"PUT", "/api/v1/memories/1?network=net"}, {"DELETE", "/api/v1/memories/1?network=net"}} {
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

func TestResetAndRecapFromTheConsole(t *testing.T) {
	r, m := newMzRig(t)
	m.persona, m.recap = "a pirate", true
	_, out := r.call(t, "GET", "/api/v1/conversation", "s3cret", "")
	conv := out["conversations"].([]any)[0].(map[string]any)
	if conv["persona"] != "a pirate" || conv["channel"] != "#chat" {
		t.Errorf("conversation = %v", conv)
	}
	if code, out := r.call(t, "POST", "/api/v1/conversation/reset", "s3cret", `{"network":"net"}`); code != http.StatusOK || out["personaCleared"] != true {
		t.Errorf("reset = %d %v", code, out)
	}
	if code, _ := r.call(t, "POST", "/api/v1/conversation/reset", "s3cret", `{"network":"elsewhere"}`); code != http.StatusNotFound {
		t.Errorf("reset on an unknown network = %d", code)
	}
	if code, _ := r.call(t, "DELETE", "/api/v1/recap?network=net", "s3cret", ""); code != http.StatusOK {
		t.Errorf("clear recap = %d", code)
	}
	if code, _ := r.call(t, "DELETE", "/api/v1/recap?network=net", "s3cret", ""); code != http.StatusConflict {
		t.Errorf("clear an empty recap = %d, want 409", code)
	}
}

func TestIgnoresFromTheConsole(t *testing.T) {
	r, m := newMzRig(t)
	if code, out := r.call(t, "POST", "/api/v1/ignores", "s3cret", `{"network":"net","nick":"mallory","minutes":90,"reason":"spam"}`); code != http.StatusOK || out["until"] == nil {
		t.Fatalf("ignore = %d %v", code, out)
	}
	if len(m.ignores) != 1 || m.ignores[0].By != "console:token" || m.ignores[0].Reason != "spam" {
		t.Errorf("ignores = %+v", m.ignores)
	}
	for _, bad := range []string{
		`{"network":"net","nick":"mallory","minutes":0}`, `{"network":"net","nick":"mallory","minutes":43201}`,
		`{"network":"net","nick":"two words","minutes":5}`, `{"network":"net","nick":"*!*@*","minutes":5}`,
		`{"network":"net","nick":"","minutes":5}`, `not json`,
	} {
		if code, _ := r.call(t, "POST", "/api/v1/ignores", "s3cret", bad); code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", bad, code)
		}
	}
	if code, out := r.call(t, "POST", "/api/v1/ignores", "s3cret", `{"network":"net","nick":"alice","minutes":5}`); code != http.StatusConflict ||
		!strings.Contains(out["error"].(string), "admin") {
		t.Errorf("ignoring an admin = %d %v", code, out)
	}
	if code, _ := r.call(t, "DELETE", "/api/v1/ignores?network=net&nick=mallory", "s3cret", ""); code != http.StatusOK || len(m.ignores) != 0 {
		t.Errorf("unignore = %d", code)
	}
	if code, _ := r.call(t, "DELETE", "/api/v1/ignores?network=net&nick=mallory", "s3cret", ""); code != http.StatusNotFound {
		t.Errorf("unignore twice = %d, want 404", code)
	}
}

func TestScreensAndSuspicionFromTheConsole(t *testing.T) {
	r, m := newMzRig(t)
	m.scores = []ScoreView{{Network: "net", Key: "eve", Score: 2.5}}
	if code, out := r.call(t, "POST", "/api/v1/screens", "s3cret", `{"network":"net","nick":"eve"}`); code != http.StatusOK || out["dropped"] != float64(2) {
		t.Fatalf("screen = %d %v", code, out)
	}
	_, out := r.call(t, "GET", "/api/v1/people", "s3cret", "")
	if asJSON(out["screened"]) != `{"in":["eve"],"out":["eve"]}` || out["quarantineAt"] != float64(3) || len(out["suspicion"].([]any)) != 1 {
		t.Errorf("people = %v", out)
	}
	if code, _ := r.call(t, "DELETE", "/api/v1/screens?nick=eve", "s3cret", ""); code != http.StatusOK {
		t.Errorf("unscreen = %d", code)
	}
	if code, _ := r.call(t, "DELETE", "/api/v1/suspicion?network=net&key=eve", "s3cret", ""); code != http.StatusOK || len(m.scores) != 0 {
		t.Errorf("clear suspicion = %d", code)
	}
	if code, _ := r.call(t, "DELETE", "/api/v1/suspicion?network=net&key=eve", "s3cret", ""); code != http.StatusNotFound {
		t.Errorf("clear a missing score = %d, want 404", code)
	}
}

func (f *fakeMizira) Bots() BotsView {
	return BotsView{Nicks: f.botNicks, Prefixes: []string{}, ReplyLimit: "3", Cooldown: "10m0s"}
}
func (f *fakeMizira) ChangeBots(add bool, kind, value, by string) (string, error) {
	if add {
		if slices.Contains(f.botNicks, value) {
			return value, errors.New(value + " is already a bot nick")
		}
		f.botNicks = append(f.botNicks, value)
		return value, nil
	}
	n := len(f.botNicks)
	f.botNicks = slices.DeleteFunc(f.botNicks, func(s string) bool { return s == value })
	if len(f.botNicks) == n {
		return value, errors.New(value + " wasn't a bot nick")
	}
	return value, nil
}
func (f *fakeMizira) Settings() []SettingView {
	return []SettingView{{Key: "maxreplylines", Value: f.setting, Default: "4", Overridden: f.setting != "4", Editable: true}}
}
func (f *fakeMizira) SetSetting(key, value, by string) (SettingChange, error) {
	if key != "maxreplylines" {
		return SettingChange{}, errors.New("no such setting")
	}
	if value == "99" {
		return SettingChange{}, errors.New("invalid value for maxreplylines")
	}
	f.setting = value
	return SettingChange{Key: key, Value: value}, nil
}
func (f *fakeMizira) ResetSetting(key, by string) (SettingChange, error) {
	f.setting = "4"
	return SettingChange{Key: key, Value: "4"}, nil
}
func (f *fakeMizira) ToolCatalog() []ToolView {
	return []ToolView{{Spec: "irc__slap", Kind: "native", Loaded: slices.Contains(f.tools, "irc__slap"), Names: []string{}}}
}
func (f *fakeMizira) SwitchTool(spec string, on bool, by string) error {
	if spec != "irc__slap" {
		return errors.New("not a tool this console offers")
	}
	f.tools = slices.DeleteFunc(f.tools, func(s string) bool { return s == spec })
	if on {
		f.tools = append(f.tools, spec)
	}
	return nil
}
func (f *fakeMizira) ResetToolSwitches(by string) error { return nil }

func TestBotsFromTheConsole(t *testing.T) {
	r, m := newMzRig(t)
	if code, _ := r.call(t, "POST", "/api/v1/bots", "s3cret", `{"kind":"nick","value":"carol"}`); code != http.StatusOK || !slices.Contains(m.botNicks, "carol") {
		t.Fatalf("add bot = %d", code)
	}
	if code, _ := r.call(t, "POST", "/api/v1/bots", "s3cret", `{"kind":"nick","value":"carol"}`); code != http.StatusConflict {
		t.Errorf("add twice = %d, want 409", code)
	}
	for _, bad := range []string{`{"kind":"host","value":"carol"}`, `{"kind":"nick","value":" "}`} {
		if code, _ := r.call(t, "POST", "/api/v1/bots", "s3cret", bad); code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", bad, code)
		}
	}
	if code, _ := r.call(t, "DELETE", "/api/v1/bots?kind=nick&value=carol", "s3cret", ""); code != http.StatusOK || len(m.botNicks) != 0 {
		t.Errorf("remove bot = %d", code)
	}
}

func TestSettingsFromTheConsole(t *testing.T) {
	r, m := newMzRig(t)
	m.setting = "4"
	if code, out := r.call(t, "PUT", "/api/v1/settings/maxreplylines", "s3cret", `{"value":"6"}`); code != http.StatusOK || out["value"] != "6" {
		t.Fatalf("set = %d %v", code, out)
	}
	_, out := r.call(t, "GET", "/api/v1/settings", "s3cret", "")
	if s := out["settings"].([]any)[0].(map[string]any); s["overridden"] != true || s["default"] != "4" {
		t.Errorf("settings = %v", s)
	}
	if code, _ := r.call(t, "PUT", "/api/v1/settings/maxreplylines", "s3cret", `{"value":"99"}`); code != http.StatusBadRequest || m.setting != "6" {
		t.Errorf("invalid value = %d, setting %s", code, m.setting)
	}
	if code, _ := r.call(t, "PUT", "/api/v1/settings/openaikey", "s3cret", `{"value":"x"}`); code != http.StatusBadRequest {
		t.Errorf("a credential = %d, want 400", code)
	}
	if code, _ := r.call(t, "PUT", "/api/v1/settings/maxreplylines", "s3cret", `{}`); code != http.StatusBadRequest {
		t.Errorf("no value = %d, want 400", code)
	}
	if code, out := r.call(t, "DELETE", "/api/v1/settings/maxreplylines", "s3cret", ""); code != http.StatusOK || out["value"] != "4" {
		t.Errorf("reset = %d %v", code, out)
	}
}

func TestToolSwitchesFromTheConsole(t *testing.T) {
	r, m := newMzRig(t)
	if code, _ := r.call(t, "PUT", "/api/v1/tools", "s3cret", `{"spec":"irc__slap","on":false}`); code != http.StatusOK || slices.Contains(m.tools, "irc__slap") {
		t.Fatalf("switch off = %d %v", code, m.tools)
	}
	if code, _ := r.call(t, "PUT", "/api/v1/tools", "s3cret", `{"spec":"/usr/bin/anything","on":true}`); code != http.StatusConflict {
		t.Errorf("a spec the console doesn't offer = %d, want 409", code)
	}
	if code, _ := r.call(t, "PUT", "/api/v1/tools", "s3cret", `{"spec":"irc__slap"}`); code != http.StatusBadRequest {
		t.Errorf("no on/off = %d, want 400", code)
	}
	if code, _ := r.call(t, "DELETE", "/api/v1/tools/switches", "s3cret", ""); code != http.StatusOK {
		t.Errorf("reset switches = %d", code)
	}
}

func (f *fakeMizira) MemorySubjects(network string) ([]SubjectView, int, error) {
	counts := map[string]int{}
	for _, m := range f.mems {
		counts[m.Subject]++
	}
	out := []SubjectView{}
	for s, n := range counts {
		out = append(out, SubjectView{Subject: s, Count: n, Room: s == "botty"})
	}
	return out, 40, nil
}
func (f *fakeMizira) Memories(network, subject, query string) ([]MemoryView, error) {
	out := []MemoryView{}
	for _, m := range f.mems {
		if (subject == "" || m.Subject == subject) && (query == "" || strings.Contains(m.Fact, query)) {
			out = append(out, m)
		}
	}
	return out, nil
}
func (f *fakeMizira) AddMemory(network, subject, fact, by string) (int64, bool, error) {
	if subject == "" || fact == "" {
		return 0, false, errors.New("give a subject and a fact")
	}
	id := int64(len(f.mems) + 1)
	f.mems = append(f.mems, MemoryView{ID: id, Subject: subject, Fact: fact, Author: by})
	return id, false, nil
}
func (f *fakeMizira) EditMemory(network string, id int64, fact, by string) error {
	for i := range f.mems {
		if f.mems[i].ID == id {
			f.mems[i].Fact = fact
			return nil
		}
	}
	return ErrNoSuchMemory
}
func (f *fakeMizira) ForgetMemory(network string, id int64, by string) error {
	n := len(f.mems)
	f.mems = slices.DeleteFunc(f.mems, func(m MemoryView) bool { return m.ID == id })
	if len(f.mems) == n {
		return ErrNoSuchMemory
	}
	return nil
}

func TestMemoriesFromTheConsole(t *testing.T) {
	r, m := newMzRig(t)
	if code, out := r.call(t, "POST", "/api/v1/memories", "s3cret", `{"network":"net","subject":"bob","fact":"bob plays chess"}`); code != http.StatusOK || out["id"] != float64(1) {
		t.Fatalf("add = %d %v", code, out)
	}
	if m.mems[0].Author != "console:token" {
		t.Errorf("author = %q", m.mems[0].Author)
	}
	if code, _ := r.call(t, "POST", "/api/v1/memories", "s3cret", `{"network":"net","subject":"","fact":"x"}`); code != http.StatusConflict {
		t.Errorf("no subject = %d, want 409", code)
	}
	_, out := r.call(t, "GET", "/api/v1/memories/subjects?network=net", "s3cret", "")
	if out["perSubject"] != float64(40) || len(out["subjects"].([]any)) != 1 {
		t.Errorf("subjects = %v", out)
	}
	if _, out := r.call(t, "GET", "/api/v1/memories?network=net&q=chess", "s3cret", ""); len(out["memories"].([]any)) != 1 {
		t.Errorf("search = %v", out)
	}
	if code, _ := r.call(t, "PUT", "/api/v1/memories/1?network=net", "s3cret", `{"fact":"bob plays chess on Sundays"}`); code != http.StatusOK || m.mems[0].Fact != "bob plays chess on Sundays" {
		t.Errorf("edit = %d", code)
	}
	if code, _ := r.call(t, "PUT", "/api/v1/memories/x?network=net", "s3cret", `{"fact":"y"}`); code != http.StatusBadRequest {
		t.Errorf("bad id = %d, want 400", code)
	}
	if code, _ := r.call(t, "DELETE", "/api/v1/memories/1?network=elsewhere", "s3cret", ""); code != http.StatusNotFound || len(m.mems) != 1 {
		t.Errorf("forget on another network = %d", code)
	}
	if code, _ := r.call(t, "DELETE", "/api/v1/memories/1?network=net", "s3cret", ""); code != http.StatusOK || len(m.mems) != 0 {
		t.Errorf("forget = %d", code)
	}
	if code, _ := r.call(t, "DELETE", "/api/v1/memories/1?network=net", "s3cret", ""); code != http.StatusNotFound {
		t.Errorf("forget twice = %d, want 404", code)
	}
}
