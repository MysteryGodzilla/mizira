// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"context"
	"strings"
	"testing"

	"github.com/alexschlessinger/pollytool/tools"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

// The person's own "forget everything about me", and forgetting a locked fact by name, both leave a
// locked memory, and the model is told so.
func TestForgetToolKeepsLockedMemories(t *testing.T) {
	store, err := core.Memories()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = store.ForgetSubject("", "dave")
	locked, _ := store.Remember("", "dave", "dave is allergic to peanuts", "dave", "#chat")
	_, _ = store.Remember("", "dave", "dave likes jazz", "dave", "#chat")
	_, _ = store.Lock("", locked, "console:token")
	t.Cleanup(func() { _, _ = store.Unlock("", locked); _, _ = store.ForgetSubject("", "dave") })

	ctx := InjectContext(context.Background(), mocktest.NewMockContext().WithSource("dave"))
	forget := newMemoryForgetTool()
	if out, _ := forget.Execute(ctx, tools.Args{"fact": "allergic to peanuts"}); !strings.Contains(out, "locked") {
		t.Errorf("forgetting a locked fact by name: %s", out)
	}
	out, _ := forget.Execute(ctx, tools.Args{"fact": "everything"})
	if !strings.Contains(out, "Forgot 1") || !strings.Contains(out, "1 locked") {
		t.Errorf("forget everything: %s", out)
	}
	if _, found, _ := store.Get("", locked); !found {
		t.Error("the locked memory was forgotten")
	}
}
