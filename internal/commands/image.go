// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"os"
	"strings"

	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
)

// ImageCommand lists the picture styles (IMAGE_STYLES) by the names people can ask for.
type ImageCommand struct{}

func (c *ImageCommand) Name() string    { return "+image" }
func (c *ImageCommand) AdminOnly() bool { return false }

func (c *ImageCommand) Available(irc.ChatContextInterface) bool {
	return strings.TrimSpace(os.Getenv("IMAGE_STYLES")) != ""
}

func (c *ImageCommand) Execute(ctx irc.ChatContextInterface) {
	prefix := ctx.GetConfig().Bot.CommandPrefix
	if args := ctx.GetArgs(); len(args) > 1 && !strings.EqualFold(args[1], "styles") {
		ctx.Reply("Usage: " + prefix + "image styles")
		return
	}
	styles, err := core.ParseImageStyles(os.Getenv("IMAGE_STYLES"))
	if err != nil {
		ctx.GetLogger().Warn("image_styles_invalid", "error", err.Error())
	}
	for _, line := range styleLines(styles) {
		ctx.Reply(line)
	}
}

// styleLines is the reply: drawing styles, then restyling ones, then how to ask.
func styleLines(styles []core.ImageStyle) []string {
	first := map[string]string{}
	listed := func(kind string) string {
		var parts []string
		for _, s := range styles {
			if s.Kind == kind {
				parts = append(parts, s.Name+" ("+s.Description+")")
				if first[kind] == "" {
					first[kind] = s.Name
				}
			}
		}
		return strings.Join(parts, ", ")
	}
	var out, asks []string
	if draw := listed("draw"); draw != "" {
		out = append(out, "new picture styles: "+draw)
		asks = append(asks, `"draw a fox in the snow, `+first["draw"]+` style"`)
	}
	if edit := listed("edit"); edit != "" {
		out = append(out, "restyle a picture she drew: "+edit)
		asks = append(asks, `"make that picture `+first["edit"]+`"`)
	}
	if edit2 := listed("edit2"); edit2 != "" {
		out = append(out, "with a second picture (admins only): "+edit2)
	}
	if len(out) == 0 {
		return []string{"no picture styles are set up"}
	}
	return append(out, "ask e.g. "+strings.Join(asks, " or "))
}
