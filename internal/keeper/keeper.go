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
	"net/url"
	"path/filepath"
	"sync"
	"time"
)

// States, as GET /status reports them.
const (
	StateStopped  = "stopped"
	StateLoading  = "loading_model" // the model server is up and loading the bot's model
	StateStarting = "starting"      // the bot is starting
	StateRunning  = "running"
	StateStopping = "stopping"
	StateCrashed  = "crashed"  // something the keeper started stopped by itself, or a start failed
	StateExternal = "external" // a copy the keeper didn't start is running (started by hand)
)

// Actions, as POST /<action> takes them.
var Actions = []string{"start", "stop", "restart", "coldstart"}

var (
	ErrBusy     = errors.New("another action is still running")
	ErrExternal = errors.New("a copy started outside the keeper is running; stop that one first")
	ErrUnknown  = errors.New("unknown action")
)

// Exit is the last time something the keeper started stopped by itself.
type Exit struct {
	What string `json:"what"`
	Code int    `json:"code"`
	At   int64  `json:"at"`
}

type Status struct {
	State    string `json:"state"`
	Since    int64  `json:"since"`
	Detail   string `json:"detail,omitempty"`
	Busy     string `json:"busy,omitempty"`
	Model    string `json:"model,omitempty"`
	BotPID   int    `json:"bot_pid,omitempty"`
	ModelPID int    `json:"model_pid,omitempty"`
	LastExit *Exit  `json:"last_exit,omitempty"`
	// Bot is the bot's own status bridge (GET /api/v1/bridge) while it answers.
	Bot json.RawMessage `json:"bot,omitempty"`
}

type Keeper struct {
	cfg    *Config
	log    *slog.Logger
	client *http.Client
	// botEnv is added to the bot's environment; tests swap the stop switch for their fake bot's.
	botEnv []string
	// poll is how often waits look again.
	poll time.Duration

	mu        sync.Mutex
	state     string
	since     time.Time
	detail    string
	busy      string
	bot       *proc
	model     *proc
	modelName string
	lastExit  *Exit
}

func New(cfg *Config, log *slog.Logger) *Keeper {
	return &Keeper{cfg: cfg, log: log, client: &http.Client{Timeout: 5 * time.Second},
		botEnv: []string{"METALD_STOP_ON_STDIN_EOF=1"}, poll: 2 * time.Second,
		state: StateStopped, since: time.Now()}
}

func (k *Keeper) set(state, detail string) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.state != state || k.detail != detail {
		k.state, k.detail, k.since = state, detail, time.Now()
	}
}

func alive(p *proc) bool { return p != nil && !p.exited() }

// Do runs an action in the background. It refuses at once when another is running, or when starting
// would collide with a copy started by hand.
func (k *Keeper) Do(action string) error {
	run := map[string]func() error{"start": k.start, "stop": k.stop, "restart": k.restart,
		"coldstart": k.coldstart}[action]
	if run == nil {
		return ErrUnknown
	}
	k.mu.Lock()
	if k.busy != "" {
		k.mu.Unlock()
		return ErrBusy
	}
	mine := alive(k.bot) || alive(k.model)
	k.busy = action
	k.mu.Unlock()
	if action != "stop" && !mine && k.external() {
		k.mu.Lock()
		k.busy = ""
		k.mu.Unlock()
		return ErrExternal
	}
	k.log.Info("keeper_action", "action", action)
	go func() {
		err := run()
		k.mu.Lock()
		k.busy = ""
		k.mu.Unlock()
		if err != nil {
			k.log.Error("keeper_action_failed", "action", action, "error", err.Error())
			k.set(StateCrashed, action+" failed: "+err.Error())
		}
	}()
	return nil
}

// Wait blocks until no action is running; for tests and shutdown.
func (k *Keeper) Wait(ctx context.Context) {
	for {
		k.mu.Lock()
		idle := k.busy == ""
		k.mu.Unlock()
		if idle || ctx.Err() != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (k *Keeper) start() error {
	s, err := readBotSettings(k.cfg.Bot.Config)
	if err != nil {
		return err
	}
	k.mu.Lock()
	needModel, needBot := k.cfg.ModelServer != nil && !alive(k.model), !alive(k.bot)
	k.mu.Unlock()
	if needModel {
		if err := k.startModel(s.Model); err != nil {
			return err
		}
	}
	if needBot {
		return k.startBot(s)
	}
	k.set(StateRunning, "")
	return nil
}

func (k *Keeper) startModel(model string) error {
	m := k.cfg.ModelServer
	k.set(StateLoading, model)
	p, err := launch(m.Exe, []string{"-config", m.Config, "-listen", m.Listen}, filepath.Dir(m.Exe), m.Log, m.ErrLog, nil)
	if err != nil {
		return fmt.Errorf("model server: %w", err)
	}
	k.mu.Lock()
	k.model, k.modelName = p, model
	k.mu.Unlock()
	go k.watch("model server", p)
	k.log.Info("keeper_model_started", "pid", p.pid(), "model", model)

	deadline := time.Now().Add(m.ReadyTimeout)
	for {
		if p.exited() {
			return fmt.Errorf("the model server stopped while loading (exit code %d); see its log", p.exitCode())
		}
		if ready, listed := k.modelReady(model); ready {
			return nil
		} else if !listed && model != "" {
			// Asking for anything through /upstream/<model> makes llama-swap start that model.
			k.get("/upstream/" + url.PathEscape(model) + "/health")
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s didn't become ready within %s", model, m.ReadyTimeout)
		}
		time.Sleep(k.poll)
	}
}

// modelReady asks llama-swap which models are running. With no model named, an answer is enough.
func (k *Keeper) modelReady(model string) (ready, listed bool) {
	var running struct {
		Running []struct {
			Model string `json:"model"`
			State string `json:"state"`
		} `json:"running"`
	}
	body, ok := k.get("/running")
	if !ok || json.Unmarshal(body, &running) != nil {
		return false, false
	}
	if model == "" {
		return true, true
	}
	for _, r := range running.Running {
		if r.Model == model {
			return r.State == "ready", true
		}
	}
	return false, false
}

// get asks the model server for path, and reports whether it answered 200.
func (k *Keeper) get(path string) ([]byte, bool) {
	resp, err := k.client.Get("http://" + k.cfg.ModelServer.Listen + path)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return body, err == nil && resp.StatusCode == http.StatusOK
}

func (k *Keeper) startBot(s botSettings) error {
	b := k.cfg.Bot
	k.set(StateStarting, "")
	args := append([]string{"--config", b.Config}, b.Args...)
	p, err := launch(b.Exe, args, b.Dir, b.Log, "", k.botEnv)
	if err != nil {
		return fmt.Errorf("bot: %w", err)
	}
	k.mu.Lock()
	k.bot = p
	k.mu.Unlock()
	go k.watch("bot", p)
	k.log.Info("keeper_bot_started", "pid", p.pid())

	// Running once her status bridge answers; without one, once she has stayed up a few seconds.
	deadline := time.Now().Add(time.Minute)
	settled := time.Now().Add(5 * time.Second)
	for {
		if p.exited() {
			return fmt.Errorf("the bot stopped while starting (exit code %d); see its log", p.exitCode())
		}
		if s.StatusToken != "" && s.AdminListen != "" {
			if k.bridge(s) != nil {
				break
			}
		} else if time.Now().After(settled) {
			break
		}
		if time.Now().After(deadline) {
			return errors.New("the bot didn't answer within a minute; see its log")
		}
		time.Sleep(k.poll)
	}
	k.set(StateRunning, "")
	return nil
}

// bridge is the bot's own status, or nil when it doesn't answer.
func (k *Keeper) bridge(s botSettings) json.RawMessage {
	if s.StatusToken == "" || s.AdminListen == "" {
		return nil
	}
	req, err := http.NewRequest(http.MethodGet, "http://"+s.AdminListen+"/api/v1/bridge", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+s.StatusToken)
	c := http.Client{Timeout: 2 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil || resp.StatusCode != http.StatusOK || !json.Valid(body) {
		return nil
	}
	return body
}

// watch records a child that stopped by itself. During an action the action handles it.
func (k *Keeper) watch(what string, p *proc) {
	<-p.done
	k.mu.Lock()
	current := k.bot == p || k.model == p
	ours := k.busy != ""
	k.mu.Unlock()
	if !current || ours {
		return
	}
	k.log.Warn("keeper_child_exited", "what", what, "code", p.exitCode())
	k.mu.Lock()
	k.lastExit = &Exit{What: what, Code: p.exitCode(), At: time.Now().Unix()}
	k.mu.Unlock()
	k.set(StateCrashed, fmt.Sprintf("the %s stopped by itself (exit code %d)", what, p.exitCode()))
}

func (k *Keeper) stopBot() {
	k.mu.Lock()
	p := k.bot
	k.mu.Unlock()
	if !alive(p) {
		return
	}
	k.set(StateStopping, "bot")
	clean := p.stop(k.cfg.Bot.StopTimeout)
	k.forget(p)
	k.log.Info("keeper_bot_stopped", "clean", clean)
}

// forget drops a child stopped on purpose, so its exit isn't taken for a crash.
func (k *Keeper) forget(p *proc) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.bot == p {
		k.bot = nil
	}
	if k.model == p {
		k.model, k.modelName = nil, ""
	}
}

func (k *Keeper) stop() error {
	k.stopBot()
	k.mu.Lock()
	m := k.model
	k.mu.Unlock()
	if alive(m) {
		k.set(StateStopping, "model server")
		// Unload first, so the model it started doesn't outlive it and keep the GPU busy.
		c := http.Client{Timeout: 30 * time.Second}
		if resp, err := c.Get("http://" + k.cfg.ModelServer.Listen + "/unload"); err == nil {
			resp.Body.Close()
		}
		m.stop(5 * time.Second)
		k.forget(m)
		k.log.Info("keeper_model_stopped")
	}
	k.set(StateStopped, "")
	return nil
}

// restart restarts the bot and keeps the model server and its loaded model.
func (k *Keeper) restart() error {
	k.stopBot()
	return k.start()
}

// coldstart stops everything, the model server too, and starts again.
func (k *Keeper) coldstart() error {
	if err := k.stop(); err != nil {
		return err
	}
	return k.start()
}

// external reports whether something the keeper didn't start answers on the bot's or the model
// server's address.
func (k *Keeper) external() bool {
	if m := k.cfg.ModelServer; m != nil && listening(m.Listen) {
		return true
	}
	s, err := readBotSettings(k.cfg.Bot.Config)
	return err == nil && s.AdminListen != "" && listening(s.AdminListen)
}

func listening(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func (k *Keeper) Status() Status {
	k.mu.Lock()
	st := Status{State: k.state, Since: k.since.Unix(), Detail: k.detail, Busy: k.busy, LastExit: k.lastExit}
	if alive(k.bot) {
		st.BotPID = k.bot.pid()
	}
	if alive(k.model) {
		st.ModelPID, st.Model = k.model.pid(), k.modelName
	}
	mine := alive(k.bot) || alive(k.model)
	k.mu.Unlock()
	if !mine && st.Busy == "" && (st.State == StateStopped || st.State == StateCrashed) && k.external() {
		st.State, st.Detail = StateExternal, "started outside the keeper"
	}
	if s, err := readBotSettings(k.cfg.Bot.Config); err == nil {
		st.Bot = k.bridge(s)
	}
	return st
}

// Shutdown stops what the keeper started, when the keeper itself is stopping.
func (k *Keeper) Shutdown() {
	k.mu.Lock()
	mine := alive(k.bot) || alive(k.model)
	k.mu.Unlock()
	if mine {
		_ = k.stop()
	}
}
