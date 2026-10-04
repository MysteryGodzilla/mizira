// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/alexschlessinger/pollytool/messages"
	"github.com/alexschlessinger/pollytool/tools"

	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// forceToolTimeout bounds the forced call; a slow backend falls back to the ordinary path.
const forceToolTimeout = 30 * time.Second

// forceIntentTool runs the tool a message plainly asks for (irc.ToolIntent) without leaving the
// decision to the model: one direct request whose reply is constrained to the tool's arguments,
// then the tool runs here, with all its own checks. It returns the call and its result to add to
// the conversation, or nil when there is no such intent or anything fails, so the request carries
// on exactly as without it.
func forceIntentTool(ctx irc.ChatContextInterface, req *CompletionRequest, msg string) []messages.ChatMessage {
	registry := ctx.GetSystem().GetToolRegistry()
	if registry == nil {
		return nil
	}
	channel := ctx.GetConfig().Server.Channel
	inChannel := func(nick string) bool {
		var nicks []string
		for _, u := range ctx.GetChannelUsers(channel) {
			nicks = append(nicks, u.Nick)
		}
		return irc.NickInList(nick, nicks)
	}
	intent, ok := irc.ToolIntent(ctx.GetConfig(), ctx.GetBotNick(), msg, inChannel)
	if !ok {
		return nil
	}
	name := intent.Tool
	// Muting someone on request is an admin's call; anyone else's request stays the model's to
	// weigh, or one user could silence another through the bot (A11).
	if name == irc.ClaimTool[irc.ClaimIgnore] && !ctx.IsAdmin() {
		return nil
	}
	tool, ok := registry.Get(name)
	if !ok {
		return nil
	}

	log := ctx.GetLogger()
	var call messages.ChatMessageToolCall
	var err error
	if intent.Complete {
		args, _ := json.Marshal(intent.Args)
		call = messages.ChatMessageToolCall{ID: "forced-" + ctx.GetRequestID(), Name: name, Arguments: string(args)}
	} else if call, err = requestToolCall(ctx, tool, msg); err != nil {
		log.Warn("forced_tool_unavailable", "tool", name, "error", err.Error())
		return nil
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(call.Arguments), &args); err != nil {
		log.Warn("forced_tool_bad_arguments", "tool", name, "error", err.Error())
		return nil
	}
	// What the message settled (who to ignore) is not the model's to choose.
	if len(intent.Args) > 0 {
		for k, v := range intent.Args {
			args[k] = v
		}
		fixed, _ := json.Marshal(args)
		call.Arguments = string(fixed)
	}

	if ctx.GetConfig().Bot.ShowToolActions && !silentTools[name] {
		ctx.ReplyAction("calling " + toolDisplayName(name))
	}
	log.Info("tool_started", "tool", name, "forced", true)
	runCtx, cancel := context.WithTimeout(irc.InjectContext(ctx, ctx), ctx.GetConfig().API.Timeout)
	defer cancel()
	result, err := tool.Execute(runCtx, args)
	if err != nil {
		log.Warn("forced_tool_failed", "tool", name, "error", err.Error())
		return nil
	}
	log.Info("tool_completed", "tool", name, "forced", true, "preview", truncateForLog(result))

	if isToolRefusal(result) || refusedResult(result) {
		score := core.Suspicions().Add(ctx.GetNetwork(), ctx.SpeakerKey(), core.SignalToolRefused)
		log.Warn("tool_refused_request", "tool", name, "source", ctx.GetSource(), "suspicion", score)
		call.Arguments = `{"redacted":"refused by a safety check"}`
	}
	return []messages.ChatMessage{
		{Role: messages.MessageRoleAssistant, ToolCalls: []messages.ChatMessageToolCall{call}},
		{Role: messages.MessageRoleTool, Content: result, ToolCallID: call.ID, ToolName: call.Name},
	}
}

// requestToolCall asks the model for the tool's arguments, seeing the system prompt and the new
// message. It uses a JSON-schema response format rather than tool_choice: llama.cpp enforces the
// schema as a grammar, while Gemma 4 there answered tool_choice "required" with plain text.
func requestToolCall(ctx irc.ChatContextInterface, tool tools.Tool, msg string) (messages.ChatMessageToolCall, error) {
	cfg := ctx.GetConfig()
	var call messages.ChatMessageToolCall
	base := strings.TrimSuffix(cfg.API.OpenAIURL, "/")
	if base == "" || strings.TrimSpace(msg) == "" {
		return call, errors.New("no openai endpoint")
	}

	// Only the person's own message and the tool: the arguments must come from what they said. With
	// the system prompt (room memory, recap) and the turn's injected memories in view, the model
	// once filled a remember with a room fact instead of the speaker's words.
	msgs := []map[string]string{
		{"role": "system", "content": tool.GetName() + ": " + tool.GetSchema().Description()},
		{"role": "user", "content": msg},
	}

	s := tool.GetSchema()
	params := map[string]any{"type": "object", "properties": s.Properties()}
	if required := s.Required(); len(required) > 0 {
		params["required"] = required
	}
	body, err := json.Marshal(map[string]any{
		"model":    modelNameOnly(cfg.Model.Model),
		"messages": msgs,
		"response_format": map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": tool.GetName(), "schema": params},
		},
		"max_tokens":       256,
		"temperature":      0,
		"reasoning_effort": "none",
	})
	if err != nil {
		return call, err
	}

	rctx, cancel := context.WithTimeout(ctx, forceToolTimeout)
	defer cancel()
	hreq, err := http.NewRequestWithContext(rctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return call, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	if cfg.API.OpenAIKey != "" {
		hreq.Header.Set("Authorization", "Bearer "+cfg.API.OpenAIKey)
	}
	resp, err := core.ModelPost(hreq, forceToolTimeout)
	if err != nil {
		return call, err
	}
	defer resp.Body.Close()

	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return call, err
	}
	if len(payload.Choices) == 0 {
		return call, fmt.Errorf("empty reply (http %s)", resp.Status)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(payload.Choices[0].Message.Content), &args); err != nil {
		return call, fmt.Errorf("arguments are not a json object: %w", err)
	}
	for _, key := range s.Required() {
		if _, ok := args[key]; !ok {
			return call, fmt.Errorf("arguments miss %q", key)
		}
	}
	compact, err := json.Marshal(args)
	if err != nil {
		return call, err
	}
	return messages.ChatMessageToolCall{ID: "forced-" + ctx.GetRequestID(), Name: tool.GetName(), Arguments: string(compact)}, nil
}

// forcedToolsRun lists the tools already run for the newest user message, by the forced path.
func forcedToolsRun(history []messages.ChatMessage) []string {
	var names []string
	for i := len(history) - 1; i >= 0 && history[i].Role != messages.MessageRoleUser; i-- {
		if history[i].Role == messages.MessageRoleTool {
			names = append(names, history[i].ToolName)
		}
	}
	return names
}
