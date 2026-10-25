// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"strings"
	"testing"

	"B4reMetal/metald/internal/core"
	mocktest "B4reMetal/metald/internal/testing"
)

func TestImageStylesCommand(t *testing.T) {
	sha := strings.Repeat("a", 64)
	t.Setenv("IMAGE_STYLES", "soyuz | draw | 1 | soyuzmult | Soviet cartoon | https://civitai.com/x | "+sha+"\n"+
		"disco | draw | 1 | | painted game art | https://civitai.com/y | "+sha+"\n"+
		"clay | edit | 1 | Transform the image into a clay sculpture style | clay figure | https://huggingface.co/z | "+sha+"\n"+
		"outfit | edit2 | 1 | Dress the person in image 1 in the {prompt} shown in image 2. | outfit swap | https://huggingface.co/o | "+sha+" | say the outfit in a few words")
	cmd := &ImageCommand{}
	if !cmd.Available(mocktest.NewMockContext()) {
		t.Fatal("not offered with styles set")
	}
	ctx := mocktest.NewMockContext().WithArgs("+image", "styles")
	cmd.Execute(ctx)
	want := []string{
		"new picture styles: soyuz (Soviet cartoon), disco (painted game art)",
		"restyle a picture she drew: clay (clay figure)",
		"with a second picture (admins only): outfit (outfit swap)",
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

// A variant named "office" was missed when asked for as "business": people can list the names.
func TestImageLooksCommand(t *testing.T) {
	const net = "image-looks"
	store, _ := core.Memories()
	store.SetProfile(net, core.Profile{Subject: "carol", Pronouns: "she/her", Aliases: []string{"caz"}})
	store.SetLook(net, "carol", core.Look{Appearance: "adult woman", Outfit: "a white t-shirt and grey shorts",
		Variants: []core.LookVariant{{Name: "office", Outfit: "a charcoal blazer"}, {Name: "bun", Hair: "a messy bun"}}}, "")
	store.SetLook(net, "dave", core.Look{Appearance: "adult man"}, "")
	store.SetProfile(net, core.Profile{Subject: "erin", Pronouns: "she/her"})
	t.Cleanup(func() {
		for _, s := range []string{"carol", "dave", "erin"} {
			store.DeleteProfile(net, s)
		}
	})

	for args, want := range map[string][]string{
		"": {
			"picture looks: carol (casual, office, bun), dave (casual)",
			`name a look when asking, e.g. "draw carol in the office look"; ~image looks <name> shows what each one is`,
		},
		"Caz": {
			"carol, casual (the default): a white t-shirt and grey shorts",
			"office: a charcoal blazer",
			"bun: hair: a messy bun",
			`ask e.g. "draw carol in the office look", or "casual carol" for the default`,
		},
		"dave": {"dave, casual (the default): no set outfit, dressed as asked"},
		"erin": {"erin has no picture look set; pictures of them go by their look memories"},
	} {
		ctx := mocktest.NewMockContext().WithArgs(append([]string{"~image", "looks"}, strings.Fields(args)...)...)
		ctx.GetConfig().Server.Name = net
		ctx.GetConfig().Bot.CommandPrefix = "~"
		(&ImageCommand{}).Execute(ctx)
		if strings.Join(ctx.Replies, "\n") != strings.Join(want, "\n") {
			t.Errorf("looks %q:\n%s", args, strings.Join(ctx.Replies, "\n"))
		}
	}
}
