// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

// Package keeper runs one bot, and the model server it talks to, on one machine, with an HTTP API to
// start, stop and restart them and to see their state. It is for a dashboard on the same machine.
package keeper

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is keeper.yml.
type Config struct {
	Listen string `yaml:"listen"`
	// ControlToken opens everything; the bot's statustoken opens GET /status only.
	ControlToken  string `yaml:"controltoken"`
	Log           string `yaml:"log"`
	StartAtLaunch bool   `yaml:"startatlaunch"`
	Bot           Bot    `yaml:"bot"`
	// ModelServer is optional: without it only the bot is run.
	ModelServer *ModelServer `yaml:"modelserver"`
}

type Bot struct {
	Exe    string `yaml:"exe"`
	Config string `yaml:"config"`
	// Args come after --config, e.g. --consoleonly.
	Args        []string      `yaml:"args"`
	Dir         string        `yaml:"dir"`
	Log         string        `yaml:"log"`
	StopTimeout time.Duration `yaml:"stoptimeout"`
}

// ModelServer is llama-swap: one address that starts whichever model is asked for.
type ModelServer struct {
	Exe          string        `yaml:"exe"`
	Config       string        `yaml:"config"`
	Listen       string        `yaml:"listen"`
	Log          string        `yaml:"log"`
	ErrLog       string        `yaml:"errlog"`
	ReadyTimeout time.Duration `yaml:"readytimeout"`
}

// LoadConfig reads and checks keeper.yml.
func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, c.check()
}

func (c *Config) check() error {
	if len(c.ControlToken) < 32 {
		return errors.New("controltoken must be at least 32 characters")
	}
	if err := checkListen(c.Listen); err != nil {
		return err
	}
	if c.Bot.Exe == "" || c.Bot.Config == "" {
		return errors.New("bot.exe and bot.config are required")
	}
	if c.Bot.StopTimeout <= 0 {
		c.Bot.StopTimeout = 30 * time.Second
	}
	if m := c.ModelServer; m != nil {
		if m.Exe == "" || m.Listen == "" {
			return errors.New("modelserver.exe and modelserver.listen are required")
		}
		if m.ReadyTimeout <= 0 {
			m.ReadyTimeout = 3 * time.Minute
		}
	}
	return nil
}

// checkListen refuses every-interface addresses: the API starts and stops the bot.
func checkListen(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("listen %q: %w", addr, err)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.IsLoopback() {
		return fmt.Errorf("listen %q: use a loopback address such as 127.0.0.1", addr)
	}
	return nil
}

// botSettings is what the keeper needs from the bot's own config.
type botSettings struct {
	Model       string
	AdminListen string
	StatusToken string
}

// readBotSettings reads the bot's config, and the model a console change picked (config-overrides.json
// in its data folder), which wins over model:. llama-swap knows the model without its "openai/".
func readBotSettings(path string) (botSettings, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return botSettings{}, err
	}
	var raw struct {
		Model       string `yaml:"model"`
		DataDir     string `yaml:"datadir"`
		AdminListen string `yaml:"adminlisten"`
		StatusToken string `yaml:"statustoken"`
	}
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return botSettings{}, fmt.Errorf("%s: %w", path, err)
	}
	model := raw.Model
	if raw.DataDir != "" {
		if ob, err := os.ReadFile(filepath.Join(raw.DataDir, "config-overrides.json")); err == nil {
			var o struct {
				Model string `json:"model"`
			}
			if json.Unmarshal(ob, &o) == nil && o.Model != "" {
				model = o.Model
			}
		}
	}
	if _, after, ok := strings.Cut(model, "/"); ok {
		model = after
	}
	return botSettings{Model: model, AdminListen: raw.AdminListen, StatusToken: raw.StatusToken}, nil
}
