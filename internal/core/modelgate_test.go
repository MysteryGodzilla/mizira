// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"context"
	"testing"
	"time"
)

func TestWithModelGateWaitsForTheGate(t *testing.T) {
	SetConcurrency(1)
	t.Cleanup(func() { SetConcurrency(3) })
	if !globalGate.LockWithContext(context.Background()) {
		t.Fatal("couldn't take the gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	ran := false
	if WithModelGate(ctx, func() { ran = true }) || ran {
		t.Error("ran while a reply held the gate")
	}
	globalGate.Unlock()
	if !WithModelGate(context.Background(), func() { ran = true }) || !ran {
		t.Error("didn't run once the gate was free")
	}
}
