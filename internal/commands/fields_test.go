// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"strings"
	"testing"

	"B4reMetal/metald/internal/config"
)

// A15: connection passwords can be the owner's account passkey. +get would print them to the
// channel and +set would let an admin session swap them, so they must not be fields at all.
func TestConnectionSecretsAreNotSettable(t *testing.T) {
	for _, key := range []string{"serverpass", "saslpass", "channelkey"} {
		if _, ok := configFields[key]; ok {
			t.Errorf("%s must not be reachable through +get/+set", key)
		}
	}
}

// API keys can be read with +get, but only masked.
func TestAPIKeyGettersMask(t *testing.T) {
	const secret = "sk-test-0123456789abcdef"
	cfg := &config.Configuration{API: &config.APIConfig{
		OpenAIKey: secret, AnthropicKey: secret, GeminiKey: secret, OllamaKey: secret,
	}}
	for _, key := range []string{"openaikey", "anthropickey", "geminikey", "ollamakey"} {
		got := configFields[key].getter(cfg)
		if strings.Contains(got, secret) || strings.Contains(got, secret[:12]) {
			t.Errorf("+get %s shows too much of the key: %q", key, got)
		}
	}
}
