// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"context"
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/alexschlessinger/pollytool/tools"

	"B4reMetal/metald/internal/core"
)

// PicturePeopleTool passes a picture tool how the people its prompt names look, as "people": the
// gender their avatar is drawn as, and their profile look or else their look memories. With only a
// name in the prompt, women were drawn as men. The value comes from memory, never from the model: any "people" it wrote is
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
		// girl with silver hair"). One mention of her own name there is how she was addressed.
		// The trigger, as her nick can be someone else's account.
		self := chatCtx.GetConfig().Bot.Trigger
		if self == "" {
			self = chatCtx.GetBotNick()
		}
		request := speakerNamed(selfNamed(strings.Join(chatCtx.GetArgs(), " "), self), chatCtx.GetSource())
		named := prompt + "\n" + request
		if who := PicturePeople(chatCtx.GetNetwork(), named, chatCtx.GetConfig().Bot.LoreFadeDays, time.Now()); who != "" {
			args["people"] = who
		}
	}
	return p.Tool.Execute(ctx, args)
}

// selfNamed is a request with the bot's name once less, as that one addressed it, and with "yourself"
// as the bot's name.
func selfNamed(request, bot string) string {
	if bot == "" {
		return request
	}
	if loc := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(bot) + `\b`).FindStringIndex(request); loc != nil {
		request = request[:loc[0]] + request[loc[1]:]
	}
	return yourself.ReplaceAllString(request, bot)
}

var yourself = regexp.MustCompile(`(?i)\byourself\b`)

// meWord is "me" or "myself" with the word before and after it, if any.
var meWord = regexp.MustCompile(`(?i)(\b\w+\s+)?\b(me|myself)\b(\s+\w+)?`)

// speakerNamed is a request with "me"/"myself" as the speaker's name when they are in the picture:
// "do me in yamamoto style" drew the bot. Not "draw me a cat" or "make one for me".
func speakerNamed(request, speaker string) string {
	if speaker == "" {
		return request
	}
	return meWord.ReplaceAllStringFunc(request, func(m string) string {
		parts := meWord.FindStringSubmatch(m)
		before, after := strings.ToLower(strings.TrimSpace(parts[1])), strings.ToLower(strings.TrimSpace(parts[3]))
		if before == "for" || before == "to" || slices.Contains([]string{"a", "an", "some", "one", "another"}, after) {
			return m
		}
		return parts[1] + speaker + parts[3]
	})
}

// PicturePerson is one person a picture names, as the picture tool gets them (JSON in "people").
// Look is set from their profile and goes into the picture word for word; without one, Notes are
// their look memories, for the prompt writer to use.
type PicturePerson struct {
	Name    string   `json:"name"`
	DrawnAs string   `json:"drawn_as,omitempty"`
	Look    string   `json:"look,omitempty"`
	Outfit  string   `json:"outfit,omitempty"`
	Variant string   `json:"variant,omitempty"`
	Notes   []string `json:"notes,omitempty"`
}

// PicturePeople describes the people text names that memory knows how to draw, as a JSON list; ""
// when it names nobody known. A profile look wins over look memories, and a variant named in text
// picks its hair and outfit.
func PicturePeople(network, text string, fadeDays int, now time.Time) string {
	store, err := core.Memories()
	if err != nil || strings.TrimSpace(text) == "" {
		return ""
	}
	names, err := store.Mentioned(network, text, nil)
	if err != nil {
		return ""
	}
	people, err := store.People(network)
	if err != nil {
		return ""
	}
	var out []PicturePerson
	for _, name := range names {
		if len(out) == maxPicturePeople {
			break
		}
		p := PicturePerson{Name: name, DrawnAs: drawnAs(people, name)}
		if pr, ok := people.Get(name); ok && pr.Look.Set() {
			hair, outfit, variant := pr.Look.For(text)
			p.Look = strings.TrimSuffix(pr.Look.Appearance, ".")
			if hair != "" {
				p.Look += ", " + hair
			}
			p.Outfit, p.Variant = outfit, variant
		} else if mems, meta, err := store.RankedFor(network, name, fadeDays, 50, now); err == nil {
			for _, m := range core.OfKind(mems, meta, core.KindLook) {
				if m.Subject != name || len(p.Notes) == maxLooks {
					continue
				}
				fact := m.Fact
				if r := []rune(fact); len(r) > maxLookChars {
					fact = string(r[:maxLookChars]) + "…"
				}
				p.Notes = append(p.Notes, fact)
			}
		}
		if p.DrawnAs != "" || p.Look != "" || len(p.Notes) > 0 {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return ""
	}
	b, _ := json.Marshal(out)
	return string(b)
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
