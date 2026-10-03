// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/alexschlessinger/pollytool/schema"
	"github.com/alexschlessinger/pollytool/tools"
	"github.com/lrstanley/girc"

	"B4reMetal/metald/internal/core"
)

const (
	// slapCooldown keeps one person from being slapped over and over through the bot (A11).
	slapCooldown  = 2 * time.Minute
	slapDefault   = "a large trout"
	maxSlapObject = 60
)

var (
	slapMu   sync.Mutex
	lastSlap = map[string]time.Time{}
)

func newIrcSlapTool() tools.Tool {
	return &tools.Func{
		Name: "irc__slap",
		Desc: "The old IRC joke: slap someone in the channel with something, sent as a /me action " +
			"(\"slaps bob around a bit with a large trout\"). Use it playfully when someone says " +
			"something silly or asks you to slap someone, or as a light warning before you ignore " +
			"someone who keeps misbehaving. Pick an object that fits the moment (their own GPU, a wet " +
			"noodle), or leave it empty for the classic trout. Once per message at most, and never to " +
			"bully. The action is the joke: don't describe the slap again in your reply.",
		Params: schema.Params{
			"nick":   schema.S("Who to slap: a nick in the channel"),
			"object": schema.S("What to slap them with, a few words; empty for a large trout"),
		},
		Required: []string{"nick"},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, err := validateContext(ctx)
			if err != nil {
				return "", err
			}
			nick := strings.TrimSpace(args.String("nick"))
			if !girc.IsValidNick(nick) || !inChannel(chatCtx, nick) {
				return fmt.Sprintf("Error: %q isn't in the channel, so there's no one to slap.", nick), nil
			}
			// The cooldown stops people pestering someone through the bot; an admin's slap is theirs.
			key := core.ScopeKey(chatCtx.GetNetwork(), girc.ToRFC1459(nick))
			if wait := slapWait(key, time.Now()); wait > 0 && !chatCtx.IsAdmin() {
				return fmt.Sprintf("Not slapping %s again so soon (%s left). Just reply normally.",
					nick, wait.Round(time.Second)), nil
			}

			object := cleanSlapObject(args.String("object"))
			chatCtx.SendAction(chatCtx.GetConfig().Server.Channel,
				fmt.Sprintf("slaps %s around a bit with %s", nick, object))
			chatCtx.GetLogger().Info("irc_slap", "nick", nick, "object", object)
			return fmt.Sprintf("Slapped %s with %s. It's done: at most add one short line, "+
				"without describing the slap again.", nick, object), nil
		},
	}
}

// slapWait reports how long until key may be slapped again, recording a slap when it is zero.
func slapWait(key string, now time.Time) time.Duration {
	slapMu.Lock()
	defer slapMu.Unlock()
	if last, ok := lastSlap[key]; ok && now.Sub(last) < slapCooldown {
		return slapCooldown - now.Sub(last)
	}
	lastSlap[key] = now
	return 0
}

// cleanSlapObject keeps the object to a short plain phrase: no formatting or control codes, no
// links, and no doubled "with". Anything unusable becomes the trout.
func cleanSlapObject(s string) string {
	s = girc.StripRaw(s)
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if lower := strings.ToLower(s); strings.HasPrefix(lower, "with ") {
		s = s[5:]
	}
	if r := []rune(s); len(r) > maxSlapObject {
		s = strings.TrimSpace(string(r[:maxSlapObject]))
	}
	lower := strings.ToLower(s)
	if s == "" || strings.Contains(lower, "://") || strings.Contains(lower, "www.") {
		return slapDefault
	}
	return s
}

func inChannel(chatCtx ChatContextInterface, nick string) bool {
	var nicks []string
	for _, u := range chatCtx.GetChannelUsers(chatCtx.GetConfig().Server.Channel) {
		nicks = append(nicks, u.Nick)
	}
	return NickInList(nick, nicks)
}
