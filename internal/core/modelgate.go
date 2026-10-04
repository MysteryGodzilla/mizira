// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import "context"

// WithModelGate runs fn holding the process-wide model gate, so model work outside any conversation
// (self-note proposals) waits its turn behind replies instead of competing with them. False if ctx
// ended before the gate was free.
func WithModelGate(ctx context.Context, fn func()) bool {
	if !globalGate.LockWithContext(ctx) {
		return false
	}
	defer globalGate.Unlock()
	fn()
	return true
}
