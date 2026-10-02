// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package config

// InsecureCredentialNetworks names the networks that would send a password (server password
// or SASL) without verified TLS.
//
// A15: the server password can be the owner's own account passkey. Sent in plain text, or to a
// server whose certificate isn't checked, anyone on the path can read it. Only names are
// returned, never the passwords, so the result is safe to log.
func InsecureCredentialNetworks(nets []*ServerConfig) []string {
	var insecure []string
	for _, n := range nets {
		hasPassword := n.ServerPass != "" || n.SASLPass != ""
		if hasPassword && (!n.SSL || n.TLSInsecure) {
			name := n.Name
			if name == "" {
				name = n.Server
			}
			insecure = append(insecure, name)
		}
	}
	return insecure
}
