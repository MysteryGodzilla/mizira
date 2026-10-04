// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// Setting is one +set key as the operator console shows it.
type Setting struct {
	Key        string
	Value      string // what +get shows
	Default    string // config.yml's value, before any persisted +set
	Overridden bool   // a +set value is persisted over config.yml's
	Editable   bool
}

var (
	ErrUnknownSetting = errors.New("no such setting")
	ErrNotEditable    = errors.New("can't be changed from the console")
)

// configDefaults are config.yml's values, captured by ApplyOverrides before it layers anything on.
var (
	configDefaultsMu sync.RWMutex
	configDefaults   = map[string]string{}
)

// secretSetting reports keys whose values are credentials: never shown or changed by the console.
func secretSetting(key string) bool {
	return strings.HasSuffix(key, "key") || strings.Contains(key, "pass") || strings.Contains(key, "token")
}

// consoleReadOnly are settings shown but not changed from the console.
var consoleReadOnly = []string{
	"prompt", // ~set prompt doesn't reach existing conversations yet (T13 review note 3)
	// The API key goes wherever these point, so a page that could change them could send it away.
	"openaiurl", "ollamaurl",
}

// rawSetting turns a getter's display value back into what its setter accepts.
func rawSetting(shown string) string {
	if strings.HasPrefix(shown, "(unset") {
		return ""
	}
	return shown
}

func captureDefaults(cfg *config.Configuration) {
	configDefaultsMu.Lock()
	defer configDefaultsMu.Unlock()
	for key, field := range configFields {
		if v, ok := readSetting(cfg, field); ok && !secretSetting(key) {
			configDefaults[key] = rawSetting(v)
		}
	}
}

// readSetting is the getter, or false for a config missing the section it reads (tests build
// partial ones).
func readSetting(cfg *config.Configuration, field configField) (v string, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return field.getter(cfg), true
}

// Settings lists every +set key except credentials, sorted.
func Settings(cfg *config.Configuration) []Setting {
	overridesMu.Lock()
	persisted := loadOverrides().Set
	overridesMu.Unlock()
	configDefaultsMu.RLock()
	defer configDefaultsMu.RUnlock()

	var out []Setting
	for _, key := range getConfigKeys() {
		if secretSetting(key) {
			continue
		}
		_, overridden := persisted[key]
		out = append(out, Setting{Key: key, Value: configFields[key].getter(cfg), Default: configDefaults[key],
			Overridden: overridden, Editable: !slices.Contains(consoleReadOnly, key)})
	}
	slices.SortFunc(out, func(a, b Setting) int { return strings.Compare(a.Key, b.Key) })
	return out
}

// applySetting is +set's work: validate and set the value, refresh the model client when the
// change affects it, and persist the raw value so it survives a restart.
func applySetting(cfg *config.Configuration, sys core.System, key, value string, log *slog.Logger) error {
	field, ok := configFields[key]
	if !ok {
		return ErrUnknownSetting
	}
	configMu.Lock()
	defer configMu.Unlock()
	if err := field.setter(cfg, value); err != nil {
		return err
	}
	var warn error
	if sys != nil && (strings.Contains(key, "key") || strings.Contains(key, "url") || strings.Contains(key, "model")) {
		if err := sys.UpdateLLM(*cfg.API); err != nil {
			log.Error("llm_update_failed", "error", err)
			warn = ErrLLMNotUpdated
		}
	}
	PersistSet(key, value)
	return warn
}

// ErrLLMNotUpdated: the value was saved, but the model client couldn't be rebuilt with it.
var ErrLLMNotUpdated = errors.New("saved, but the model client didn't update")

// SetSetting is "+set key value" from the operator console: credentials and read-only keys are
// refused. It returns the new value as +get shows it.
func SetSetting(cfg *config.Configuration, sys core.System, key, value, by string, log *slog.Logger) (string, error) {
	if _, ok := configFields[key]; !ok || secretSetting(key) {
		return "", ErrUnknownSetting
	}
	if slices.Contains(consoleReadOnly, key) {
		return "", ErrNotEditable
	}
	before := configFields[key].getter(cfg)
	err := applySetting(cfg, sys, key, value, log)
	if err != nil && !errors.Is(err, ErrLLMNotUpdated) {
		return "", err
	}
	after := configFields[key].getter(cfg)
	log.Info("config_changed", "key", key, "from", before, "to", after, "by", by)
	return after, err
}

// ResetSetting puts a key back to config.yml's value and drops its persisted override.
func ResetSetting(cfg *config.Configuration, sys core.System, key, by string, log *slog.Logger) (string, error) {
	if _, ok := configFields[key]; !ok || secretSetting(key) {
		return "", ErrUnknownSetting
	}
	configDefaultsMu.RLock()
	def, ok := configDefaults[key]
	configDefaultsMu.RUnlock()
	if !ok {
		return "", fmt.Errorf("config.yml's value of %s isn't known", key)
	}
	before := configFields[key].getter(cfg)
	err := applySetting(cfg, sys, key, def, log)
	if err != nil && !errors.Is(err, ErrLLMNotUpdated) {
		return "", err
	}
	PersistUnset(key)
	after := configFields[key].getter(cfg)
	log.Info("config_reset", "key", key, "from", before, "to", after, "by", by)
	return after, err
}

// BotKind is how another bot is recognised: by its nick, or by the prefix on its lines.
type BotKind string

const (
	BotByNick   BotKind = "nick"
	BotByPrefix BotKind = "prefix"
)

var (
	ErrAlreadyListed = errors.New("is already listed")
	ErrNotListed     = errors.New("wasn't listed")
)

// ChangeBots is "+bots add|remove nick|prefix <value>". It returns the value as stored (prefixes
// are normalised).
func ChangeBots(cfg *config.Configuration, botNick string, add bool, kind BotKind, value, by string, log *slog.Logger) (string, error) {
	bot := cfg.Bot
	var list *[]string
	switch kind {
	case BotByNick:
		list = &bot.BotNicks
		if add {
			if err := irc.ValidateBotNick(cfg, value, botNick); err != nil {
				return "", err
			}
		}
	case BotByPrefix:
		raw := value
		list, value = &bot.BotPrefixes, irc.NormaliseBotPrefix(raw)
		if add {
			if err := irc.ValidateBotPrefix(cfg, raw); err != nil {
				return "", err
			}
		}
	default:
		return "", fmt.Errorf("unknown kind %q", kind)
	}

	configMu.Lock()
	defer configMu.Unlock()
	if add && !addNick(list, value) {
		return value, ErrAlreadyListed
	}
	if !add && !removeNick(list, value) {
		return value, ErrNotListed
	}
	PersistBots(bot.BotPrefixes, bot.BotNicks)
	action := "remove"
	if add {
		action = "add"
	}
	log.Info("bots_changed", "action", action, "kind", string(kind), "value", value, "by", by)
	return value, nil
}
