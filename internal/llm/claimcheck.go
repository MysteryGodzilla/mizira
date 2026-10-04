// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"regexp"
	"slices"
	"strings"

	"github.com/alexschlessinger/pollytool/llm"
	"github.com/alexschlessinger/pollytool/messages"
	"github.com/alexschlessinger/pollytool/tools"

	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// claimAction names each action in the claimnudge prompt's {action}.
var claimAction = map[irc.ClaimKind]string{
	irc.ClaimRemember: "save a memory",
	irc.ClaimIgnore:   "ignore someone",
	irc.ClaimForget:   "forget a memory",
}

// claimFallback replaces a claim the retry still didn't back with a tool call.
var claimFallback = map[irc.ClaimKind]string{
	irc.ClaimRemember: "sorry, i didn't actually save that. ask me again?",
	irc.ClaimIgnore:   "sorry, i didn't actually ignore anyone. ask me again?",
	irc.ClaimForget:   "sorry, i didn't actually forget that. ask me again?",
}

// claimableTools lists the claim tools this request can call; nil when it has no tools.
func claimableTools(registry *tools.ToolRegistry) map[string]bool {
	if registry == nil {
		return nil
	}
	out := map[string]bool{}
	for _, name := range irc.ClaimTool {
		if _, ok := registry.Get(name); ok {
			out[name] = true
		}
	}
	return out
}

func claimNudge(template string, kinds []irc.ClaimKind) string {
	actions := make([]string, len(kinds))
	for i, k := range kinds {
		actions[i] = claimAction[k]
	}
	return strings.ReplaceAll(strings.TrimSpace(template), "{action}", strings.Join(actions, " and "))
}

// withoutFinalReply drops the closing text-only assistant message, the one that made the claim.
func withoutFinalReply(msgs []messages.ChatMessage) []messages.ChatMessage {
	n := len(msgs)
	if n > 0 && msgs[n-1].Role == messages.MessageRoleAssistant && len(msgs[n-1].ToolCalls) == 0 {
		return slices.Clone(msgs[:n-1])
	}
	return slices.Clone(msgs)
}

// retryUnbackedClaim gives the model one more turn after its reply claimed an action no tool
// call took, and returns the messages to commit in place of the first reply. If the retry still
// claims without the tool, an honest line goes out instead.
func retryUnbackedClaim(chatCtx core.ChatContextInterface, agent *llm.Agent, req *CompletionRequest,
	chunker *irc.Chunker, claimTools map[string]bool, first []messages.ChatMessage, kinds []irc.ClaimKind,
) []messages.ChatMessage {
	log := chatCtx.GetLogger()
	held := chunker.DropHeld()
	log.Warn("claim_without_tool", "claims", kinds, "held", truncateForLog(strings.Join(held, " / ")))

	kept := withoutFinalReply(first)
	retry := *req
	retry.Messages = slices.Concat(req.Messages, kept, []messages.ChatMessage{{
		Role:    messages.MessageRoleUser,
		Content: claimNudge(chatCtx.GetConfig().Bot.ClaimNudge, kinds),
	}})

	cb := newCallbackHandler(chatCtx, chunker, chatCtx.GetConfig())
	cb.watchClaims(claimTools)
	resp, err := agent.Run(chatCtx, &retry, cb.build())
	if chatCtx.Err() != nil {
		cb.discard()
		return kept
	}
	if err == nil {
		cb.flush()
	}
	if err != nil || len(cb.unbackedClaims()) > 0 {
		cb.discard()
		fields := []any{"claims", kinds}
		if err != nil {
			fields = append(fields, "error", err.Error())
		}
		log.Warn("claim_retry_failed", fields...)
		fallback := claimFallback[kinds[0]]
		chunker.SetHold(nil)
		chunker.Write(fallback + "\n")
		chunker.Flush()
		return append(kept, messages.ChatMessage{Role: messages.MessageRoleAssistant, Content: fallback})
	}
	log.Info("claim_retry_done", "claims", kinds, "tool_count", cb.toolCount)
	if cb.looped {
		return append(kept, withReplyText(resp.AllMessages, cb.loopKept)...)
	}
	return append(kept, resp.AllMessages...)
}

// alreadyWord marks a reply about something done before this message: "I already saved those".
var alreadyWord = regexp.MustCompile(`(?i)\balready\b`)

// recentTurns is how far back an earlier tool call can back an "already" claim.
const recentTurns = 12

// alreadyDone reports a claim that points at a tool call made earlier in the conversation, so the
// claim needs no new call: the reply says "already", and every claimed tool was called in the last
// few messages.
func alreadyDone(held string, history []messages.ChatMessage, kinds []irc.ClaimKind) bool {
	if !alreadyWord.MatchString(held) {
		return false
	}
	recent := history[max(0, len(history)-recentTurns):]
	ran := map[string]bool{}
	for _, m := range recent {
		for _, tc := range m.ToolCalls {
			ran[tc.Name] = true
		}
	}
	for _, k := range kinds {
		if !ran[irc.ClaimTool[k]] {
			return false
		}
	}
	return true
}
