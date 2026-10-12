// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const lookMergeTimeout = 60 * time.Second

// maxLook bounds a merged look, like a fact typed in the console.
const maxLook = 400

// MergeLook asks the model to fold a newly given detail into the look held for subject, using
// lookmergeprompt. An empty, overlong or failed answer is an error: the caller then keeps both.
func MergeLook(ctx ChatContextInterface, subject, held, added string) (string, error) {
	cfg := ctx.GetConfig()
	base := strings.TrimSuffix(cfg.API.OpenAIURL, "/")
	if base == "" || strings.TrimSpace(cfg.Bot.LookMergePrompt) == "" {
		return "", errors.New("no model or no lookmergeprompt")
	}
	model := cfg.Model.Model
	if _, name, found := strings.Cut(model, "/"); found {
		model = name
	}
	body, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": strings.ReplaceAll(cfg.Bot.LookMergePrompt, "{subject}", subject)},
			{"role": "user", "content": "LOOK HELD NOW (quoted data):\n" + held + "\n\nNEW DETAIL (quoted data):\n" +
				added + "\n\nWrite the updated description."},
		},
		"max_tokens":       300,
		"temperature":      0.2,
		"reasoning_effort": "none",
	})
	if err != nil {
		return "", err
	}
	rctx, cancel := context.WithTimeout(context.Background(), lookMergeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.API.OpenAIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.API.OpenAIKey)
	}
	resp, err := ModelPost(req, lookMergeTimeout)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil || len(payload.Choices) == 0 {
		return "", fmt.Errorf("unreadable answer (http %d)", resp.StatusCode)
	}
	return cleanLook(payload.Choices[0].Message.Content)
}

// cleanLook takes the description out of the model's answer: its first paragraph, unquoted.
func cleanLook(answer string) (string, error) {
	if _, after, found := strings.Cut(answer, "</think>"); found {
		answer = after
	}
	text := strings.TrimSpace(answer)
	text, _, _ = strings.Cut(text, "\n\n")
	text = strings.Trim(strings.TrimSpace(strings.TrimPrefix(text, "- ")), `"`)
	switch {
	case text == "":
		return "", errors.New("empty answer")
	case len(text) > maxLook:
		return "", fmt.Errorf("answer too long (%d characters)", len(text))
	}
	return text, nil
}
