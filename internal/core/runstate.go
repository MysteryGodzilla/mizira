// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import "sync/atomic"

// RunState is whether the bot is taking on new work. It applies to every network at once:
// +pause and +stop are for "the bot is acting up", not for one channel.
type RunState int32

const (
	// Running is normal operation.
	Running RunState = iota
	// Paused is the soft stop: nothing new starts, but work already running may finish.
	Paused
	// Stopped is the emergency stop: everything running was cancelled, and nothing new starts.
	Stopped
)

var runState atomic.Int32

// State returns the current run state.
func State() RunState { return RunState(runState.Load()) }

// SetState changes the run state and returns the previous one.
func SetState(s RunState) RunState { return RunState(runState.Swap(int32(s))) }

// Halted reports whether the bot is paused or stopped. Anything that would start new work
// (a reply, a greeting, a reminder) checks this first.
func Halted() bool { return State() != Running }

func (s RunState) String() string {
	switch s {
	case Paused:
		return "paused"
	case Stopped:
		return "stopped"
	default:
		return "running"
	}
}

// ParseRunState reads a state saved with String. Unknown text is not a state.
func ParseRunState(s string) (RunState, bool) {
	switch s {
	case "running":
		return Running, true
	case "paused":
		return Paused, true
	case "stopped":
		return Stopped, true
	}
	return Running, false
}
