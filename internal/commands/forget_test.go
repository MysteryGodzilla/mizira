// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"fmt"
	"strings"
	"testing"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

// "+forget [6]" works like "+memories forget 6", with the same rule: your own memories only,
// unless you are an admin.
func TestForgetCommand(t *testing.T) {
	store, err := core.Memories()
	if err != nil {
		t.Fatal(err)
	}
	const network = "forget-cmd"
	own, _ := store.Remember(network, "alice", "alice likes tea", "alice", "#test")
	other, _ := store.Remember(network, "bob", "bob likes coffee", "bob", "#test")
	t.Cleanup(func() { store.Clear(network) })

	ctx := mocktest.NewMockContext().WithSource("alice")
	ctx.GetConfig().Server.Name = network

	if out := runCmd(ctx, &ForgetCommand{}, fmt.Sprintf("[%d]", other)); !strings.Contains(out, "only they or an operator") {
		t.Errorf("someone else's memory: %q", out)
	}
	if out := runCmd(ctx, &ForgetCommand{}, fmt.Sprintf("[%d]", own)); !strings.HasPrefix(out, "forgot memory") {
		t.Errorf("own memory: %q", out)
	}
	if out := runCmd(ctx, &ForgetCommand{}); !strings.Contains(out, "usage") {
		t.Errorf("no id: %q", out)
	}
}
