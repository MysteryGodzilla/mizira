// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/alexschlessinger/pollytool/tools"

	"B4reMetal/metald/internal/core"
)

// PicturePeopleTool passes a picture tool how the people its prompt names look, as "people": the
// gender their avatar is drawn as, and their profile look or else their look memories. With only a
// name in the prompt, women were drawn as men. The value comes from memory, never from the model: any "people" it wrote is
// replaced. Edits (an image_url) get them too, for "back in her default outfit". The style the request
// names replaces another, a drawing style it doesn't name is dropped, and the request itself is passed
// on ("request").
type PicturePeopleTool struct {
	tools.Tool
}

// NewPicturePeopleTool wraps tool so each new picture's call carries the people it names.
func NewPicturePeopleTool(tool tools.Tool) *PicturePeopleTool {
	return &PicturePeopleTool{Tool: tool}
}

// maxPicturePeople and maxLooks bound what one picture is told about the people in it.
const (
	maxPicturePeople = 10
	maxLooks         = 2
	maxLookChars     = 300
	// promptAltWithin is how near their name an alt the model wrote must be ("bob dressed in her
	// villainess gown"); further off it was the scene.
	promptAltWithin = 4
)

func (p *PicturePeopleTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	args = maps.Clone(args)
	if args == nil {
		args = map[string]any{}
	}
	delete(args, "people")
	delete(args, "request")
	prompt, _ := args["prompt"].(string)
	if chatCtx, err := GetIRCContext(ctx); err == nil {
		// The request names them even when the prompt the model wrote doesn't ("draw bob" became "a
		// girl with silver hair"). One mention of her own name there is how she was addressed.
		// The trigger, as her nick can be someone else's account.
		self := chatCtx.GetConfig().Bot.Trigger
		if self == "" {
			self = chatCtx.GetBotNick()
		}
		asked := strings.Join(chatCtx.GetArgs(), " ")
		// The asker's own words, so the tool can tell clothes they asked for from ones the model made up.
		args["request"] = asked
		if style, _ := args["style"].(string); style != "" {
			if named := namedStyle(asked); named != "" && named != style {
				// "outfitswap the last image to the outfit in this one" went out as vn, was refused, and
				// she drew a new picture instead.
				args["style"] = named
				chatCtx.GetLogger().Info("picture_style_replaced", "style", style, "asked", named)
			} else if unaskedDrawStyle(style, asked) {
				delete(args, "style")
				chatCtx.GetLogger().Info("picture_style_dropped", "style", style, "reason", "not in the request")
			}
		}
		request := speakerNamed(selfNamed(asked, self), chatCtx.GetSource())
		network, key := chatCtx.GetNetwork(), chatCtx.GetLockKey()+"\x00"+strings.ToLower(chatCtx.GetSource())
		earlier := ""
		if namesSomeone(network, request) {
			lastAsked.keep(key, request, time.Now())
		} else if earlier = lastAsked.get(key, time.Now()); earlier != "" {
			// "try that image request again" names nobody; the request before it said who and in what.
			args["request"] = earlier + "\n" + asked
		}
		if rewritten := unaskedSelf(network, prompt, strings.TrimSpace(earlier+"\n"+request), self, chatCtx.GetSource()); rewritten != prompt {
			chatCtx.GetLogger().Info("picture_self_unasked", "prompt", rewritten)
			prompt = rewritten
			args["prompt"] = prompt
		}
		if who := PicturePeople(network, prompt, request, earlier, chatCtx.GetConfig().Bot.LoreFadeDays, time.Now()); who != "" {
			args["people"] = who
		}
	}
	return p.Tool.Execute(ctx, args)
}

// lastAsked is each asker's last picture request that named someone, per conversation, for an hour.
var lastAsked = &askedStore{m: map[string]askedAt{}}

type askedStore struct {
	mu sync.Mutex
	m  map[string]askedAt
}

type askedAt struct {
	text string
	at   time.Time
}

func (s *askedStore) keep(key, text string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = askedAt{text, now}
}

func (s *askedStore) get(key string, now time.Time) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.m[key]; ok && now.Sub(a.at) < time.Hour {
		return a.text
	}
	return ""
}

// namesSomeone reports whether text names someone memory knows, other than those in except.
func namesSomeone(network, text string, except ...string) bool {
	store, err := core.Memories()
	if err != nil {
		return false
	}
	names, _ := store.Mentioned(network, text, except)
	return len(names) > 0
}

// youWord is "you" as someone in the picture, not "can you draw".
var youWord = regexp.MustCompile(`(?i)(\b\w+\s+)?\byou\b`)

// unaskedSelf is the prompt without the bot where a request naming others didn't ask for her: for
// "villainess bob, retainer carol and office me" she added herself as a fourth. With the asker in the
// request and her in the prompt in their place, she wrote herself for "me": their name goes in.
func unaskedSelf(network, prompt, request, self, speaker string) string {
	if self == "" || !namesSomeone(network, request, self) {
		return prompt
	}
	selfWord := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(self) + `\b`)
	if selfWord.MatchString(request) || !selfWord.MatchString(prompt) {
		return prompt
	}
	for _, m := range youWord.FindAllStringSubmatch(request, -1) {
		switch strings.ToLower(strings.TrimSpace(m[1])) {
		case "can", "could", "would", "will", "do", "did", "are", "thank", "if", "should", "have", "when":
		default:
			return prompt
		}
	}
	speakerWord := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(speaker) + `\b`)
	if speaker != "" && speakerWord.MatchString(request) && !speakerWord.MatchString(prompt) {
		return selfWord.ReplaceAllLiteralString(prompt, speaker)
	}
	var kept strings.Builder
	for _, clause := range clauseWord.FindAllString(prompt, -1) {
		if selfWord.MatchString(clause) && !namesSomeone(network, clause, self) {
			continue
		}
		kept.WriteString(clause)
	}
	return strings.TrimSpace(kept.String())
}

// clauseWord is a clause with the punctuation and space after it.
var clauseWord = regexp.MustCompile(`[^,.;]+[,.;]*\s*`)

// unaskedDrawStyle reports whether style is a drawing style the request doesn't name ("pc-98" names
// pc98): she put the last picture's style on "draw baremetal". Edit styles are left to the tool.
func unaskedDrawStyle(style, request string) bool {
	styles, _ := core.ParseImageStyles(os.Getenv("IMAGE_STYLES"))
	for _, s := range styles {
		if s.Name == style && s.Kind == "draw" {
			return !saysStyle(request, s.Name)
		}
	}
	return false
}

// namedStyle is the one style the request names, or "" for none or several.
func namedStyle(request string) string {
	styles, _ := core.ParseImageStyles(os.Getenv("IMAGE_STYLES"))
	named := ""
	for _, s := range styles {
		if saysStyle(request, s.Name) {
			if named != "" {
				return ""
			}
			named = s.Name
		}
	}
	return named
}

var linkWord = regexp.MustCompile(`\S+://\S+`)

// saysStyle reports whether the request names style, outside its links ("pc-98" names pc98).
func saysStyle(request, style string) bool {
	said := strings.ReplaceAll(strings.ToLower(linkWord.ReplaceAllString(request, " ")), "-", "")
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(style) + `\b`).MatchString(said)
}

// selfNamed is a request with the bot's name once less, as that one addressed it (said twice running,
// "mizira mizira now draw bob", both: the second put her in the picture), and with "yourself" as the
// bot's name.
func selfNamed(request, bot string) string {
	if bot == "" {
		return request
	}
	name := `\b` + regexp.QuoteMeta(bot) + `\b`
	if loc := regexp.MustCompile(`(?i)` + name + `(?:[\s,.:;!~]+` + name + `)*`).FindStringIndex(request); loc != nil {
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
// their look memories, for the prompt writer to use. Aliases are their other names, which the prompt
// may use too ("alice Ally" drew two women). Usual is the outfit a variant's replaced, which the model
// copied into the prompt clause by clause ("deep blue panel hanging down front, golden feathered
// shoulder pieces, ...") and the prompt writer drew on a second alice.
type PicturePerson struct {
	Name    string   `json:"name"`
	Aliases []string `json:"aliases,omitempty"`
	DrawnAs string   `json:"drawn_as,omitempty"`
	Look    string   `json:"look,omitempty"`
	Outfit  string   `json:"outfit,omitempty"`
	Variant string   `json:"variant,omitempty"`
	Usual   string   `json:"usual,omitempty"`
	// UsualHair is the hair a variant's replaced, which the model copied in too: "very long wavy hair" with
	// her ponytail alt.
	UsualHair string `json:"usual_hair,omitempty"`
	// DefaultName is what their default outfit is called ("casual"), so "her casual outfit" isn't new clothes.
	DefaultName string   `json:"default_name,omitempty"`
	Notes       []string `json:"notes,omitempty"`
}

// PicturePeople describes the people the prompt or request names that memory knows how to draw, as a
// JSON list; "" when they name nobody known. A profile look wins over look memories, and a variant
// named with them picks its hair and outfit: the request's, then the earlier request's (for "try that
// again"), then the prompt's beside their name. The prompt the model wrote called bob a chef when the
// asker said heist, and opened on "a sleek modern office" when they said villainess.
func PicturePeople(network, prompt, request, earlier string, fadeDays int, now time.Time) string {
	store, err := core.Memories()
	text := prompt + "\n" + request
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
		pr, ok := people.Get(name)
		if ok {
			p.Aliases = pr.Aliases
		}
		if ok && pr.Look.Set() {
			names := append([]string{name}, pr.Aliases...)
			hair, outfit, variant := pr.Look.For(request, 0, names...)
			if variant == "" {
				hair, outfit, variant = pr.Look.For(earlier, 0, names...)
			}
			if variant == "" {
				hair, outfit, variant = pr.Look.For(prompt, promptAltWithin, names...)
				if variant != "" && variant != pr.Look.Default() && outfitSays(pr.Look.Outfit, variant) {
					// "remy a rat chef" described his default chef's jacket, and drew his chef alt.
					hair, outfit, variant = pr.Look.Hair, pr.Look.Outfit, ""
				}
			}
			if variant == pr.Look.Default() {
				// Named on purpose ("casual hori"), the defaults stand over an earlier request's alt.
				variant = ""
			}
			p.DefaultName = pr.Look.Default()
			p.Look = strings.TrimSuffix(pr.Look.Appearance, ".")
			if hair != "" {
				p.Look += ", " + hair
			}
			p.Outfit, p.Variant = outfit, variant
			if outfit != pr.Look.Outfit {
				p.Usual = pr.Look.Outfit
			}
			if hair != pr.Look.Hair {
				p.UsualHair = pr.Look.Hair
			}
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

// outfitSays reports whether outfit has word as a word of its own ("chef's jacket" has chef).
func outfitSays(outfit, word string) bool {
	return slices.Contains(strings.FieldsFunc(strings.ToLower(outfit), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-')
	}), strings.ToLower(word))
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
