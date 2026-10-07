// Copyright (C) 2026 BareMetal
// Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"B4reMetal/metald/internal/irc"
)

// ModelsCommand lists the models the proxy can serve, and switches between them.
type ModelsCommand struct{}

func (c *ModelsCommand) Name() string    { return "+models" }
func (c *ModelsCommand) AdminOnly() bool { return true }

const modelsProbeTimeout = 15 * time.Second

func (c *ModelsCommand) Execute(ctx irc.ChatContextInterface) {
	cfg := ctx.GetConfig()
	base := strings.TrimSuffix(cfg.API.OpenAIURL, "/")
	if base == "" {
		ctx.Reply("No openaiurl configured")
		return
	}

	models, err := ListModels(ctx, cfg)
	if err != nil {
		// The URL names an internal host; the channel gets none of it.
		ctx.GetLogger().Warn("models_probe_failed", "url", base, "error", err.Error())
		ctx.Reply("Could not reach the model list - see logs")
		return
	}
	if len(models) == 0 {
		ctx.Reply("The proxy reports no models")
		return
	}
	current := modelNameOnly(cfg.Model.Model)

	// No argument: list what's on offer and mark the active one.
	if len(ctx.GetArgs()) < 2 {
		var b strings.Builder
		for i, m := range models {
			if i > 0 {
				b.WriteString(", ")
			}
			if m.ID == current {
				fmt.Fprintf(&b, "[%s]", m.ID)
			} else {
				b.WriteString(m.ID)
			}
		}
		ctx.Reply(fmt.Sprintf("models: %s  (current in brackets; +models <name> to switch)", b.String()))
		return
	}

	canonical, err := SwitchModel(cfg, ctx.GetSystem(), models, ctx.GetArgs()[1], ctx.GetSource(), ctx.GetLogger())
	switch {
	case errors.Is(err, ErrNoSuchModel):
		ids := make([]string, len(models))
		for i, m := range models {
			ids[i] = m.ID
		}
		ctx.Reply(fmt.Sprintf("no such model: %s. available: %s", modelNameOnly(strings.TrimSpace(ctx.GetArgs()[1])), strings.Join(ids, ", ")))
		return
	case errors.Is(err, ErrSameModel):
		ctx.Reply(fmt.Sprintf("already on %s", canonical))
		return
	case errors.Is(err, ErrLLMNotUpdated):
		ctx.Reply("Model saved, but failed to update the LLM client")
		return
	case err != nil:
		ctx.Reply(err.Error())
		return
	}
	ctx.GetSession().Clear()
	ctx.Reply(fmt.Sprintf("model set to %s (history cleared; first reply may be slow while it loads)", canonical))
}

// fetchModels asks the proxy which model groups it serves.
func fetchModels(ctx context.Context, base, key string) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, err
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := (&http.Client{Timeout: modelsProbeTimeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("model list returned http %d", resp.StatusCode)
	}

	var payload struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"` // llama-swap's display name, if any
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	var out []ModelInfo
	for _, m := range payload.Data {
		// The "-backup" groups are failover targets, not things to select by hand: picking one pins the
		// bot to the weak local model with no way back if the remote recovers.
		if m.ID == "" || strings.HasSuffix(m.ID, "-backup") || strings.HasSuffix(m.ID, "-fallback") {
			continue
		}
		out = append(out, ModelInfo{ID: m.ID, Name: m.Name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
