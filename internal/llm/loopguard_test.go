// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"testing"

	"github.com/alexschlessinger/pollytool/messages"

	"B4reMetal/metald/internal/irc"
	mocktest "B4reMetal/metald/internal/testing"
)

func TestLoopStart(t *testing.T) {
	loops := map[string]string{
		// From the Qwen3.8 distill run.
		"Oh... um... I... I... blushes ...I... I... fidgets ...I... I... blushes ...I... I... fidgets ...I... I... blushes ...I... I... fidgets ...I... I... blushes ...I... I... fidgets": "Oh... um... I... I... blushes ...I... I... fidgets",
		"it's... it's... it's... it's... it's... it's... it's... it's... it's... it's... it's... it's... it's":                                                                             "it's",
		"Sure! ha ha ha ha ha ha ha ha ha ha ha ha ha": "Sure! ha",
	}
	for text, keep := range loops {
		off, ok := loopStart(text)
		if !ok {
			t.Errorf("loop not found in %q", text)
			continue
		}
		if text[:off] != keep {
			t.Errorf("kept %q, want %q", text[:off], keep)
		}
	}

	fine := []string{
		"E-eh?! I-I can't do that, it's a secret! ごめんね.",
		"I like strawberries, and I like ramen, and I like tea, and I like rain.",
		"no no no, that's not it",
		"ha ha ha ha",
		"",
	}
	for _, text := range fine {
		if _, ok := loopStart(text); ok {
			t.Errorf("false loop in %q", text)
		}
	}
}

// Streamed in small pieces, a loop is cut where it began: the channel and history get only the start.
func TestLoopGuardCutsStreamedReply(t *testing.T) {
	ctx := mocktest.NewMockContext()
	out := make(chan string, 20)
	chunker := irc.NewChunker(out, 400)
	chunker.SetJoinLines(true)
	h := newCallbackHandler(ctx, chunker, ctx.GetConfig())

	reply := "Oh... um... I... I... blushes ...I... I... fidgets"
	for range 6 {
		reply += " ...I... I... blushes ...I... I... fidgets"
	}
	for i := 0; i < len(reply); i += 7 {
		h.onContent(reply[i:min(i+7, len(reply))])
	}
	h.flush()
	close(out)

	var got []string
	for l := range out {
		got = append(got, l)
	}
	want := "Oh... um... I... I... blushes ...I... I... fidgets"
	if !h.looped || len(got) != 1 || got[0] != want {
		t.Fatalf("looped=%v, sent %q", h.looped, got)
	}
	msgs := []messages.ChatMessage{
		{Role: messages.MessageRoleUser, Content: "hi"},
		{Role: messages.MessageRoleAssistant, Content: reply},
	}
	if hist := withReplyText(msgs, h.loopKept); hist[1].Content != want || msgs[1].Content != reply {
		t.Fatalf("history %q (original must be untouched)", hist[1].Content)
	}
}
