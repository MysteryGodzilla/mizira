// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package keeper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// The test binary doubles as the children: KEEPER_FAKE=bot or model makes it act as one.
func TestMain(m *testing.M) {
	switch os.Getenv("KEEPER_FAKE") {
	case "bot":
		fakeBot()
	case "model":
		fakeModel()
	}
	os.Exit(m.Run())
}

// fakeBot answers the status bridge on KEEPER_ADDR, and leaves when stdin closes (writing
// KEEPER_MARK), unless KEEPER_STUBBORN; KEEPER_CRASH exits with code 3 shortly after starting.
func fakeBot() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/bridge", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer status-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"state":"idle"}`))
	})
	go func() { _ = http.ListenAndServe(os.Getenv("KEEPER_ADDR"), mux) }()
	if os.Getenv("KEEPER_CRASH") == "1" {
		time.Sleep(500 * time.Millisecond)
		os.Exit(3)
	}
	if os.Getenv("KEEPER_STUBBORN") == "1" {
		select {}
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	_ = os.WriteFile(os.Getenv("KEEPER_MARK"), []byte("clean"), 0o600)
	os.Exit(0)
}

// fakeModel is llama-swap: m1 is listed as ready once /upstream/m1/health asked for it.
func fakeModel() {
	listen := ""
	for i, a := range os.Args {
		if a == "-listen" && i+1 < len(os.Args) {
			listen = os.Args[i+1]
		}
	}
	var loaded atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/running", func(w http.ResponseWriter, _ *http.Request) {
		if loaded.Load() {
			_, _ = w.Write([]byte(`{"running":[{"model":"m1","state":"ready"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"running":[]}`))
	})
	mux.HandleFunc("/upstream/m1/health", func(http.ResponseWriter, *http.Request) { loaded.Store(true) })
	mux.HandleFunc("/unload", func(http.ResponseWriter, *http.Request) {
		_ = os.WriteFile(os.Getenv("KEEPER_UNLOADED"), []byte("yes"), 0o600)
	})
	_ = http.ListenAndServe(listen, mux)
	os.Exit(1)
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

type rig struct {
	k        *Keeper
	dir      string
	botAddr  string
	mark     string
	unloaded string
}

// newRig sets up a keeper over fake children. env is extra for the fake bot.
func newRig(t *testing.T, withModel bool, env ...string) *rig {
	t.Helper()
	dir := t.TempDir()
	r := &rig{dir: dir, botAddr: freeAddr(t), mark: filepath.Join(dir, "mark"), unloaded: filepath.Join(dir, "unloaded")}
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatal(err)
	}
	botCfg := filepath.Join(dir, "bot.yml")
	body := fmt.Sprintf("model: openai/m1\ndatadir: %s\nadminlisten: %s\nstatustoken: status-key\n",
		filepath.ToSlash(data), r.botAddr)
	if err := os.WriteFile(botCfg, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	cfg := &Config{Listen: "127.0.0.1:1", ControlToken: strings.Repeat("c", 32),
		Bot: Bot{Exe: exe, Config: botCfg, Dir: dir, Log: filepath.Join(dir, "bot.log"), StopTimeout: 3 * time.Second}}
	if withModel {
		cfg.ModelServer = &ModelServer{Exe: exe, Config: "x.yaml", Listen: freeAddr(t),
			Log: filepath.Join(dir, "model.log"), ReadyTimeout: 10 * time.Second}
		t.Setenv("KEEPER_FAKE", "model")
		t.Setenv("KEEPER_UNLOADED", r.unloaded)
	}
	r.k = New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.k.poll = 50 * time.Millisecond
	r.k.botEnv = append([]string{"KEEPER_FAKE=bot", "KEEPER_ADDR=" + r.botAddr, "KEEPER_MARK=" + r.mark}, env...)
	t.Cleanup(func() { r.k.Wait(context.Background()); r.k.Shutdown() })
	return r
}

func (r *rig) do(t *testing.T, action string) {
	t.Helper()
	if err := r.k.Do(action); err != nil {
		t.Fatalf("%s: %v", action, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r.k.Wait(ctx)
}

func TestStartLoadsTheModelThenTheBotAndStopEndsBoth(t *testing.T) {
	r := newRig(t, true)
	r.do(t, "start")
	st := r.k.Status()
	if st.State != StateRunning || st.Model != "m1" || st.BotPID == 0 || st.ModelPID == 0 {
		t.Fatalf("after start: %+v", st)
	}
	if string(st.Bot) != `{"state":"idle"}` {
		t.Errorf("bot bridge = %s, want the bot's own status", st.Bot)
	}

	r.do(t, "stop")
	if st := r.k.Status(); st.State != StateStopped || st.BotPID != 0 || st.ModelPID != 0 {
		t.Fatalf("after stop: %+v", st)
	}
	if b, _ := os.ReadFile(r.mark); string(b) != "clean" {
		t.Error("the bot was killed; closing its stdin should have stopped it cleanly")
	}
	if _, err := os.Stat(r.unloaded); err != nil {
		t.Error("the model server wasn't asked to unload before it was stopped")
	}
}

func TestRestartKeepsTheModelServer(t *testing.T) {
	r := newRig(t, true)
	r.do(t, "start")
	before := r.k.Status()
	r.do(t, "restart")
	after := r.k.Status()
	if after.State != StateRunning || after.ModelPID != before.ModelPID || after.BotPID == before.BotPID {
		t.Fatalf("restart: before %+v, after %+v; want a new bot and the same model server", before, after)
	}
}

func TestColdstartRestartsTheModelServerToo(t *testing.T) {
	r := newRig(t, true)
	r.do(t, "start")
	before := r.k.Status()
	r.do(t, "coldstart")
	after := r.k.Status()
	if after.State != StateRunning || after.ModelPID == before.ModelPID || after.BotPID == before.BotPID {
		t.Fatalf("coldstart: before %+v, after %+v; want both new", before, after)
	}
}

func TestACrashIsReported(t *testing.T) {
	r := newRig(t, false, "KEEPER_CRASH=1")
	r.do(t, "start")
	deadline := time.Now().Add(10 * time.Second)
	for r.k.Status().State != StateCrashed {
		if time.Now().After(deadline) {
			t.Fatalf("status = %+v, want crashed", r.k.Status())
		}
		time.Sleep(50 * time.Millisecond)
	}
	if st := r.k.Status(); st.LastExit == nil || st.LastExit.Code != 3 || st.LastExit.What != "bot" {
		t.Errorf("last exit = %+v, want the bot's code 3", st.LastExit)
	}
}

func TestStopKillsABotThatWontLeave(t *testing.T) {
	r := newRig(t, false, "KEEPER_STUBBORN=1")
	r.k.cfg.Bot.StopTimeout = 300 * time.Millisecond
	r.do(t, "start")
	r.do(t, "stop")
	if st := r.k.Status(); st.State != StateStopped || st.BotPID != 0 {
		t.Fatalf("after stop: %+v", st)
	}
}

func TestACopyStartedByHandIsLeftAlone(t *testing.T) {
	r := newRig(t, false)
	ln, err := net.Listen("tcp", r.botAddr) // her console, answering without the keeper
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if err := r.k.Do("start"); !errors.Is(err, ErrExternal) {
		t.Fatalf("start = %v, want ErrExternal", err)
	}
	if st := r.k.Status(); st.State != StateExternal {
		t.Errorf("state = %s, want external", st.State)
	}
}

func TestOneActionAtATime(t *testing.T) {
	r := newRig(t, true)
	if err := r.k.Do("start"); err != nil {
		t.Fatal(err)
	}
	if err := r.k.Do("stop"); !errors.Is(err, ErrBusy) {
		t.Errorf("second action = %v, want ErrBusy", err)
	}
	if err := r.k.Do("explode"); !errors.Is(err, ErrUnknown) {
		t.Errorf("unknown action = %v", err)
	}
}

func TestAPIKeys(t *testing.T) {
	r := newRig(t, false)
	ts := httptest.NewServer(r.k.Handler())
	defer ts.Close()
	call := func(method, path, key string) int {
		req, _ := http.NewRequest(method, ts.URL+path, nil)
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	control := strings.Repeat("c", 32)
	for _, c := range []struct {
		method, path, key string
		want              int
	}{
		{"GET", "/status", "status-key", 200},
		{"GET", "/status", control, 200},
		{"GET", "/status", "", 401},
		{"GET", "/status", "wrong", 401},
		{"POST", "/start", "status-key", 401},
		{"POST", "/stop", "", 401},
		{"POST", "/stop", control, 202},
	} {
		if got := call(c.method, c.path, c.key); got != c.want {
			t.Errorf("%s %s with %q: %d, want %d", c.method, c.path, c.key, got, c.want)
		}
		r.k.Wait(context.Background())
	}
}

func TestStatusIsJSONWithoutTokens(t *testing.T) {
	r := newRig(t, false)
	b, _ := json.Marshal(r.k.Status())
	if strings.Contains(string(b), "status-key") || strings.Contains(string(b), strings.Repeat("c", 32)) {
		t.Errorf("status shows a key: %s", b)
	}
}

func TestLockoutAfterWrongKeys(t *testing.T) {
	var l lockout
	now := time.Now()
	for i := 0; i < lockFailures; i++ {
		l.fail("10.0.0.1", now)
	}
	if !l.locked("10.0.0.1", now) || l.locked("10.0.0.2", now) {
		t.Error("want only the guessing address locked")
	}
	if l.locked("10.0.0.1", now.Add(lockFor+time.Second)) {
		t.Error("the lock should lapse")
	}
}

func TestConfigChecks(t *testing.T) {
	good := func() *Config {
		return &Config{Listen: "127.0.0.1:8427", ControlToken: strings.Repeat("x", 32),
			Bot: Bot{Exe: "bot.exe", Config: "config.yml"}}
	}
	if err := good().check(); err != nil {
		t.Fatalf("good config: %v", err)
	}
	for name, change := range map[string]func(*Config){
		"short token":    func(c *Config) { c.ControlToken = "short" },
		"all interfaces": func(c *Config) { c.Listen = "0.0.0.0:8427" },
		"lan address":    func(c *Config) { c.Listen = "192.168.1.5:8427" },
		"no bot":         func(c *Config) { c.Bot.Exe = "" },
		"model no addr":  func(c *Config) { c.ModelServer = &ModelServer{Exe: "x"} },
	} {
		c := good()
		change(c)
		if c.check() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestModelFromConsoleOverride(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "bot.yml")
	_ = os.WriteFile(cfg, []byte("model: openai/m1\ndatadir: "+filepath.ToSlash(dir)+"\n"), 0o600)
	if s, _ := readBotSettings(cfg); s.Model != "m1" {
		t.Errorf("model = %q, want m1 without its provider", s.Model)
	}
	_ = os.WriteFile(filepath.Join(dir, "config-overrides.json"), []byte(`{"model":"openai/m2"}`), 0o600)
	if s, _ := readBotSettings(cfg); s.Model != "m2" {
		t.Errorf("model = %q, want the console's m2", s.Model)
	}
}

func TestShippedExampleParses(t *testing.T) {
	b, err := os.ReadFile("../../examples/keeper.yml")
	if err != nil {
		t.Fatal(err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	c.ControlToken = strings.Repeat("x", 32)
	if err := c.check(); err != nil || c.Listen == "" || c.Bot.StopTimeout != 30*time.Second {
		t.Errorf("examples/keeper.yml with a token: %v, %+v", err, c)
	}
}
