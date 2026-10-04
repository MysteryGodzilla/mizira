// Copyright (C) 2026 BareMetal
// Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"fmt"
	"log/slog"

	"github.com/alexschlessinger/pollytool/sessions"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// ResetCommand handles +reset: wipes this channel's conversation history and makes the channel's
// request lock immediately acquirable again.
type ResetCommand struct{}

func (c *ResetCommand) Name() string    { return "+reset" }
func (c *ResetCommand) AdminOnly() bool { return false }

// restoreDefaultModel puts the model back to whatever config.yml names, undoing a "+models" switch.
func restoreDefaultModel(cfg *config.Configuration, sys core.System, by string, log *slog.Logger) string {
	def := DefaultModel()
	if def == "" {
		// ApplyOverrides never ran.
		return ""
	}

	configMu.Lock()
	defer configMu.Unlock()
	if cfg.Model.Model == def {
		return ""
	}

	previous := cfg.Model.Model
	if err := configFields["model"].setter(cfg, def); err != nil {
		log.Error("model_restore_failed", "error", err.Error())
		return ""
	}
	if sys != nil {
		if err := sys.UpdateLLM(*cfg.API); err != nil {
			log.Error("llm_update_failed", "error", err.Error())
		}
	}
	PersistUnset("model")

	log.Info("model_restored_to_default", "from", previous, "to", def, "by", by)
	return modelNameOnly(def)
}

// restoreOperatorPrompt undoes a "+prompt" persona, putting the operator's prompt back and re-
// enabling tools.
func restoreOperatorPrompt(cfg *config.Configuration, session sessions.Session, lockKey, by string, log *slog.Logger) bool {
	if !core.Prompts().Clear(lockKey) {
		return false
	}

	metadata := session.GetMetadata()
	metadata.SystemPrompt = cfg.Bot.Prompt
	session.SetMetadata(metadata)

	log.Info("prompt_override_cleared", "channel", lockKey, "by", by)
	return true
}

func (c *ResetCommand) Execute(ctx irc.ChatContextInterface) {
	r := ResetConversation(ctx.GetConfig(), ctx.GetSystem(), ctx.GetSession(), ctx.GetLockKey(),
		ctx.GetRequestID(), ctx.GetSource(), ctx.GetLogger())
	personaCleared, restored, cancelled := r.PersonaCleared, r.ModelRestored, r.Cancelled

	suffix := ""
	if personaCleared {
		suffix += " Custom persona removed, tools re-enabled."
	}
	if restored != "" {
		suffix += fmt.Sprintf(" Model restored to %s.", restored)
	}

	if cancelled > 0 {
		ctx.Reply(fmt.Sprintf("Reset: conversation history cleared, %d in-flight request(s) cancelled.%s", cancelled, suffix))
		return
	}
	ctx.Reply("Reset: conversation history cleared and pending requests released." + suffix)
}
