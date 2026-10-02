// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"fmt"

	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// PauseCommand handles +pause: the soft stop. Nothing new starts, but replies already being
// written are allowed to finish. Admin commands keep working so +resume is always possible.
type PauseCommand struct{}

func (c *PauseCommand) Name() string    { return "+pause" }
func (c *PauseCommand) AdminOnly() bool { return true }

func (c *PauseCommand) Execute(ctx irc.ChatContextInterface) {
	switch core.State() {
	case core.Paused:
		ctx.Reply("Already paused. +resume to continue.")
		return
	case core.Stopped:
		// Stopped is the stronger state; pausing must not quietly weaken it.
		ctx.Reply("Already stopped. +resume to continue.")
		return
	}
	setRunState(core.Paused)
	ctx.GetLogger().Warn("bot_paused", "by", ctx.GetSource())
	ctx.Reply("Paused. Anything already running will finish. +resume to continue.")
}

// StopCommand handles +stop: the emergency brake. The state changes first, so nothing new can
// start, then every running request except this one is cancelled. Cancelled requests are
// silenced at the send path, so their half-written replies never reach the channel.
type StopCommand struct{}

func (c *StopCommand) Name() string    { return "+stop" }
func (c *StopCommand) AdminOnly() bool { return true }

func (c *StopCommand) Execute(ctx irc.ChatContextInterface) {
	setRunState(core.Stopped)
	cancelled := core.Requests().CancelAll(ctx.GetRequestID())
	ctx.GetLogger().Warn("bot_stopped", "by", ctx.GetSource(), "cancelled", cancelled)
	ctx.Reply(fmt.Sprintf("Stopped. %d running request(s) cancelled. +resume to continue.", cancelled))
}

// ResumeCommand handles +resume: back to normal after +pause or +stop.
type ResumeCommand struct{}

func (c *ResumeCommand) Name() string    { return "+resume" }
func (c *ResumeCommand) AdminOnly() bool { return true }

func (c *ResumeCommand) Execute(ctx irc.ChatContextInterface) {
	if core.State() == core.Running {
		ctx.Reply("Not paused.")
		return
	}
	previous := setRunState(core.Running)
	ctx.GetLogger().Info("bot_resumed", "by", ctx.GetSource(), "was", previous.String())
	ctx.Reply("Resumed.")
}

// setRunState changes the state and saves it, so a bot that was stopped stays stopped after a
// restart until someone resumes it.
func setRunState(s core.RunState) core.RunState {
	previous := core.SetState(s)
	PersistRunState(s)
	return previous
}
