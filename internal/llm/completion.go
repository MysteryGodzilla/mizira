// Copyright (C) 2023-2026 Alex Schlessinger and soulshack contributors
// Modified 2026 by BareMetal
// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"cmp"
	"encoding/json"
	"fmt"
	"github.com/alexschlessinger/pollytool/llm"
	"github.com/alexschlessinger/pollytool/messages"
	"github.com/alexschlessinger/pollytool/sessions"
	"github.com/alexschlessinger/pollytool/tools"
	"maps"
	"strings"
	"sync"
	"time"
	"unicode"

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
	applyThinking(req, thinkingEffort)

	// Set streaming mode (nil = streaming default, false = non-streaming)
	if !config.Model.Stream {
		stream := false
		req.Stream = &stream
	}

	return req
}

// maxInjectedMemories bounds what goes into every request.
const maxInjectedMemories = 12

// maxRelevantMemories bounds the facts about other people and topics the message brings up.
const maxRelevantMemories = 8

// maxBacklogLine bounds one channel line handed to the model.
const maxBacklogLine = 300

// recallForSpeaker builds the memory block for the current speaker, or "" if
// there is nothing to say.
func recallForSpeaker(ctx irc.ChatContextInterface, known func(string) bool) string {
	source := ctx.Speaker()
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
	mems = unseen(mems, known)
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

// unseen drops the memories the conversation already carries, so a fact is sent once rather than with
// every turn. One the recap folded away is no longer in the conversation, and is sent again.
func unseen(mems []core.Memory, known func(string) bool) []core.Memory {
	var out []core.Memory
	for _, m := range mems {
		if !known(m.Fact) {
			out = append(out, m)
		}
	}
	return out
}

// conversationText is everything the user turns of a conversation said, for checking what it
// already carries.
func conversationText(history []messages.ChatMessage) string {
	var b strings.Builder
	for _, m := range history {
		if m.Role == messages.MessageRoleUser {
			b.WriteString(m.GetContent())
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// relevantMemories builds the block of remembered facts that text brings up, beyond the speaker's own.
func relevantMemories(ctx irc.ChatContextInterface, text string, known func(string) bool) string {
	store, err := core.Memories()
	if err != nil {
		return ""
	}
	exclude := []string{ctx.Speaker(), ctx.GetConfig().Server.Channel}
	if t := ctx.GetConfig().Bot.Trigger; t != "" {
		exclude = append(exclude, t)
	} else {
		exclude = append(exclude, ctx.GetBotNick())
	}
	mems, err := store.Relevant(ctx.GetNetwork(), text, exclude, maxRelevantMemories)
	if err != nil {
		ctx.GetLogger().Warn("relevant_memories_failed", "error", err.Error())
		return ""
	}
	mems = unseen(mems, known)
	if len(mems) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(ctx.GetConfig().Bot.RelevantFrame))
	for _, m := range mems {
		fmt.Fprintf(&b, "\n  - about %s: %s", m.Subject, m.Fact)
	}
	ctx.GetLogger().Debug("relevant_memories_injected", "count", len(mems))
	return b.String()
}

// recapBlock is the channel's running summary of older conversation, framed, or "".
func recapBlock(ctx irc.ChatContextInterface) string {
	db, err := core.Context()
	if err != nil {
		return ""
	}
	recap := db.Recap(ctx.GetSession().GetName())
	if recap == "" {
		return ""
	}
	return strings.TrimSpace(ctx.GetConfig().Bot.RecapFrame) + "\n" + recap
}

// maxRoomChars bounds the room memory block.
const maxRoomChars = 1500

// roomBlock is room memory: what an operator has said about the channel and about the bot itself,
// framed, newest first within roommemorylimit and maxRoomChars, or "".
func roomBlock(ctx irc.ChatContextInterface) string {
	cfg := ctx.GetConfig()
	limit := cfg.Bot.RoomMemoryLimit
	if limit <= 0 || ctx.IsPrivate() {
		return ""
	}
	store, err := core.Memories()
	if err != nil {
		return ""
	}
	self := cfg.Bot.Trigger
	if self == "" {
		self = ctx.GetBotNick()
	}
	var mems []core.Memory
	for _, subject := range []string{self, cfg.Server.Channel} {
		ms, err := store.Recall(ctx.GetNetwork(), subject, limit)
		if err != nil {
			ctx.GetLogger().Warn("room_memory_failed", "error", err.Error())
			return ""
		}
		mems = append(mems, ms...)
	}
	if len(mems) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(strings.ReplaceAll(strings.TrimSpace(cfg.Bot.RoomMemoryFrame), "{channel}", cfg.Server.Channel))
	for i, m := range mems {
		line := fmt.Sprintf("\n  - %s", m.Fact)
		if i >= limit || b.Len()+len(line) > maxRoomChars {
			break
		}
		b.WriteString(line)
	}
	return b.String()
}

// appendSystem adds a block to the request's system prompt, or starts one.
func appendSystem(req *CompletionRequest, block string) {
	if len(req.Messages) > 0 && req.Messages[0].Role == messages.MessageRoleSystem {
		req.Messages[0].Content += "\n\n" + block
		return
	}
	req.Messages = append([]messages.ChatMessage{{Role: messages.MessageRoleSystem, Content: block}}, req.Messages...)
}

// backlogBlock takes the channel lines said since the bot last answered here, framed as quoted chat,
// or "". Lines the quoted-chat check refuses are left out.
func backlogBlock(ctx irc.ChatContextInterface) (block, plain string) {
	cfg := ctx.GetConfig()
	if cfg.Session.Backlog <= 0 || ctx.IsPrivate() {
		return "", ""
	}
	lines := screenBacklog(ctx, core.Backlog().Take(ctx.GetSession().GetName(), cfg.Session.BacklogWindow, cfg.Session.Backlog))
	if len(lines) == 0 {
		return "", ""
	}
	var b, p strings.Builder
	b.WriteString(strings.TrimSpace(cfg.Bot.BacklogFrame))
	for _, l := range lines {
		text := truncate(l.Text, maxBacklogLine)
		if l.Action {
			fmt.Fprintf(&b, "\n[%s] * %s %s", l.At.Format("15:04"), l.Nick, text)
		} else {
			fmt.Fprintf(&b, "\n[%s] <%s> %s", l.At.Format("15:04"), l.Nick, text)
		}
		p.WriteString(text + "\n")
	}
	ctx.GetLogger().Debug("backlog_injected", "lines", len(lines))
	return b.String(), p.String()
}

// Complete processes a user message and returns a channel of response chunks.
func Complete(ctx irc.ChatContextInterface, msg string) (<-chan string, error) {
	// Screen non-admin messages BEFORE the session sees them.
	if allowed, reason := ScreenIncoming(ctx, msg); !allowed {
		var score float64
		if reason != screenUnavailable {
			score = core.Suspicions().Add(ctx.GetNetwork(), ctx.SpeakerKey(), core.SignalScreenDenied)
		}
		ctx.GetLogger().Info("message_screened_out",
			"source", ctx.GetSource(), "reason", reason, "suspicion", score)
		ignoreHarasser(ctx, reason)
		out := make(chan string, 1)
		out <- ctx.GetConfig().Bot.ScreenRefusal
		close(out)
		return out, nil
	}

	// Channel lines the bot was not addressed in go after the speaker's own line, so the turn
	// still starts with its "(nick:x)" prefix.
	backlog, backlogText := backlogBlock(ctx)
	content := msg
	if backlog != "" {
		content = msg + "\n\n" + backlog
	}

	// What the bot knows about the speaker, and about whatever the message brings up, rides on the
	// turn after the speaker's words and is saved with it, so none of it depends on the model calling
	// memory__recall. Saving the turn exactly as sent means the next request's prompt starts with
	// this one's, and the backend resumes from its cache instead of reprocessing the conversation.
	if !core.Prompts().Active(ctx.GetLockKey()) {
		carried := conversationText(ctx.GetSession().GetHistory())
		inConversation := func(fact string) bool { return strings.Contains(carried, fact) }
		var known []string
		if mem := recallForSpeaker(ctx, inConversation); mem != "" {
			known = append(known, mem)
		}
		if rel := relevantMemories(ctx, msg+"\n"+backlogText, inConversation); rel != "" {
			known = append(known, rel)
		}
		if len(known) > 0 {
			content += "\n\n" + strings.Join(known, "\n\n")
		}
	}

	// Add user message to session
	cmsg := messages.ChatMessage{
		Role:    messages.MessageRoleUser,
		Content: content,
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

	// Room memory changes only when an operator writes it, so like the recap it sits in the system
	// prompt. A custom persona doesn't get it: it describes Mizira, not the persona.
	if !restricted {
		if r := roomBlock(ctx); r != "" {
			appendSystem(req, r)
		}
	}

	// The recap changes only when history is folded, which rewrites history anyway, so it costs the
	// cache nothing in the system prompt.
	if r := recapBlock(ctx); r != "" {
		appendSystem(req, r)
	}

	// T23: a plain "remember …" or "ignore bob" runs its tool before the model replies.
	if !restricted {
		if forced := forceIntentTool(ctx, req, msg); len(forced) > 0 {
			req.Messages = append(req.Messages, forced...)
			setPending(req, append([]messages.ChatMessage{cmsg}, forced...)...)
		}
	}

	vouchConversation(ctx.GetRequestID(), req.Messages)

	// Get response stream from LLM
	stream := sys.GetLLM().ChatCompletionStream(ctx, req)

	output := make(chan string, 10)

	go func() {
		defer close(output)
		// If the backend did not commit the exchange, still record the question.
		defer func() {
			commitExchange(session, takePending(req))
			MaybeFold(cfg, session)
		}()
		defer postedActions.Delete(ctx.GetRequestID())
		defer forgetLinks(ctx.GetRequestID())

		// The ordinary path: pass chunks through as they arrive.
		if !outboundScreened(ctx) {
			for chunk := range stream {
				if echoesAction(ctx, chunk) || dividerLine(chunk) {
					continue
				}
				if chunk, ok := guardLinks(ctx, chunk); ok {
					output <- chunk
				}
			}
			return
		}

		// The filtered path: hold the whole reply, look at it, then decide.
		var lines []string
		for chunk := range stream {
			if echoesAction(ctx, chunk) || dividerLine(chunk) {
				continue
			}
			if chunk, ok := guardLinks(ctx, chunk); ok {
				lines = append(lines, chunk)
			}
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
		if reason != core.ClassifyUnavailable && reason != core.ClassifyInconclusive && !core.Prompts().Active(ctx.GetLockKey()) {
			score = core.Suspicions().Add(ctx.GetNetwork(), ctx.SpeakerKey(), core.SignalReplyDenied)
		}
		ctx.GetLogger().Warn("reply_screened_out",
			"source", ctx.GetSource(), "reason", reason,
			"suspicion", score, "reply", truncate(reply, maxLoggedMessage))

		// The reply is already in the session by now - polly adds the agent's messages before this
		// channel closes. It becomes what was posted instead; the speaker's words stay, since the
		// reply was hers. If it can't be found, the whole exchange goes as before.
		if !core.ReplaceReply(ctx.GetSession(), reply, ctx.GetConfig().Bot.ScreenRefusal) {
			if n := core.QuarantineSpeaker(ctx.GetSession(), ctx.SpeakerKey()); n > 0 {
				ctx.GetLogger().Warn("exchange_quarantined",
					"source", ctx.GetSource(), "messages", n, "cause", "reply_screened")
			}
		}

		output <- ctx.GetConfig().Bot.ScreenRefusal
	}()

	return output, nil
}

// logReplies adds the bot's answer to the searchable chat log.
func logReplies(ctx irc.ChatContextInterface, msgs []messages.ChatMessage) {
	if ctx.GetConfig().Session.HistoryDays <= 0 || ctx.IsPrivate() {
		return
	}
	db, err := core.Context()
	if err != nil {
		return
	}
	// Logged under the trigger word where there is one: the bot's nick may be an operator's account.
	nick := ctx.GetConfig().Bot.Trigger
	if nick == "" {
		nick = ctx.GetBotNick()
	}
	key, now := ctx.GetSession().GetName(), time.Now()
	for _, m := range msgs {
		if m.Role != messages.MessageRoleAssistant {
			continue
		}
		var lines []string
		for _, l := range strings.Split(m.GetContent(), "\n") {
			if l = irc.CleanReplyLine(l); l != "" {
				lines = append(lines, l)
			}
		}
		if len(lines) > 0 {
			if err := db.LogLine(key, nick, strings.Join(lines, "\n"), now); err != nil {
				ctx.GetLogger().Warn("chatlog_write_failed", "error", err.Error())
				return
			}
		}
	}
}

// outboundScreened reports whether this speaker's replies get the extra
// outbound check.
func outboundScreened(ctx irc.ChatContextInterface) bool {
	// A custom persona is when the rules are most likely to bend (one talked the bot into romance
	// live), so while one is active every reply is checked, whoever asked.
	return core.Prompts().Active(ctx.GetLockKey()) || isScreened(ctx, config.List(&ctx.GetConfig().Bot.FilterNicks))
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
// one contiguous block, in completion order, with bulky tool traffic trimmed.
func commitExchange(session sessions.Session, msgs []messages.ChatMessage) {
	if session == nil || len(msgs) == 0 {
		return
	}
	mu := core.CommitLock(session)
	mu.Lock()
	defer mu.Unlock()
	now := time.Now().Unix()
	for _, m := range msgs {
		session.AddMessage(stampTime(trimForHistory(m), now))
	}
}

// stampTime records when a message entered history, for the operator page. Metadata is never sent
// to the model, so the prompt and its cache are unchanged.
func stampTime(m messages.ChatMessage, at int64) messages.ChatMessage {
	md := maps.Clone(m.Metadata)
	if md == nil {
		md = map[string]any{}
	}
	md[core.MessageTimeKey] = at
	m.Metadata = md
	return m
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

// thinkingHeadroom is added to maxtokens while she thinks: the thinking comes out of the same budget,
// and at 600 tokens Qwen spent all of it thinking and replied with nothing.
var thinkingHeadroom = map[llm.ThinkingEffort]int{llm.ThinkingLow: 1024, llm.ThinkingMedium: 2048, llm.ThinkingHigh: 4096}

// applyThinking switches thinking on or off for a chat reply. llama.cpp's Gemma 4 and Qwen templates
// read enable_thinking and ignore reasoning_effort; "none" is off, as "off" is.
func applyThinking(req *CompletionRequest, effort llm.ThinkingEffort) {
	on := effort.IsEnabled() && effort != "none"
	req.ExtraBody = withExtra(req.ExtraBody, "chat_template_kwargs", map[string]any{"enable_thinking": on})
	if on {
		req.MaxTokens += cmp.Or(thinkingHeadroom[effort], thinkingHeadroom[llm.ThinkingMedium])
	}
}

func withExtra(extra map[string]any, key string, v any) map[string]any {
	if extra == nil {
		extra = map[string]any{}
	}
	extra[key] = v
	return extra
}

// Past the request that made them, a tool call's arguments and a tool's result only need to say what
// happened: a whole file sent to the paste tool, or a whole fetched page, would otherwise ride along
// in every later prompt. Trimming them is safe for the prompt cache, which resumes from the end of
// the user turn before them.
const (
	historyArgsMax   = 1500
	historyResultMax = 4000
)

func trimForHistory(m messages.ChatMessage) messages.ChatMessage {
	// Reasoning is never sent back to the model, and the operator page has already seen it live; kept,
	// it would only swell the saved history and fold it sooner.
	m.Reasoning = ""
	if m.Role == messages.MessageRoleTool && len(m.Content) > historyResultMax {
		m.Content = m.Content[:historyResultMax] + fmt.Sprintf("\n... [%d more characters, trimmed from history]", len(m.Content)-historyResultMax)
	}
	if len(m.ToolCalls) > 0 {
		calls := make([]messages.ChatMessageToolCall, len(m.ToolCalls))
		copy(calls, m.ToolCalls)
		for i := range calls {
			if a := calls[i].Arguments; len(a) > historyArgsMax {
				calls[i].Arguments = trimmedArgs(a)
			}
		}
		m.ToolCalls = calls
	}
	return m
}

// trimmedArgs keeps a tool call's arguments valid JSON: long string values are cut, the rest kept.
func trimmedArgs(raw string) string {
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		out, _ := json.Marshal(map[string]string{"trimmed": raw[:historyArgsMax/2] + "..."})
		return string(out)
	}
	for k, v := range args {
		if s, ok := v.(string); ok && len(s) > 300 {
			args[k] = s[:300] + fmt.Sprintf("... [%d more characters, trimmed from history]", len(s)-300)
		}
	}
	out, _ := json.Marshal(args)
	return string(out)
}

// postedActions holds the /me lines a request's tools posted, so the model's
// reply cannot post the same thing again as text.
var postedActions sync.Map

func recordAction(requestID, action string) {
	v, _ := postedActions.LoadOrStore(requestID, &[]string{})
	list := v.(*[]string)
	*list = append(*list, action)
}

// stems lowercases s, splits it into words and drops a plural "s", so
// "slap" and "slaps" count as the same word.
func stems(s string) []string {
	ws := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for i, w := range ws {
		if len(w) > 3 && strings.HasSuffix(w, "s") {
			ws[i] = w[:len(w)-1]
		}
	}
	return ws
}

// echoesAction reports whether a reply line only restates a /me this request
// already posted: it covers at least 70% of the action's words and adds at
// most a few of its own.
func echoesAction(ctx irc.ChatContextInterface, line string) bool {
	v, ok := postedActions.Load(ctx.GetRequestID())
	if !ok {
		return false
	}
	lw := stems(line)
	if len(lw) < 3 {
		return false
	}
	for _, action := range *v.(*[]string) {
		aw := make(map[string]bool)
		for _, w := range stems(action) {
			aw[w] = true
		}
		seen := make(map[string]bool)
		extra := 0
		for _, w := range lw {
			if aw[w] {
				seen[w] = true
			} else {
				extra++
			}
		}
		if len(seen)*10 >= len(aw)*7 && extra <= max(2, len(lw)/3) {
			ctx.GetLogger().Debug("action_echo_dropped", "line", line)
			return true
		}
	}
	return false
}

// dividerLine reports whether a reply line is only a Markdown divider such as
// "* * *" or "---", which means nothing on IRC.
func dividerLine(line string) bool {
	t := strings.TrimSpace(line)
	return t != "" && strings.Trim(t, "*-_=~# ") == ""
}
