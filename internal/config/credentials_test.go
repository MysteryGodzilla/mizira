// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package config

import (
	"reflect"
	"strings"
	"testing"
)

const testPasskey = "passkey-0123456789"

func TestInsecureCredentialNetworks(t *testing.T) {
	tests := []struct {
		name string
		nets []*ServerConfig
		want []string
	}{
		{"no password, no tls is fine", []*ServerConfig{{Name: "local", Server: "127.0.0.1"}}, nil},
		{"server password over verified tls", []*ServerConfig{{Name: "net", ServerPass: testPasskey, SSL: true}}, nil},
		{"sasl over verified tls", []*ServerConfig{{Name: "net", SASLPass: testPasskey, SSL: true}}, nil},
		{"server password in plain text", []*ServerConfig{{Name: "net", ServerPass: testPasskey}}, []string{"net"}},
		{"server password, certificate unchecked", []*ServerConfig{{Name: "net", ServerPass: testPasskey, SSL: true, TLSInsecure: true}}, []string{"net"}},
		{"sasl in plain text", []*ServerConfig{{Name: "net", SASLPass: testPasskey}}, []string{"net"}},
		{"unnamed network falls back to host", []*ServerConfig{{Server: "irc.example.com", ServerPass: testPasskey}}, []string{"irc.example.com"}},
		{"only the bad network is named", []*ServerConfig{
			{Name: "good", ServerPass: testPasskey, SSL: true},
			{Name: "bad", ServerPass: testPasskey},
		}, []string{"bad"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := InsecureCredentialNetworks(tt.nets)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
			// The result is logged at startup, so it must never carry the password.
			if strings.Contains(strings.Join(got, " "), testPasskey) {
				t.Error("result contains the password")
			}
		})
	}
}

// A15: tools inherit the process environment. Only the env: section is exported to it, so
// connection secrets set as top-level keys must never reach a tool.
func TestToolEnvNeverCarriesConnectionSecrets(t *testing.T) {
	vars, err := parseEnvSection(t, `
serverpass: "`+testPasskey+`"
saslpass: "`+testPasskey+`"
openaikey: "`+testPasskey+`"
env:
  SOME_TOOL_SETTING: "value"
`)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range vars {
		if strings.Contains(v, testPasskey) || strings.Contains(strings.ToLower(k), "pass") {
			t.Errorf("tool environment would carry a connection secret: %s", k)
		}
	}
	if vars["SOME_TOOL_SETTING"] != "value" {
		t.Errorf("env: entries should still be exported, got %v", vars)
	}
}
