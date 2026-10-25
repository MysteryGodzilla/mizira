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

// ImageCommand lists the picture styles (IMAGE_STYLES) and the looks set for people, by the names people
// can ask for.
type ImageCommand struct{}

func (c *ImageCommand) Name() string    { return "+image" }
func (c *ImageCommand) AdminOnly() bool { return false }

func (c *ImageCommand) Available(irc.ChatContextInterface) bool {
	return strings.TrimSpace(os.Getenv("IMAGE_STYLES")) != ""
}

func (c *ImageCommand) Execute(ctx irc.ChatContextInterface) {
	prefix := ctx.GetConfig().Bot.CommandPrefix
	args := ctx.GetArgs()
	if len(args) > 1 && strings.EqualFold(args[1], "looks") {
		store, err := core.Memories()
		if err != nil {
			ctx.GetLogger().Error("memory_store_unavailable", "error", err.Error())
			ctx.Reply("memory is unavailable")
			return
		}
		profiles, err := store.Profiles(ctx.GetNetwork())
		if err != nil {
			ctx.Reply("memory is unavailable")
			return
		}
		people, _ := store.People(ctx.GetNetwork())
		for _, line := range lookLines(profiles, people, strings.Join(args[2:], " "), prefix) {
			ctx.Reply(line)
		}
		return
	}
	if len(args) > 1 && !strings.EqualFold(args[1], "styles") {
		ctx.Reply("Usage: " + prefix + "image styles | " + prefix + "image looks [name]")
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

// lookLines is the reply to "looks": everyone with a set look and their variants' names, or what each
// of one person's looks wears, so a variant can be asked for by its name ("office", not "business").
func lookLines(profiles []core.Profile, people core.People, name, prefix string) []string {
	if name == "" {
		var parts []string
		example := ""
		for _, p := range profiles {
			if !p.Look.Set() {
				continue
			}
			names := []string{p.Look.Default()}
			for _, v := range p.Look.Variants {
				names = append(names, v.Name)
			}
			parts = append(parts, p.Subject+" ("+strings.Join(names, ", ")+")")
			if example == "" && len(names) > 1 {
				example = `"draw ` + p.Subject + ` in the ` + names[1] + ` look"`
			}
		}
		if len(parts) == 0 {
			return []string{"no picture looks are set up"}
		}
		out := []string{"picture looks: " + strings.Join(parts, ", ")}
		if example != "" {
			out = append(out, "name a look when asking, e.g. "+example+"; "+prefix+"image looks <name> shows what each one is")
		}
		return out
	}
	p, ok := people.Get(people.Resolve(name))
	if !ok || !p.Look.Set() {
		return []string{name + " has no picture look set; pictures of them go by their look memories"}
	}
	outfit := p.Look.Outfit
	if outfit == "" {
		outfit = "no set outfit, dressed as asked"
	}
	out := []string{p.Subject + ", " + p.Look.Default() + " (the default): " + truncateText(outfit, 200)}
	for _, v := range p.Look.Variants {
		var about []string
		if v.Outfit != "" {
			about = append(about, v.Outfit)
		}
		if v.Hair != "" {
			about = append(about, "hair: "+v.Hair)
		}
		out = append(out, v.Name+": "+truncateText(strings.Join(about, "; "), 200))
	}
	if len(p.Look.Variants) > 0 {
		out = append(out, `ask e.g. "draw `+p.Subject+` in the `+p.Look.Variants[0].Name+` look", or "`+
			p.Look.Default()+` `+p.Subject+`" for the default`)
	}
	return out
}
