// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"fmt"
	"log/slog"

	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// PauseCommand handles +pause: the soft stop. Nothing new starts, but replies already being
// written are allowed to finish. Admin commands keep working so +resume is always possible.
type PauseCommand struct{}

func (c *PauseCommand) Name() string    { return "+pause" }
func (c *PauseCommand) AdminOnly() bool { return true }

func (c *PauseCommand) Execute(ctx irc.ChatContextInterface) {
	change := ChangeRunState(core.Paused, ctx.GetSource(), ctx.GetRequestID(), ctx.GetLogger())
	switch {
	case change.Changed:
		ctx.Reply("Paused. Anything already running will finish. +resume to continue.")
	case change.Previous == core.Stopped:
		// Stopped is the stronger state; pausing must not quietly weaken it.
		ctx.Reply("Already stopped. +resume to continue.")
	default:
		ctx.Reply("Already paused. +resume to continue.")
	}
}

// StopCommand handles +stop: the emergency brake. The state changes first, so nothing new can
// start, then every running request except this one is cancelled. Cancelled requests are
// silenced at the send path, so their half-written replies never reach the channel.
type StopCommand struct{}

func (c *StopCommand) Name() string    { return "+stop" }
func (c *StopCommand) AdminOnly() bool { return true }

func (c *StopCommand) Execute(ctx irc.ChatContextInterface) {
	change := ChangeRunState(core.Stopped, ctx.GetSource(), ctx.GetRequestID(), ctx.GetLogger())
	ctx.Reply(fmt.Sprintf("Stopped. %d running request(s) cancelled. +resume to continue.", change.Cancelled))
}

// ResumeCommand handles +resume: back to normal after +pause or +stop.
type ResumeCommand struct{}

func (c *ResumeCommand) Name() string    { return "+resume" }
func (c *ResumeCommand) AdminOnly() bool { return true }

func (c *ResumeCommand) Execute(ctx irc.ChatContextInterface) {
	if !ChangeRunState(core.Running, ctx.GetSource(), ctx.GetRequestID(), ctx.GetLogger()).Changed {
		ctx.Reply("Not paused.")
		return
	}
	ctx.Reply("Resumed.")
}

// RunChange is what ChangeRunState did.
type RunChange struct {
	Previous  core.RunState
	Changed   bool
	Cancelled int // requests stopped, for a stop
}

// ChangeRunState is +pause, +stop and +resume without the chat, shared with the operator console so
// both behave the same. Pausing never weakens a stop, resuming a running bot does nothing, and a
// stop always cancels whatever is running except keepRequest (the request asking, if any).
func ChangeRunState(to core.RunState, by, keepRequest string, log *slog.Logger) RunChange {
	now := core.State()
	switch {
	case to == core.Paused && now != core.Running, to == core.Running && now == core.Running:
		return RunChange{Previous: now}
	}
	change := RunChange{Previous: setRunState(to), Changed: true}
	switch to {
	case core.Paused:
		log.Warn("bot_paused", "by", by)
	case core.Stopped:
		change.Cancelled = core.Requests().CancelAll(keepRequest)
		log.Warn("bot_stopped", "by", by, "cancelled", change.Cancelled)
	case core.Running:
		log.Info("bot_resumed", "by", by, "was", change.Previous.String())
	}
	return change
}

// setRunState changes the state and saves it, so a bot that was stopped stays stopped after a
// restart until someone resumes it.
func setRunState(s core.RunState) core.RunState {
	previous := core.SetState(s)
	PersistRunState(s)
	return previous
}
