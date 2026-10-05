// Copyright (C) 2023-2026 Alex Schlessinger and soulshack contributors
// Modified 2026 by BareMetal
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"errors"
	"fmt"
	"strings"

	"B4reMetal/metald/internal/irc"
)

// SetCommand handles the +set command for configuration changes
type SetCommand struct{}

func (c *SetCommand) Name() string    { return "+set" }
func (c *SetCommand) AdminOnly() bool { return true }

func (c *SetCommand) Execute(ctx irc.ChatContextInterface) {
	keys := getConfigKeys()
	if len(ctx.GetArgs()) < 3 {
		ctx.Reply(fmt.Sprintf("Usage: +set <key> <value>. Available keys: %s", strings.Join(keys, ", ")))
		return
	}

	param, v := ctx.GetArgs()[1], ctx.GetArgs()[2:]
	value := strings.Join(v, " ")
	cfg := ctx.GetConfig()

	ctx.GetLogger().Debug("config_change_requested", "param", param, "value", value)

	// Handle standard config fields
	field, ok := configFields[param]
	if !ok {
		ctx.Reply(fmt.Sprintf("Unknown key. Available keys: %s", strings.Join(keys, ", ")))
		return
	}

	// Shared with the operator console's Settings page.
	if err := applySetting(cfg, ctx.GetSystem(), param, value, ctx.GetLogger()); errors.Is(err, ErrLLMNotUpdated) {
		ctx.Reply("Configuration saved, but failed to update LLM client")
	} else if err != nil {
		ctx.Reply(err.Error())
		return
	}

	ctx.Reply(fmt.Sprintf("%s set to: %s", param, field.getter(cfg)))
	if ClearsHistory(param) {
		ctx.GetSession().Clear()
	}
}
