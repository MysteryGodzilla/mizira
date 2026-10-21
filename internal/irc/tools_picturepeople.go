// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"strings"
	"time"

	"github.com/alexschlessinger/pollytool/tools"

	"B4reMetal/metald/internal/core"
)

// PicturePeopleTool passes a picture tool how the people its prompt names look, as "people": the
// gender their avatar is drawn as and their look memories. With only a name in the prompt, women
// were drawn as men. The value comes from memory, never from the model: any "people" it wrote is
// replaced. Edits (an image_url) get none, as the picture already shows them.
type PicturePeopleTool struct {
	tools.Tool
}

// NewPicturePeopleTool wraps tool so each new picture's call carries the people it names.
func NewPicturePeopleTool(tool tools.Tool) *PicturePeopleTool {
	return &PicturePeopleTool{Tool: tool}
}

// maxPicturePeople and maxLooks bound what one picture is told about the people in it.
const (
	maxPicturePeople = 6
	maxLooks         = 2
	maxLookChars     = 300
)

func (p *PicturePeopleTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	args = maps.Clone(args)
	if args == nil {
		args = map[string]any{}
	}
	delete(args, "people")
	prompt, _ := args["prompt"].(string)
	edit, _ := args["image_url"].(string)
	if chatCtx, err := GetIRCContext(ctx); err == nil && strings.TrimSpace(edit) == "" {
		// The request names them even when the prompt the model wrote doesn't ("draw bob" became "a
		// girl with silver hair"). Her own name there is how she was addressed, so she is in the
		// picture only when the prompt names her.
		named := prompt + "\n" + withoutName(strings.Join(chatCtx.GetArgs(), " "), chatCtx.GetBotNick())
		if who := PicturePeople(chatCtx.GetNetwork(), named, chatCtx.GetConfig().Bot.LoreFadeDays, time.Now()); who != "" {
			args["people"] = who
		}
	}
	return p.Tool.Execute(ctx, args)
}

// withoutName is text with every mention of name removed.
func withoutName(text, name string) string {
	if name == "" {
		return text
	}
	return regexp.MustCompile(`(?i)\b`+regexp.QuoteMeta(name)+`\b`).ReplaceAllString(text, "")
}

// PicturePeople describes the people prompt names that memory knows how to draw, one per line:
// "bob: drawn as female; long silver hair, a red ribbon". "" when it names nobody known.
func PicturePeople(network, prompt string, fadeDays int, now time.Time) string {
	store, err := core.Memories()
	if err != nil || strings.TrimSpace(prompt) == "" {
		return ""
	}
	names, err := store.Mentioned(network, prompt, nil)
	if err != nil {
		return ""
	}
	people, err := store.People(network)
	if err != nil {
		return ""
	}
	var lines []string
	for _, name := range names {
		if len(lines) == maxPicturePeople {
			break
		}
		var parts []string
		if g := drawnAs(people, name); g != "" {
			parts = append(parts, "drawn as "+g)
		}
		mems, meta, err := store.RankedFor(network, name, fadeDays, 50, now)
		if err == nil {
			n := 0
			for _, m := range core.OfKind(mems, meta, core.KindLook) {
				if m.Subject != name || n == maxLooks {
					continue
				}
				fact := m.Fact
				if r := []rune(fact); len(r) > maxLookChars {
					fact = string(r[:maxLookChars]) + "…"
				}
				parts = append(parts, fact)
				n++
			}
		}
		if len(parts) > 0 {
			lines = append(lines, fmt.Sprintf("%s: %s", name, strings.Join(parts, "; ")))
		}
	}
	return strings.Join(lines, "\n")
}

// drawnAs is the gender a person's picture shows, from the first of their avatar's pronouns, or their
// own when the avatar has none of its own; "" for they/them or when unknown.
func drawnAs(people core.People, name string) string {
	p, ok := people.Get(name)
	if !ok {
		return ""
	}
	pronouns := p.AvatarPronouns
	if pronouns == "" {
		pronouns = p.Pronouns
	}
	first, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(pronouns)), "/")
	switch strings.TrimSpace(first) {
	case "she":
		return "female"
	case "he":
		return "male"
	}
	return ""
}
