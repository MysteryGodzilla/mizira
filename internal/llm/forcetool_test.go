// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alexschlessinger/pollytool/messages"
	"github.com/alexschlessinger/pollytool/schema"
	"github.com/alexschlessinger/pollytool/tools"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

// forceSetup wires a fake remember tool returning result, and a model server answering with reply.
func forceSetup(t *testing.T, reply, result string) (*mocktest.MockChatContext, *map[string]any, *map[string]any) {
	t.Helper()
	sent := map[string]any{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &sent)
		w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)

	ran := map[string]any{}
	remember := &tools.Func{
		Name:     "memory__remember",
		Desc:     "Store a fact",
		Params:   schema.Params{"subject": schema.S("who"), "fact": schema.S("what")},
		Required: []string{"subject", "fact"},
		Run: func(_ context.Context, args tools.Args) (string, error) {
			for k, v := range args {
				ran[k] = v
			}
			return result, nil
		},
	}
	sys := mocktest.NewMockSystem()
	sys.ToolRegistry = tools.NewToolRegistry([]tools.Tool{remember})
	ctx := mocktest.NewMockContext().WithSystem(sys)
	ctx.GetConfig().API.OpenAIURL = srv.URL
	ctx.GetConfig().Model.Model = "openai/chat"
	return ctx, &sent, &ran
}

const rememberCall = `{"choices":[{"message":{"content":"{\"subject\":\"dave\",\"fact\":\"hates mornings\"}"}}]}`

func forceRequest(msg string) *CompletionRequest {
	return &CompletionRequest{Messages: []messages.ChatMessage{
		{Role: messages.MessageRoleSystem, Content: "you are a bot"},
		{Role: messages.MessageRoleUser, Content: msg},
	}}
}

func TestForceIntentToolRunsTheTool(t *testing.T) {
	ctx, sent, ran := forceSetup(t, rememberCall, "Remembered about dave: hates mornings.")
	got := forceIntentTool(ctx, forceRequest("(nick:alice) metald remember that dave hates mornings"),
		"(nick:alice) metald remember that dave hates mornings")

	if len(got) != 2 || got[0].ToolCalls[0].Name != "memory__remember" || got[1].ToolCallID != got[0].ToolCalls[0].ID {
		t.Fatalf("want the call and its result, got %+v", got)
	}
	if (*ran)["fact"] != "hates mornings" {
		t.Errorf("tool ran with %v", *ran)
	}
	format, _ := (*sent)["response_format"].(map[string]any)
	if format["type"] != "json_schema" || (*sent)["tools"] != nil {
		t.Errorf("request must constrain the reply to the arguments: %v", *sent)
	}
	if names := forcedToolsRun(append(forceRequest("x").Messages, got...)); len(names) != 1 || names[0] != "memory__remember" {
		t.Errorf("forcedToolsRun = %v", names)
	}
}

func TestForceIntentToolRedactsRefusedArguments(t *testing.T) {
	ctx, _, _ := forceSetup(t, rememberCall, "Refused: not storing that (address).")
	got := forceIntentTool(ctx, forceRequest("x"), "(nick:alice) metald remember bob lives at 12 Example Street")
	if len(got) != 2 || strings.Contains(got[0].ToolCalls[0].Arguments, "mornings") {
		t.Fatalf("refused arguments must not reach history: %+v", got)
	}
}

func TestForceIntentToolFallsBack(t *testing.T) {
	cases := map[string]string{
		"no intent": "(nick:alice) metald how are you",
		"not json":  "(nick:alice) metald remember that dave hates mornings",
	}
	for name, msg := range cases {
		ctx, _, ran := forceSetup(t, `{"choices":[{"message":{"content":"ok"}}]}`, "Remembered.")
		if got := forceIntentTool(ctx, forceRequest(msg), msg); got != nil || len(*ran) != 0 {
			t.Errorf("%s: want nil and no tool run, got %+v", name, got)
		}
	}
}

func TestForceIntentToolNeedsRequiredArguments(t *testing.T) {
	ctx, _, ran := forceSetup(t, `{"choices":[{"message":{"content":"{\"subject\":\"dave\"}"}}]}`, "Remembered.")
	msg := "(nick:alice) metald remember that dave hates mornings"
	if got := forceIntentTool(ctx, forceRequest(msg), msg); got != nil || len(*ran) != 0 {
		t.Fatalf("missing fact must fall back, got %+v", got)
	}
}

// A non-admin asking to mute someone is left to the model; an admin's request is forced.
func TestForcedIgnoreIsAdminOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"nick\":\"metald\",\"minutes\":5,\"reason\":\"asked\"}"}}]}`))
	}))
	defer srv.Close()

	for _, admin := range []bool{false, true} {
		var target any
		ignore := &tools.Func{
			Name:     "irc__ignore",
			Desc:     "Stop responding to a user",
			Params:   schema.Params{"nick": schema.S("who"), "minutes": schema.Int("how long"), "reason": schema.S("why")},
			Required: []string{"nick", "minutes", "reason"},
			Run: func(_ context.Context, args tools.Args) (string, error) {
				target = args["nick"]
				return "Now ignoring bob.", nil
			},
		}
		sys := mocktest.NewMockSystem()
		sys.ToolRegistry = tools.NewToolRegistry([]tools.Tool{ignore})
		ctx := mocktest.NewMockContext().WithSystem(sys).WithAdmin(admin)
		ctx.GetConfig().API.OpenAIURL = srv.URL
		ctx.ChannelUsers[ctx.GetConfig().Server.Channel] = []core.ChannelUser{{Nick: "bob"}}

		msg := "(nick:alice) metald ignore bob, he's annoying"
		got := forceIntentTool(ctx, forceRequest(msg), msg)
		if !admin && got != nil {
			t.Errorf("non-admin request was forced: %+v", got)
		}
		// The model answered with the bot's own nick; the message's target wins.
		if admin && (got == nil || target != "bob") {
			t.Errorf("admin request: got %+v, ignored %v", got, target)
		}
	}
}

// Red-team 2026-10-04: with the system prompt and the turn's injected memories in view, a forced
// remember was filled from a room fact. The request carries only the tool and the speaker's words.
func TestForcedCallSeesOnlyTheMessage(t *testing.T) {
	ctx, sent, _ := forceSetup(t, rememberCall, "Remembered about dave: hates mornings.")
	req := &CompletionRequest{Messages: []messages.ChatMessage{
		{Role: messages.MessageRoleSystem, Content: "you are a bot\n\nfacts about you: you are a night owl"},
		{Role: messages.MessageRoleUser, Content: "(nick:alice) metald remember that dave hates mornings\n\nother things you remember: carol likes jazz"},
	}}
	forceIntentTool(ctx, req, "(nick:alice) metald remember that dave hates mornings")

	msgs, _ := (*sent)["messages"].([]any)
	raw, _ := json.Marshal(msgs)
	if len(msgs) != 2 || strings.Contains(string(raw), "night owl") || strings.Contains(string(raw), "carol likes jazz") ||
		!strings.Contains(string(raw), "dave hates mornings") {
		t.Errorf("forced call saw more than the message: %s", raw)
	}
}

// A search query is written with the recent chat in view, so "yes search it" knows what "it" is;
// other tools see none of it.
func TestRecentForSearch(t *testing.T) {
	req := &CompletionRequest{}
	req.Messages = []messages.ChatMessage{
		{Role: messages.MessageRoleSystem, Content: "system prompt"},
		{Role: messages.MessageRoleUser, Content: "(nick:bob) mizira unicorn gundam!"},
		{Role: messages.MessageRoleAssistant, Content: "oh, the Unicorn Gundam?"},
		{Role: messages.MessageRoleTool, Content: "tool output"},
		{Role: messages.MessageRoleUser, Content: "(nick:alice) mizira yes search it"},
	}
	got := recentFor(true, req)
	want := "them: (nick:bob) mizira unicorn gundam!\nyou: oh, the Unicorn Gundam?"
	if got != want {
		t.Errorf("recent = %q, want %q", got, want)
	}
	if recentFor(false, req) != "" {
		t.Error("a non-search tool was given the chat")
	}
}
