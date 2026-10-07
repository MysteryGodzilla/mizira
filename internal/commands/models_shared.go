// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
)

// ModelInfo is one model the model server offers: its id, and a display name if the server gives one
// (llama-swap does).
type ModelInfo struct {
	ID, Name string
}

var (
	ErrNoModelServer = errors.New("no openaiurl configured")
	ErrNoSuchModel   = errors.New("the model server doesn't offer that model")
	ErrSameModel     = errors.New("already on that model")
)

// ListModels is "+models": what the model server offers.
func ListModels(ctx context.Context, cfg *config.Configuration) ([]ModelInfo, error) {
	base := strings.TrimSuffix(cfg.API.OpenAIURL, "/")
	if base == "" {
		return nil, ErrNoModelServer
	}
	return fetchModels(ctx, base, cfg.API.OpenAIKey)
}

// SwitchModel is "+models <name>", shared with the console: switch to one of models (matched without
// case), saved across restarts. It returns the model's own spelling. The caller clears the
// conversation, as a new model shouldn't carry on from turns written by the old one.
func SwitchModel(cfg *config.Configuration, sys core.System, models []ModelInfo, want, by string, log *slog.Logger) (string, error) {
	want = modelNameOnly(strings.TrimSpace(want))
	canonical := ""
	for _, m := range models {
		if strings.EqualFold(m.ID, want) {
			canonical = m.ID
			break
		}
	}
	if canonical == "" {
		log.Info("model_switch_rejected", "requested", want, "by", by)
		return "", ErrNoSuchModel
	}
	current := modelNameOnly(cfg.Model.Model)
	if canonical == current {
		return canonical, ErrSameModel
	}

	// Keep the provider prefix the existing config uses ("openai/chat"), since the LLM client routes
	// on it.
	value := canonical
	if prefix, _, found := strings.Cut(cfg.Model.Model, "/"); found {
		value = prefix + "/" + canonical
	}
	configMu.Lock()
	defer configMu.Unlock()
	if err := configFields["model"].setter(cfg, value); err != nil {
		return "", err
	}
	if err := sys.UpdateLLM(*cfg.API); err != nil {
		log.Error("llm_update_failed", "error", err)
		return canonical, ErrLLMNotUpdated
	}
	PersistSet("model", value)
	log.Info("model_switched", "from", current, "to", canonical, "by", by)
	return canonical, nil
}
