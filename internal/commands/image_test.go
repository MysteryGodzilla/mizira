// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"strings"
	"testing"

	mocktest "B4reMetal/metald/internal/testing"
)

func TestImageStylesCommand(t *testing.T) {
	sha := strings.Repeat("a", 64)
	t.Setenv("IMAGE_STYLES", "soyuz | draw | 1 | soyuzmult | Soviet cartoon | https://civitai.com/x | "+sha+"\n"+
		"disco | draw | 1 | | painted game art | https://civitai.com/y | "+sha+"\n"+
		"clay | edit | 1 | Transform the image into a clay sculpture style | clay figure | https://huggingface.co/z | "+sha)
	cmd := &ImageCommand{}
	if !cmd.Available(mocktest.NewMockContext()) {
		t.Fatal("not offered with styles set")
	}
	ctx := mocktest.NewMockContext().WithArgs("+image", "styles")
	cmd.Execute(ctx)
	want := []string{
		"new picture styles: soyuz (Soviet cartoon), disco (painted game art)",
		"restyle a picture she drew: clay (clay figure)",
		`ask e.g. "draw a fox in the snow, soyuz style" or "make that picture clay"`,
	}
	if strings.Join(ctx.Replies, "\n") != strings.Join(want, "\n") {
		t.Errorf("replies:\n%s", strings.Join(ctx.Replies, "\n"))
	}

	t.Setenv("IMAGE_STYLES", "")
	if cmd.Available(mocktest.NewMockContext()) {
		t.Error("offered with no styles")
	}
}
