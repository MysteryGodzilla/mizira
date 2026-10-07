// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	mocktest "B4reMetal/metald/internal/testing"
)

// llama-swap's display names come through; a switch saves the server's spelling with the config's
// provider prefix, and switching to the current model or an unknown one is refused.
func TestListAndSwitchModels(t *testing.T) {
	useTempOverrides(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[{"id":"small-model","name":"Small Model"},{"id":"chat"}]}`))
	}))
	defer srv.Close()
	cfg := mocktest.DefaultTestConfig()
	cfg.API.OpenAIURL = srv.URL
	cfg.Model.Model = "openai/chat"
	sys := mocktest.NewMockSystem()

	models, err := ListModels(context.Background(), cfg)
	if err != nil || len(models) != 2 || models[1] != (ModelInfo{ID: "small-model", Name: "Small Model"}) {
		t.Fatalf("models: %+v %v", models, err)
	}
	if got, err := SwitchModel(cfg, sys, models, "Small-Model", "console:token", quiet); err != nil || got != "small-model" || cfg.Model.Model != "openai/small-model" {
		t.Errorf("switch: %q %v, model now %q", got, err, cfg.Model.Model)
	}
	if _, err := SwitchModel(cfg, sys, models, "small-model", "console:token", quiet); !errors.Is(err, ErrSameModel) {
		t.Errorf("switching to the current model: %v", err)
	}
	if _, err := SwitchModel(cfg, sys, models, "huge-model", "console:token", quiet); !errors.Is(err, ErrNoSuchModel) {
		t.Errorf("an unknown model: %v", err)
	}
	cfg.API.OpenAIURL = ""
	if _, err := ListModels(context.Background(), cfg); !errors.Is(err, ErrNoModelServer) {
		t.Errorf("no server: %v", err)
	}
}
