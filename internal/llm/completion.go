// Copyright (C) 2023-2026 Alex Schlessinger and soulshack contributors
// Modified 2026 by BareMetal
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"fmt"
	"github.com/alexschlessinger/pollytool/llm"
	"github.com/alexschlessinger/pollytool/messages"
	"github.com/alexschlessinger/pollytool/sessions"
	"github.com/alexschlessinger/pollytool/tools"
	"strings"
	"sync"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// maxLoggedMessage bounds how much of an inbound message reaches the log.
const maxLoggedMessage = 4000

type CompletionRequest = llm.CompletionRequest

func NewCompletionRequest(config *config.Configuration, session sessions.Session, tools []tools.Tool) *CompletionRequest {
	thinkingEffort, err := llm.ParseThinkingEffort(config.Model.ThinkingEffort)
	if err != nil {
		// An unrecognised value is passed THROUGH, not dropped.
		if raw := strings.TrimSpace(config.Model.ThinkingEffort); raw != "" {
			thinkingEffort = llm.ThinkingEffort(raw)
			core.GetLogger().Debug("thinking_effort_passthrough",
				"value", raw, "note", "not a protocol level; backend decides")
		} else {
			core.GetLogger().Warn("invalid_thinking_effort_config",
				"value", config.Model.ThinkingEffort, "error", err.Error())
		}
	}

	req := &CompletionRequest{
		BaseURL:        config.API.OpenAIURL,
		Timeout:        config.API.Timeout,
		Model:          config.Model.Model,
		MaxTokens:      config.Model.MaxTokens,
		Messages:       session.GetHistory(),
		Temperature:    llm.Float32Ptr(config.Model.Temperature),
		Tools:          tools,
		ThinkingEffort: thinkingEffort,
	}
	applySampling(req, config.Model.Sampling)

	// Set streaming mode (nil = streaming default, false = non-streaming)
	if !config.Model.Stream {
		stream := false
		req.Stream = &stream
	}

	return req
}

// maxInjectedMemories bounds what goes into every request.
const maxInjectedMemories = 12

// recallForSpeaker builds the memory block for the current speaker, or "" if
// there is nothing to say.
func recallForSpeaker(ctx irc.ChatContextInterface) string {
	source := ctx.GetSource()
	if source == "" {
		return ""
	}

	store, err := core.Memories()
	if err != nil {
		ctx.GetLogger().Warn("memory_unavailable_for_injection", "error", err.Error())
		return ""
	}

	mems, err := store.Recall(ctx.GetNetwork(), source, maxInjectedMemories)
	if err != nil {
		ctx.GetLogger().Warn("memory_injection_failed", "error", err.Error())
		return ""
	}
	if len(mems) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(strings.ReplaceAll(strings.TrimSpace(ctx.GetConfig().Bot.MemoryFrame), "{nick}", source))
	for _, m := range mems {
		fmt.Fprintf(&b, "\n  - %s", m.Fact)
	}

	ctx.GetLogger().Debug("memories_injected", "source", source, "count", len(mems))
	return b.String()
}

// Complete processes a user message and returns a channel of response chunks.
func Complete(ctx irc.ChatContextInterface, msg string) (<-chan string, error) {
	// Screen non-admin messages BEFORE the session sees them.
	if allowed, reason := ScreenIncoming(ctx, msg); !allowed {
		var score float64
		if reason != screenUnavailable {
			score = core.Suspicions().Add(ctx.GetNetwork(), ctx.GetSource(), core.SignalScreenDenied)
		}
		ctx.GetLogger().Info("message_screened_out",
			"source", ctx.GetSource(), "reason", reason, "suspicion", score)
		out := make(chan string, 1)
		out <- ctx.GetConfig().Bot.ScreenRefusal
		close(out)
		return out, nil
	}

	// Add user message to session
	cmsg := messages.ChatMessage{
		Role:    messages.MessageRoleUser,
		Content: msg,
	}
	logged := msg
	if len(logged) > maxLoggedMessage {
		logged = logged[:maxLoggedMessage] + fmt.Sprintf("...[truncated, %d bytes total]", len(msg))
	}
	ctx.GetLogger().Info("message_received", "message", logged)

	// The user message is held back and committed with its answer as one block
	// (commitExchange), so overlapping requests never interleave in history.
	session := ctx.GetSession()
	cfg := ctx.GetConfig()
	sys := ctx.GetSystem()

	// A channel running a user-supplied persona gets no tools at all.
	var allTools []tools.Tool
	restricted := core.Prompts().Active(ctx.GetLockKey())
	if sys.GetToolRegistry() != nil && !restricted {
		allTools = sys.GetToolRegistry().All()
	}
	if restricted {
		ctx.GetLogger().Debug("tools_withheld_custom_prompt", "channel", ctx.GetLockKey())
	}

	req := NewCompletionRequest(cfg, session, allTools)
	req.Messages = append(req.Messages, cmsg)
	setPending(req, cmsg)

	// Inject what the bot already knows about the speaker, so memories reach the model without
	// it having to call memory__recall.
	if !restricted {
		if mem := recallForSpeaker(ctx); mem != "" {
			if len(req.Messages) > 0 && req.Messages[0].Role == messages.MessageRoleSystem {
				req.Messages[0].Content += "\n\n" + mem
			} else {
				req.Messages = append([]messages.ChatMessage{{
					Role:    messages.MessageRoleSystem,
					Content: mem,
				}}, req.Messages...)
			}
		}
	}

	// T23: a plain "remember …" or "ignore bob" runs its tool before the model replies.
	if !restricted {
		if forced := forceIntentTool(ctx, req, msg); len(forced) > 0 {
			req.Messages = append(req.Messages, forced...)
			setPending(req, append([]messages.ChatMessage{cmsg}, forced...)...)
		}
	}

	// Get response stream from LLM
	stream := sys.GetLLM().ChatCompletionStream(ctx, req)

	output := make(chan string, 10)

	go func() {
		defer close(output)
		// If the backend did not commit the exchange, still record the question.
		defer func() { commitExchange(session, takePending(req)) }()

		// The ordinary path: pass chunks through as they arrive.
		if !outboundScreened(ctx) {
			for chunk := range stream {
				output <- chunk
			}
			return
		}

		// The filtered path: hold the whole reply, look at it, then decide.
		var lines []string
		for chunk := range stream {
			lines = append(lines, chunk)
		}
		reply := strings.Join(lines, "\n")

		allowed, reason := ScreenOutgoing(ctx, reply)
		if allowed {
			for _, line := range lines {
				output <- line
			}
			return
		}

		// Neither a failed check nor a custom persona's doing is the speaker's fault.
		var score float64
		if reason != core.ClassifyUnavailable && !core.Prompts().Active(ctx.GetLockKey()) {
			score = core.Suspicions().Add(ctx.GetNetwork(), ctx.GetSource(), core.SignalReplyDenied)
		}
		ctx.GetLogger().Warn("reply_screened_out",
			"source", ctx.GetSource(), "reason", reason,
			"suspicion", score, "reply", truncate(reply, maxLoggedMessage))

		// The reply is already in the session by now - polly adds the agent's messages before this
		// channel closes.
		if n := core.QuarantineSpeaker(ctx.GetSession(), ctx.GetSource()); n > 0 {
			ctx.GetLogger().Warn("exchange_quarantined",
				"source", ctx.GetSource(), "messages", n, "cause", "reply_screened")
		}

		output <- ctx.GetConfig().Bot.ScreenRefusal
	}()

	return output, nil
}

// outboundScreened reports whether this speaker's replies get the extra
// outbound check.
func outboundScreened(ctx irc.ChatContextInterface) bool {
	// A custom persona is when the rules are most likely to bend (one talked the bot into romance
	// live), so while one is active every reply is checked, whoever asked.
	return core.Prompts().Active(ctx.GetLockKey()) || isScreened(ctx, ctx.GetConfig().Bot.FilterNicks)
}

// pending holds each in-flight request's own user message, and any tool call forced for it,
// until the exchange is committed.
var pending sync.Map

func setPending(req *CompletionRequest, msgs ...messages.ChatMessage) { pending.Store(req, msgs) }

func takePending(req *CompletionRequest) []messages.ChatMessage {
	v, ok := pending.LoadAndDelete(req)
	if !ok {
		return nil
	}
	return v.([]messages.ChatMessage)
}

// commitExchange appends a finished request's messages to the session as
// one contiguous block, in completion order.
func commitExchange(session sessions.Session, msgs []messages.ChatMessage) {
	if session == nil || len(msgs) == 0 {
		return
	}
	mu := core.CommitLock(session)
	mu.Lock()
	defer mu.Unlock()
	for _, m := range msgs {
		session.AddMessage(m)
	}
}

// applySampling puts the set sampling values on the request. top_p and presence_penalty are OpenAI
// parameters; the rest are understood by OpenAI-compatible servers such as llama.cpp.
func applySampling(req *CompletionRequest, sampling map[string]float64) {
	for key, v := range sampling {
		switch key {
		case "top_p":
			req.TopP = &v
		case "presence_penalty":
			req.PresencePenalty = &v
		case "top_k", "dry_allowed_length", "dry_penalty_last_n":
			req.ExtraBody = withExtra(req.ExtraBody, key, int(v))
		default:
			req.ExtraBody = withExtra(req.ExtraBody, key, v)
		}
	}
}

func withExtra(extra map[string]any, key string, v any) map[string]any {
	if extra == nil {
		extra = map[string]any{}
	}
	extra[key] = v
	return extra
}
