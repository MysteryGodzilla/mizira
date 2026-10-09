// SPDX-License-Identifier: GPL-3.0-only

package llm

import (
	"testing"

	"github.com/alexschlessinger/pollytool/llm"
)

// llama.cpp's templates read enable_thinking, so each setting sends it plainly, and thinking gets
// room of its own on top of maxtokens.
func TestApplyThinking(t *testing.T) {
	cases := []struct {
		effort llm.ThinkingEffort
		on     bool
		tokens int
	}{
		{"", false, 600},
		{llm.ThinkingOff, false, 600},
		{"none", false, 600},
		{llm.ThinkingLow, true, 600 + 1024},
		{llm.ThinkingMedium, true, 600 + 2048},
		{llm.ThinkingHigh, true, 600 + 4096},
		{"max", true, 600 + 2048},
	}
	for _, c := range cases {
		req := &CompletionRequest{MaxTokens: 600, ExtraBody: map[string]any{"top_k": 20}}
		applyThinking(req, c.effort)
		kwargs, _ := req.ExtraBody["chat_template_kwargs"].(map[string]any)
		if kwargs["enable_thinking"] != c.on || req.MaxTokens != c.tokens || req.ExtraBody["top_k"] != 20 {
			t.Errorf("%q: kwargs %v, max tokens %d, extra %v; want thinking %v, %d", c.effort, kwargs,
				req.MaxTokens, req.ExtraBody, c.on, c.tokens)
		}
	}
}
