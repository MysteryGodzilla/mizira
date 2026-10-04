// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"strings"
	"testing"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

// Room memory - facts about the bot and the channel - rides in the system prompt of every request,
// and a custom persona doesn't get it.
func TestRoomMemoryInSystemPrompt(t *testing.T) {
	mockSys := mocktest.NewMockSystem()
	llmMock := &mocktest.MockLLM{Responses: []string{"ok", "ok"}}
	mockSys.LLM = llmMock
	session, _ := mockSys.SessionStore.Get("room/#room")
	ctx := mocktest.NewMockContext().WithSystem(mockSys).WithSession(session).WithSource("alice")
	cfg := ctx.GetConfig()
	cfg.Bot.Trigger = "botty"
	mem, _ := core.Memories()
	_, _ = mem.Remember(ctx.GetNetwork(), "botty", "botty is a night owl", "admin", cfg.Server.Channel)
	_, _ = mem.Remember(ctx.GetNetwork(), cfg.Server.Channel, "the channel runs a weekly music quiz", "admin", cfg.Server.Channel)
	t.Cleanup(func() {
		mem.ForgetSubject(ctx.GetNetwork(), "botty")
		mem.ForgetSubject(ctx.GetNetwork(), cfg.Server.Channel)
	})

	out, _ := Complete(ctx, "(nick:alice) botty hi")
	for range out {
	}
	sys := llmMock.LastRequest().Messages[0]
	if !strings.Contains(sys.Content, "botty is a night owl") || !strings.Contains(sys.Content, "weekly music quiz") {
		t.Fatalf("room memory missing from the system prompt: %q", sys.Content)
	}

	core.Prompts().Set(ctx.GetLockKey(), "you are a pirate", "alice")
	t.Cleanup(func() { core.Prompts().Clear(ctx.GetLockKey()) })
	out, _ = Complete(ctx, "(nick:alice) botty hi again")
	for range out {
	}
	for _, m := range llmMock.LastRequest().Messages {
		if strings.Contains(m.Content, "night owl") {
			t.Fatal("a custom persona must not get room memory")
		}
	}
}

// With a trigger set, the bot's nick is its owner's: what's remembered about the owner still comes
// up when someone else mentions them.
func TestRelevantKeepsOwnerSharingTheBotsNick(t *testing.T) {
	ctx := mocktest.NewMockContext().WithSource("alice")
	ctx.GetConfig().Bot.Trigger = "botty"
	mem, _ := core.Memories()
	_, _ = mem.Remember(ctx.GetNetwork(), ctx.GetBotNick(), ctx.GetBotNick()+" collects vintage synthesizers", "bob", "#chat")
	t.Cleanup(func() { mem.ForgetSubject(ctx.GetNetwork(), ctx.GetBotNick()) })

	got := relevantMemories(ctx, "botty what does "+ctx.GetBotNick()+" collect?", func(string) bool { return false })
	if !strings.Contains(got, "vintage synthesizers") {
		t.Errorf("owner's memory left out: %q", got)
	}
}
