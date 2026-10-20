// Copyright (C) 2026 BareMetal
// Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"context"
	"maps"

	"github.com/alexschlessinger/pollytool/tools"
)

// RequesterTool passes the nick that asked for a tool call to the tool as "requested_by", and
// "requested_by_admin" when that nick is an admin, for tools that credit, record or cap who asked.
// Both come from the IRC context, never from the model: whatever the model wrote is replaced.
type RequesterTool struct {
	tools.Tool
}

// NewRequesterTool wraps tool so each call carries the asking nick.
func NewRequesterTool(tool tools.Tool) *RequesterTool {
	return &RequesterTool{Tool: tool}
}

func (r *RequesterTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	args = maps.Clone(args)
	if args == nil {
		args = map[string]any{}
	}
	delete(args, "requested_by")
	delete(args, "requested_by_admin")
	if chatCtx, err := GetIRCContext(ctx); err == nil {
		args["requested_by"] = chatCtx.GetSource()
		if chatCtx.IsAdmin() {
			args["requested_by_admin"] = true
		}
	}
	return r.Tool.Execute(ctx, args)
}
