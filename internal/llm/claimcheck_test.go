// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"slices"
	"testing"

	"github.com/alexschlessinger/pollytool/messages"

	"B4reMetal/metald/internal/irc"
	mocktest "B4reMetal/metald/internal/testing"
)

func claimHandler(claimTools map[string]bool) (*callbackHandler, *irc.Chunker, chan string) {
	ctx := mocktest.NewMockContext()
	out := make(chan string, 10)
	chunker := irc.NewChunker(out, 350)
	h := newCallbackHandler(ctx, chunker, ctx.GetConfig())
	h.watchClaims(claimTools)
	return h, chunker, out
}

var rememberTool = map[string]bool{"memory__remember": true}

// The claim streams before the tool call in the same turn; it is held, then released by the call.
func TestClaimHeldUntilToolRuns(t *testing.T) {
	h, _, out := claimHandler(rememberTool)
	h.onContent("Okay, I'll remember that.\nStrawberries are great.\n")
	if len(out) != 0 || !slices.Equal(h.unbackedClaims(), []irc.ClaimKind{irc.ClaimRemember}) {
		t.Fatalf("claim should be held: %d sent, unbacked %v", len(out), h.unbackedClaims())
	}
	h.onToolStart([]messages.ChatMessageToolCall{{Name: "memory__remember"}})
	if len(out) != 2 || len(h.unbackedClaims()) != 0 {
		t.Fatalf("want both lines released, got %d, unbacked %v", len(out), h.unbackedClaims())
	}
}

// Once the tool has run, a later claim is not held.
func TestClaimAfterToolPassesThrough(t *testing.T) {
	h, _, out := claimHandler(rememberTool)
	h.onToolStart([]messages.ChatMessageToolCall{{Name: "memory__remember"}})
	h.onContent("I'll remember that!\n")
	if len(out) != 1 {
		t.Fatalf("want the line sent, got %d", len(out))
	}
}

// A different tool does not back the claim.
func TestClaimNotBackedByOtherTool(t *testing.T) {
	h, _, _ := claimHandler(rememberTool)
	h.onContent("I'll remember that.\n")
	h.onToolStart([]messages.ChatMessageToolCall{{Name: "memory__recall"}})
	if len(h.unbackedClaims()) != 1 {
		t.Fatalf("recall must not back a remember claim")
	}
}

// Without the tool on offer (a custom persona has none) a claim is just text.
func TestClaimIgnoredWithoutTools(t *testing.T) {
	h, _, out := claimHandler(nil)
	h.onContent("Arr, I'll remember that, matey.\n")
	if len(out) != 1 || len(h.unbackedClaims()) != 0 {
		t.Fatalf("got %d sent, unbacked %v", len(out), h.unbackedClaims())
	}
}

func TestWithoutFinalReplyDropsOnlyTheClosingText(t *testing.T) {
	call := messages.ChatMessage{Role: messages.MessageRoleAssistant,
		ToolCalls: []messages.ChatMessageToolCall{{Name: "memory__recall"}}}
	result := messages.ChatMessage{Role: messages.MessageRoleTool, Content: "nothing", ToolCallID: "1"}
	claim := messages.ChatMessage{Role: messages.MessageRoleAssistant, Content: "I'll remember that."}

	if got := withoutFinalReply([]messages.ChatMessage{call, result, claim}); len(got) != 2 {
		t.Errorf("want the claim dropped, got %d messages", len(got))
	}
	if got := withoutFinalReply([]messages.ChatMessage{call}); len(got) != 1 {
		t.Errorf("a tool-call message must be kept, got %d", len(got))
	}
}

func TestClaimNudgeNamesEveryAction(t *testing.T) {
	got := claimNudge("  you said you would {action}.\n", []irc.ClaimKind{irc.ClaimIgnore, irc.ClaimRemember})
	if got != "you said you would ignore someone and save a memory." {
		t.Errorf("got %q", got)
	}
}

// Live test 4: "I already saved those details" after the save ran a message earlier is true, so it
// goes out instead of the "didn't actually save that" fallback.
func TestAlreadyDone(t *testing.T) {
	saved := []messages.ChatMessage{
		{Role: messages.MessageRoleUser, Content: "(nick:carol) Mizira, here's the party: ..."},
		{Role: messages.MessageRoleAssistant, ToolCalls: []messages.ChatMessageToolCall{{Name: "memory__remember"}}},
		{Role: messages.MessageRoleTool, Content: "Remembered about pip: ..."},
		{Role: messages.MessageRoleAssistant, Content: "Okay, I've noted down who everyone is!"},
	}
	remember := []irc.ClaimKind{irc.ClaimRemember}
	if !alreadyDone("I already saved those details for you!", saved, remember) {
		t.Error("an earlier save should back 'already saved'")
	}
	if alreadyDone("Okay, I'll remember that.", saved, remember) {
		t.Error("a new promise to remember needs a new save")
	}
	if alreadyDone("I already saved it!", saved[:1], remember) {
		t.Error("'already' with no earlier save is still unbacked")
	}
	if alreadyDone("I already ignored him.", saved, []irc.ClaimKind{irc.ClaimIgnore}) {
		t.Error("an earlier save doesn't back an ignore")
	}
}
