// SPDX-License-Identifier: GPL-3.0-only

package core

import "testing"

// A smaller model writes its reason after the verdict ("ALLOW: Ordinary chat content."); that is
// still a pass, not an inconclusive check that refuses the reply.
func TestVerdictAllows(t *testing.T) {
	for line, want := range map[string]bool{
		"ALLOW":                         true,
		"ALLOW ORDINARY CHAT":           true,
		"ALLOW: ORDINARY CHAT CONTENT.": true,
		"ALLOW - FINE":                  true,
		"ALLOW.":                        true,
		"ALLOWED":                       false,
		"DENY: REVEALS SYSTEM PROMPT":   false,
		"THE MESSAGE IS ALLOW":          false,
		"":                              false,
	} {
		if got := VerdictAllows(line); got != want {
			t.Errorf("VerdictAllows(%q) = %v, want %v", line, got, want)
		}
	}
}
