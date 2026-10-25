// Copyright (C) 2023-2026 Alex Schlessinger and soulshack contributors
// Modified 2026 by BareMetal
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/alexschlessinger/pollytool/llm"
	"github.com/alexschlessinger/pollytool/messages"
	"github.com/alexschlessinger/pollytool/tools"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// PollyLLM wraps pollytool's MultiPass and Agent to implement metald's LLM interface
type PollyLLM struct {
	client llm.LLM
}

// NewPollyLLM creates a new pollytool-based LLM client
func NewPollyLLM(config config.APIConfig) *PollyLLM {
	apiKeys := map[string]string{
		"openai":    config.OpenAIKey,
		"anthropic": config.AnthropicKey,
		"gemini":    config.GeminiKey,
		"ollama":    config.OllamaKey,
	}
	return &PollyLLM{client: stableToolOrder{llm.NewMultiPass(apiKeys)}}
}

// ChatCompletionStream returns a channel of string chunks for IRC output
func (p *PollyLLM) ChatCompletionStream(chatCtx core.ChatContextInterface, req *CompletionRequest) <-chan string {
	cfg := chatCtx.GetConfig()
	// Only apply OllamaURL for ollama/ models
	if strings.HasPrefix(req.Model, "ollama/") && cfg.API.OllamaURL != "" {
		req.BaseURL = cfg.API.OllamaURL
	}

	maxChunkSize := 400
	if cfg.Session.ChunkMax > 0 {
		maxChunkSize = cfg.Session.ChunkMax
	}

	output := make(chan string, 10)

	go func() {
		defer close(output)

		// A nil registry is what denies tools under a custom prompt: the model is never told they
		// exist, and a tool call emitted from memory cannot resolve.
		registry := toolView(chatCtx.GetSystem().GetToolRegistry(), scopeOf(chatCtx.GetSession().GetName()))
		if core.Prompts().Active(chatCtx.GetLockKey()) {
			registry = nil
			chatCtx.GetLogger().Debug("tool_registry_withheld_custom_prompt")
		}

		iterations := cfg.Session.MaxIterations
		if iterations <= 0 {
			iterations = 10
		}
		agent := llm.NewAgent(p.client, registry, llm.AgentConfig{
			MaxIterations: iterations,
			ToolTimeout:   cfg.API.Timeout,
		})

		chunker := irc.NewChunker(output, maxChunkSize)
		chunker.SetMaxLines(cfg.Bot.MaxReplyLines)
		chunker.SetJoinLines(true)
		claimTools := claimableTools(registry)
		newHandler := func() *callbackHandler {
			h := newCallbackHandler(chatCtx, chunker, cfg)
			h.watchClaims(claimTools)
			for _, name := range forcedToolsRun(req.Messages) {
				h.toolsRun[name] = true
			}
			return h
		}
		cb := newHandler()

		resp, err := agent.Run(chatCtx, req, cb.build())

		// A tool call written as text never ran: ask once more, telling the model the exact tool
		// names. Only when no tool has run yet, so a retry cannot repeat one. The note is not
		// committed; only the retry's answer is.
		retried := false
		if err == nil && cb.leaked && cb.toolCount == 0 && registry != nil && chatCtx.Err() == nil {
			retried = true
			chatCtx.GetLogger().Warn("tool_syntax_retry")
			retry := *req
			retry.Messages = append(append([]messages.ChatMessage(nil), req.Messages...), messages.ChatMessage{
				Role:    messages.MessageRoleUser,
				Content: toolRetryNote(cfg.Bot.ToolRetryNote, registry),
			})
			cb = newHandler()
			resp, err = agent.Run(chatCtx, &retry, cb.build())
		}

		// A turn that ended with nothing to show - no text, no tool, no link - usually thought itself
		// out and never wrote the answer. Ask once for the answer itself.
		if err == nil && cb.silent() && !retried && chatCtx.Err() == nil {
			retried = true
			chatCtx.GetLogger().Warn("empty_reply_retry")
			retry := *req
			retry.Messages = append(append([]messages.ChatMessage(nil), req.Messages...), messages.ChatMessage{
				Role:    messages.MessageRoleUser,
				Content: strings.TrimSpace(cfg.Bot.EmptyReplyNote),
			})
			cb = newHandler()
			resp, err = agent.Run(chatCtx, &retry, cb.build())
		}

		// Flush only if this request is still wanted.
		if err := chatCtx.Err(); errors.Is(err, context.Canceled) {
			// Cancelled (+reset, an ignore): the exchange never happened.
			takePending(req)
			chatCtx.GetLogger().Debug("output_discarded_cancelled")
			return
		}

		// Out of iterations is not a backend failure: keep what was said and the work done so far.
		if err != nil && err.Error() == "max iterations exceeded" && resp != nil {
			chatCtx.GetLogger().Warn("agent_max_iterations", "iterations", resp.IterationCount)
			cb.flush()
			commitExchange(chatCtx.GetSession(),
				append(takePending(req), redactRefusedArguments(resp.AllMessages)...))
			logReplies(chatCtx, resp.AllMessages)
			if !cb.hadContent {
				output <- outOfSteps
			}
			return
		}

		// A run that failed does not get to finish its sentence.
		if err != nil {
			chatCtx.GetLogger().Error("agent_error", "error", err.Error())
			held := cb.discard()
			queued := drainQueued(output)
			if held > 0 || queued > 0 {
				chatCtx.GetLogger().Warn("dead_request_output_voided",
					"buffered_bytes", held, "queued_lines", queued)
			}
			// Keep the question so the conversation still shows it was asked.
			commitExchange(chatCtx.GetSession(), takePending(req))
			output <- genericBackendError
			return
		}

		cb.flush()
		reply := resp.AllMessages
		if cb.looped {
			reply = withReplyText(reply, cb.loopKept)
		}
		if kinds := cb.unbackedClaims(); len(kinds) > 0 && alreadyDone(chunker.HeldText(), req.Messages, kinds) {
			chatCtx.GetLogger().Info("claim_backed_earlier", "claims", kinds)
			chunker.Release()
		} else if len(kinds) > 0 {
			reply = retryUnbackedClaim(chatCtx, agent, req, chunker, claimTools, reply, kinds)
			if errors.Is(chatCtx.Err(), context.Canceled) {
				takePending(req)
				chatCtx.GetLogger().Debug("output_discarded_cancelled")
				return
			}
		}
		if chunker.Truncated() {
			// Not a suspicion signal: asking for a long story is normal, not an attack.
			chatCtx.GetLogger().Info("reply_line_limit", "limit", cfg.Bot.MaxReplyLines)
		}

		commitExchange(chatCtx.GetSession(),
			append(takePending(req), redactRefusedArguments(reply)...))
		logReplies(chatCtx, reply)
		MaybeFold(cfg, chatCtx.GetSession())

		// Still nothing after the retry: say so rather than leave the channel waiting.
		if cb.silent() {
			chatCtx.GetLogger().Warn("empty_reply_posted_notice")
			output <- noAnswer
		}

		// An admin's turns are never dropped: a refused memory of theirs is a test, not an attack.
		source := chatCtx.SpeakerKey()
		if score := core.Suspicions().Score(chatCtx.GetNetwork(), source); score >= core.SuspicionQuarantine && !chatCtx.IsAdmin() {
			if n := core.QuarantineSpeaker(chatCtx.GetSession(), source); n > 0 {
				chatCtx.GetLogger().Warn("exchange_quarantined",
					"source", source, "messages", n,
					"suspicion", score, "cause", "score_threshold")
			}
			core.Suspicions().Discount(chatCtx.GetNetwork(), source, core.SuspicionQuarantine)
		}
	}()

	return output
}

// refusedResult reports whether a tool result is a safety refusal.
func refusedResult(content string) bool {
	c := strings.ToLower(strings.TrimSpace(content))
	return strings.HasPrefix(c, "refused") || strings.HasPrefix(c, "error: refused")
}

// redactRefusedArguments strips the arguments of any tool call that was refused, before the turn is
// written to the session.
func redactRefusedArguments(msgs []messages.ChatMessage) []messages.ChatMessage {
	refused := map[string]bool{}
	for _, m := range msgs {
		if m.ToolCallID != "" && refusedResult(m.GetContent()) {
			refused[m.ToolCallID] = true
		}
	}
	if len(refused) == 0 {
		return msgs
	}

	out := make([]messages.ChatMessage, len(msgs))
	copy(out, msgs)
	for i := range out {
		if len(out[i].ToolCalls) == 0 {
			continue
		}
		calls := make([]messages.ChatMessageToolCall, len(out[i].ToolCalls))
		copy(calls, out[i].ToolCalls)
		for j := range calls {
			if refused[calls[j].ID] {
				calls[j].Arguments = `{"redacted":"refused by a safety check"}`
			}
		}
		out[i].ToolCalls = calls
	}
	return out
}

// callbackHandler organizes callback construction
type callbackHandler struct {
	chatCtx          core.ChatContextInterface
	chunker          *irc.Chunker
	cfg              *config.Configuration
	startTime        time.Time
	lastThinkingTime time.Time
	toolCount        int
	announcedTools   map[string]bool
	refusals         map[string]int      // refusals per tool this request, to stop a refusal loop
	lastToolURL      string              // most recent "url: ..." a successful tool call handed back this request
	drawFailed       string              // why the last picture call this request made nothing, or ""
	hadContent       bool                // whether the model's final reply ever wrote any actual content
	leaked           bool                // this turn emitted tool-call syntax; suppress the rest
	leakedBuf        string              // rolling tail, so a marker split across chunks is still seen
	reasoning        irc.ReasoningFilter // strips think blocks the model wrote into content
	reasoningBlocks  int                 // last logged value of reasoning.Blocks
	reasoningStray   int                 // last logged value of reasoning.Stray
	contentBytes     int                 // visible content this turn has streamed
	overBudget       bool                // budget spent; suppress the rest of the turn
	visible          strings.Builder     // the reply's text so far, for the loop guard
	looped           bool                // the reply fell into a phrase loop; suppress the rest
	loopKept         string              // what the reply said before the loop
	claimTools       map[string]bool     // tools a claim is checked against; nil checks nothing
	toolsRun         map[string]bool     // tools started this request
	claims           map[irc.ClaimKind]bool
	askedPicture     bool // the message asks for a picture, so a link in the reply must be one drawn now
	shown            bool // the channel saw a tool's effect: its start line, its own /me, or its action
	repeats          int  // calls this request to a once-per-request tool after its first
}

// maxTurnContent bounds how much visible text ONE request may post.
const maxTurnContent = 2000

func newCallbackHandler(chatCtx core.ChatContextInterface, chunker *irc.Chunker, cfg *config.Configuration) *callbackHandler {
	return &callbackHandler{
		chatCtx:        chatCtx,
		chunker:        chunker,
		cfg:            cfg,
		startTime:      time.Now(),
		announcedTools: make(map[string]bool),
		toolsRun:       make(map[string]bool),
		claims:         make(map[irc.ClaimKind]bool),
		refusals:       make(map[string]int),
	}
}

// watchClaims makes the chunker hold back a line claiming an action whose tool hasn't run yet.
func (h *callbackHandler) watchClaims(claimTools map[string]bool) {
	h.claimTools = claimTools
	h.askedPicture = irc.AsksForPicture(strings.Join(h.chatCtx.GetArgs(), " "))
	h.chunker.SetHold(h.holdClaim)
}

func (h *callbackHandler) holdClaim(line string) bool {
	kind, ok := irc.DetectClaim(line)
	// Asked for a picture, she twice answered with a link another bot had posted earlier.
	if !ok && h.askedPicture && irc.HasLink(line) {
		kind, ok = irc.ClaimDraw, true
	}
	if !ok {
		return false
	}
	tool := irc.ClaimTool[kind]
	if !h.claimTools[tool] || h.toolsRun[tool] {
		return false
	}
	h.claims[kind] = true
	return true
}

// unbackedClaims lists the claims made this request whose tool never ran.
func (h *callbackHandler) unbackedClaims() []irc.ClaimKind {
	var out []irc.ClaimKind
	for kind := range h.claims {
		if !h.toolsRun[irc.ClaimTool[kind]] {
			out = append(out, kind)
		}
	}
	slices.Sort(out)
	return out
}

func (h *callbackHandler) build() *llm.AgentCallbacks {
	return &llm.AgentCallbacks{
		OnReasoning:       h.onReasoning,
		OnContent:         h.onContent,
		BeforeToolExecute: h.beforeToolExecute,
		OnToolStart:       h.onToolStart,
		ApproveToolCalls:  h.approveToolCalls,
		OnToolEnd:         h.onToolEnd,
		OnComplete:        h.onComplete,
		OnError:           h.onError,
	}
}

func (h *callbackHandler) onComplete(response *messages.ChatMessage) {
	duration := time.Since(h.startTime)
	inputTokens := response.GetInputTokens()
	outputTokens := response.GetOutputTokens()

	fields := []any{"duration_ms", duration.Milliseconds()}
	if inputTokens > 0 || outputTokens > 0 {
		fields = append(fields, "input_tokens", inputTokens, "output_tokens", outputTokens)
	}
	if h.toolCount > 0 {
		fields = append(fields, "tool_count", h.toolCount)
	}
	h.chatCtx.GetLogger().Info("request_complete", fields...)

	// A tool that made something answers "url: ..."; if the reply didn't post that link, the channel
	// would never see what was made.
	if h.lastToolURL != "" && (!h.hadContent || h.overBudget || !strings.Contains(h.visible.String(), h.lastToolURL)) {
		h.chatCtx.GetLogger().Warn("reply_missing_tool_url", "url", h.lastToolURL)
		h.overBudget = false // let the url itself through
		// Its own line, not the end of the reply's last one; the chunker drops the empty line otherwise.
		h.chunker.Write("\n" + h.lastToolURL + "\n")
	}
	// A picture call that made nothing wasn't always said: she posted the last picture's link as the
	// new one, and people had to ask or check the console.
	if h.drawFailed != "" && h.lastToolURL == "" {
		h.chatCtx.GetLogger().Info("picture_failure_shown", "reason", h.drawFailed)
		h.overBudget = false
		h.chunker.Write("\n(no picture this time: " + h.drawFailed + ")\n")
	}
}

func (h *callbackHandler) onReasoning(content string) {
	h.chatCtx.GetLogger().Debug("reasoning_chunk", "content", content)
	core.LiveThinking.Publish(core.LiveEvent{Request: h.chatCtx.GetRequestID(), Network: h.chatCtx.GetNetwork(),
		Source: h.chatCtx.GetSource(), Target: h.chatCtx.GetTarget(), Text: content})

	if !h.cfg.Bot.ShowThinkingAction {
		return
	}

	now := time.Now()
	if now.Sub(h.startTime) > 30*time.Second {
		if h.lastThinkingTime.IsZero() || now.Sub(h.lastThinkingTime) > 30*time.Second {
			elapsed := now.Sub(h.startTime).Round(time.Second)
			h.chatCtx.ReplyAction(fmt.Sprintf("is thinking... (%s)", elapsed))
			h.lastThinkingTime = now
		}
	}
}

func (h *callbackHandler) onContent(content string) {
	h.chatCtx.GetLogger().Debug("oncontent_callback",
		"content", content,
		"content_len", len(content),
	)

	h.leakedBuf += content
	if !h.leaked && irc.LooksLikeToolCall(h.leakedBuf) {
		h.leaked = true
		score := core.Suspicions().Add(h.chatCtx.GetNetwork(), h.chatCtx.SpeakerKey(), core.SignalToolSyntax)
		h.chatCtx.GetLogger().Warn("tool_syntax_latched",
			"preview", truncateForLog(h.leakedBuf), "suspicion", score)
	}
	if h.leaked {
		return
	}
	// Only the tail can begin a split marker, so the buffer stays bounded.
	if len(h.leakedBuf) > 256 {
		h.leakedBuf = h.leakedBuf[len(h.leakedBuf)-256:]
	}

	content = h.reasoning.Feed(content)
	if h.reasoning.Blocks != h.reasoningBlocks || h.reasoning.Stray != h.reasoningStray {
		h.reasoningBlocks, h.reasoningStray = h.reasoning.Blocks, h.reasoning.Stray
		h.chatCtx.GetLogger().Warn("reasoning_in_content_stripped",
			"blocks", h.reasoning.Blocks, "stray_closers", h.reasoning.Stray)
	}

	if h.looped || h.guardLoop(content) {
		return
	}

	// Budget latch.
	if h.overBudget {
		return
	}
	h.contentBytes += len(content)
	if h.contentBytes > maxTurnContent {
		h.overBudget = true
		score := core.Suspicions().Add(h.chatCtx.GetNetwork(), h.chatCtx.SpeakerKey(), core.SignalRunaway)
		h.chatCtx.GetLogger().Warn("turn_content_budget_exceeded",
			"bytes", h.contentBytes, "limit", maxTurnContent,
			"preview", truncateForLog(content), "suspicion", score)
		return
	}

	if content != "" {
		h.hadContent = true
	}
	h.chunker.Write(content)
}

// guardLoop watches the reply for a phrase loop. When one starts it latches like the budget does:
// what was buffered is dropped, and unless lines already went out, only the text up to the first
// copy of the looping phrase is sent. The clean text is kept so history never holds the loop, which the model would copy.
func (h *callbackHandler) guardLoop(content string) bool {
	h.visible.WriteString(content)
	text := h.visible.String()
	off, ok := loopStart(text)
	if !ok {
		return false
	}
	h.looped = true
	h.loopKept = strings.TrimSpace(text[:off])
	h.chunker.Discard()
	if h.chunker.Sent() == 0 {
		h.hadContent = true
		h.chunker.Write(h.loopKept)
	}
	h.chatCtx.GetLogger().Warn("reply_loop_cut",
		"kept", truncateForLog(h.loopKept), "loop", truncateForLog(text[off:]))
	return true
}

// withReplyText swaps the text of the closing assistant message, the reply the channel saw.
func withReplyText(msgs []messages.ChatMessage, text string) []messages.ChatMessage {
	out := slices.Clone(msgs)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i].Role == messages.MessageRoleAssistant && len(out[i].ToolCalls) == 0 {
			out[i].Content = text
			break
		}
	}
	return out
}

// discard drops everything a dead request was still holding and returns how much it dropped.
func (h *callbackHandler) discard() int {
	h.reasoning.Flush()
	return h.chunker.Discard()
}

// flush releases anything the reasoning filter was holding back, then flushes the chunker.
func (h *callbackHandler) flush() {
	if tail := h.reasoning.Flush(); tail != "" {
		h.hadContent = true
		h.chunker.Write(tail)
	}
	h.chunker.Flush()
}

func truncateForLog(s string) string {
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

func (h *callbackHandler) beforeToolExecute(ctx context.Context, tc messages.ChatMessageToolCall, args map[string]any) context.Context {
	return irc.InjectContext(ctx, h.chatCtx)
}

// silentTools are not announced with "calling X": their own /me action is what the channel sees.
var silentTools = map[string]bool{"irc__action": true, "irc__slap": true}

// toolDisplayName is what the channel sees in "calling X".
var toolDisplayOverrides = map[string]string{
	"musicgen__song":    "musicgen",
	"imagegen__picture": "imagegen",
}

func toolDisplayName(name string) string {
	if d, ok := toolDisplayOverrides[name]; ok {
		return d
	}
	if idx := strings.Index(name, "__"); idx != -1 {
		return name[idx+2:]
	}
	return name
}

func (h *callbackHandler) onToolStart(calls []messages.ChatMessageToolCall) {
	h.flush()
	h.correctToolNames(calls)
	if n := mergePictureCalls(calls, irc.ClaimTool[irc.ClaimDraw]); n > 0 {
		h.chatCtx.GetLogger().Info("picture_calls_merged", "merged", n)
	}

	h.toolCount += len(calls)
	for _, tc := range calls {
		h.toolsRun[tc.Name] = true
	}
	if h.chunker.Holding() && len(h.unbackedClaims()) == 0 {
		h.chunker.Release()
	}

	// Log each tool
	for _, tc := range calls {
		h.chatCtx.GetLogger().Info("tool_started", "tool", tc.Name)
	}

	if len(calls) == 0 {
		return
	}
	if !h.cfg.Bot.ShowToolActions {
		h.shown = true // with no start lines, a tool that ran counts as an answer, as it always has
		return
	}

	// Filter and format tool names, skipping ones already announced this request - a retried tool call
	// (e.g. after a failed safety check) shouldn't re-announce "calling X" a second time.
	var names []string
	for _, tc := range calls {
		if silentTools[tc.Name] || core.QuietTool(tc.Name) {
			h.shown = true // its own effect is what the channel sees
			continue
		}
		if !h.toolExists(tc.Name) || h.refusedHere(tc.Name) {
			continue
		}
		displayName := toolDisplayName(tc.Name)
		if h.announcedTools[displayName] {
			continue
		}
		h.announcedTools[displayName] = true
		h.shown = true
		if out, ok := core.Announcement(tc.Name, tc.Arguments); ok {
			if action, ok := toolAction(out); ok {
				h.chatCtx.ReplyAction(action)
				continue
			}
		}
		names = append(names, displayName)
	}

	if len(names) > 0 {
		h.chatCtx.ReplyAction(fmt.Sprintf("calling %s", strings.Join(names, ", ")))
	}
}

func (h *callbackHandler) onToolEnd(tc messages.ChatMessageToolCall, result string, duration time.Duration, toolErr error) {
	if toolErr != nil {
		h.chatCtx.GetLogger().Error("tool_failed",
			"tool", tc.Name,
			"duration_ms", duration.Milliseconds(),
			"error", toolErr.Error(),
		)
		return
	}

	// A tool refusing what it was handed is the strongest single signal available.
	if isToolRefusal(result) {
		score := core.Suspicions().Add(h.chatCtx.GetNetwork(), h.chatCtx.SpeakerKey(), core.SignalToolRefused)
		h.chatCtx.GetLogger().Warn("tool_refused_request",
			"tool", tc.Name, "source", h.chatCtx.GetSource(), "suspicion", score)
	}

	preview := result
	if len(preview) > 60 && !h.cfg.Bot.Verbose {
		preview = preview[:60] + "..."
	}
	h.chatCtx.GetLogger().Info("tool_completed",
		"tool", tc.Name,
		"duration_ms", duration.Milliseconds(),
		"result_size", len(result),
		"preview", preview,
	)

	if strings.HasPrefix(result, irc.OnceRepeated) {
		h.repeats++
	}

	// A plugin posts a /me by starting its result with "action: ".
	if action, ok := toolAction(result); ok {
		h.shown = true
		h.flush()
		h.chatCtx.ReplyAction(action)
		recordAction(h.chatCtx.GetRequestID(), action)
	}

	if isToolRefusal(result) {
		h.refusals[tc.Name]++
	}

	// The reply may link to what this tool returned.
	learnOwnHosts(result)
	vouchLinks(h.chatCtx.GetRequestID(), result)

	// Remember the last url a tool returned, as a fallback if the model's final reply is empty.
	firstLine, made := strings.CutPrefix(result, "url: ")
	if made {
		if nl := strings.IndexByte(firstLine, '\n'); nl != -1 {
			firstLine = firstLine[:nl]
		}
		h.lastToolURL = strings.TrimSpace(firstLine)
	}
	if tc.Name == irc.ClaimTool[irc.ClaimDraw] && !strings.HasPrefix(result, irc.OnceRepeated) && !strings.HasPrefix(result, irc.OnceMerged) {
		h.drawFailed = ""
		if !made {
			h.drawFailed = failureReason(result)
		}
	}
}

// failureReason is the first sentence of a tool's failure ("Not done: outfit restyles an existing
// picture. Call again with ..."), without its label: the part a person can read.
func failureReason(result string) string {
	reason := strings.TrimSpace(result)
	// "Error", "Not done", "Limit reached".
	if label, rest, ok := strings.Cut(reason, ": "); ok && len(label) <= 14 {
		reason = rest
	}
	if end := strings.IndexAny(reason, "\n"); end != -1 {
		reason = reason[:end]
	}
	if end := strings.Index(reason, ". "); end != -1 {
		reason = reason[:end]
	}
	reason = strings.TrimSuffix(strings.TrimSpace(reason), ".")
	if r := []rune(reason); len(r) > 160 {
		reason = string(r[:160]) + "..."
	}
	return reason
}

// maxToolRefusals is how often one tool may refuse a request before the model stops being allowed
// to call it; past that it only burns iterations asking again.
const maxToolRefusals = 2

// maxRepeats is how often a request may call a once-per-request tool again before it may call no
// tool at all.
const maxRepeats = 2

// approveToolCalls denies calls to a tool that has already refused this request twice.
func (h *callbackHandler) approveToolCalls(calls []messages.ChatMessageToolCall) []bool {
	ok := make([]bool, len(calls))
	// Told twice it had already run a once-per-request tool, the model called it again and again
	// until it ran out of steps, posting half-replies on the way. No more tools this request.
	if h.repeats >= maxRepeats {
		h.chatCtx.GetLogger().Warn("tool_calls_denied_after_repeats", "repeats", h.repeats)
		return ok
	}
	for i, tc := range calls {
		ok[i] = h.refusals[tc.Name] < maxToolRefusals
		if !ok[i] {
			h.chatCtx.GetLogger().Warn("tool_call_denied_after_refusals", "tool", tc.Name, "refusals", h.refusals[tc.Name])
		}
	}
	return ok
}

// drainQueued empties a buffered channel without blocking, returning how many items it threw away.
func drainQueued(ch chan string) int {
	n := 0
	for {
		select {
		case <-ch:
			n++
		default:
			return n
		}
	}
}

// isToolRefusal reports whether a tool result is a refusal rather than an ordinary failure.
func isToolRefusal(result string) bool {
	return strings.HasPrefix(strings.TrimSpace(strings.ToLower(result)), "error: refused")
}

// noAnswer is what the channel sees when the model twice finished without writing anything.
const noAnswer = "i went quiet on that one - thought it through and never wrote an answer. ask again, maybe more specifically."

// silent reports whether this turn produced nothing the channel saw: no text, no tool effect, no
// fallback link. A tool that only refused unseen (admin-only, for someone else) shows nothing.
func (h *callbackHandler) silent() bool {
	return !h.hadContent && !h.shown && h.lastToolURL == ""
}

// outOfSteps is what the channel sees when a request used every tool iteration without answering.
const outOfSteps = "that took more steps than i'm allowed and i didn't get to an answer. try asking for less at once."

// genericBackendError is what the channel sees when the LLM backend fails.
const genericBackendError = "something went wrong talking to my backend. try again in a moment."

func (h *callbackHandler) onError(err error) {
	h.chatCtx.GetLogger().Error("stream_error", "error", err.Error())
}

// CreateAgentForRegistry creates an agent with the given registry for external use
func CreateAgentForRegistry(client *llm.MultiPass, registry *tools.ToolRegistry, timeout time.Duration) *llm.Agent {
	return llm.NewAgent(client, registry, llm.AgentConfig{
		MaxIterations: 10,
		ToolTimeout:   timeout,
	})
}

const maxToolAction = 300

// toolAction extracts the /me text from a tool result whose first line starts
// with "action: ", stripped of control characters and capped in length.
func toolAction(result string) (string, bool) {
	first, _, _ := strings.Cut(result, "\n")
	text, ok := strings.CutPrefix(first, "action: ")
	if !ok {
		return "", false
	}
	text = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, text)
	text = strings.TrimSpace(text)
	if len(text) > maxToolAction {
		text = strings.TrimSpace(text[:maxToolAction])
	}
	return text, text != ""
}

// toolExists reports whether a call names a registered tool; a misspelled call
// fails anyway and must not be announced.
func (h *callbackHandler) toolExists(name string) bool {
	sys := h.chatCtx.GetSystem()
	if sys == nil || sys.GetToolRegistry() == nil {
		return true
	}
	_, ok := sys.GetToolRegistry().Get(name)
	return ok
}

// refusedHere reports whether the tool will only refuse this caller (admin-only for someone who
// isn't, people-only for another bot): announcing it would say something is happening when nothing is.
func (h *callbackHandler) refusedHere(name string) bool {
	sys := h.chatCtx.GetSystem()
	if sys == nil || sys.GetToolRegistry() == nil {
		return false
	}
	tool, ok := sys.GetToolRegistry().Get(name)
	return ok && irc.RefusedFor(tool, h.chatCtx)
}
