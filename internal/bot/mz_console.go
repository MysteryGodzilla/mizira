// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package bot

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alexschlessinger/pollytool/messages"
	"github.com/alexschlessinger/pollytool/sessions"

	"B4reMetal/metald/internal/admin"
	"B4reMetal/metald/internal/commands"
	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
)

// console is the bot side of the operator console's own pages (admin.Mizira): every action runs the
// code the matching ~ command runs.
type console struct {
	cfg  *config.Configuration
	sys  core.System
	nets []*config.ServerConfig
}

// recentLines is how much of the conversation the page shows; lineChars clips each line.
const (
	recentLines = 30
	lineChars   = 500
)

func (c console) RunState() string { return core.State().String() }

func (c console) SetRunState(state, by string) (string, bool, int) {
	to, ok := core.ParseRunState(state)
	if !ok {
		return core.State().String(), false, 0
	}
	change := commands.ChangeRunState(to, by, "", core.GetLogger())
	return change.Previous.String(), change.Changed, change.Cancelled
}

func (c console) Tools() []string {
	var names []string
	if reg := c.sys.GetToolRegistry(); reg != nil {
		for _, t := range reg.All() {
			names = append(names, t.GetName())
		}
	}
	return names
}

// Thinking is off for "off" (nothing sent) and "none" (reasoning turned off at the model).
func (c console) Thinking() bool {
	effort := strings.ToLower(c.cfg.Model.ThinkingEffort)
	return effort != "" && effort != "off" && effort != "none"
}

// network finds a network's server settings by name.
func (c console) network(name string) (*config.ServerConfig, bool) {
	for _, n := range c.nets {
		if n.Name == name {
			return n, true
		}
	}
	return nil, false
}

// channelSession is the channel conversation on n, keyed the way a chat request keys it.
func (c console) channelSession(n *config.ServerConfig) (sessions.Session, string, error) {
	key := core.ScopeKey(n.Name, n.Channel)
	session, err := c.sys.GetSessionStore().Get(key)
	return session, key, err
}

func (c console) Conversations() []admin.ConversationView {
	out := []admin.ConversationView{}
	for _, n := range c.nets {
		session, key, err := c.channelSession(n)
		if err != nil {
			continue
		}
		history := session.GetHistory()
		v := admin.ConversationView{Network: n.Name, Channel: n.Channel, Messages: len(history),
			MaxContext: c.cfg.Session.MaxContext, LastUsed: session.GetLastUsed().Unix(), Recent: []admin.LineView{}}
		for _, m := range history {
			v.Tokens += sessions.EstimateTokens(m)
		}
		for _, m := range history[max(0, len(history)-recentLines):] {
			if m.Role != messages.MessageRoleSystem {
				v.Recent = append(v.Recent, admin.LineView{Role: m.Role, Text: clip(m.Content, lineChars)})
			}
		}
		if o, ok := core.Prompts().Get(key); ok {
			v.Persona, v.PersonaBy = o.Prompt, o.Source
		}
		if db, err := core.Context(); err == nil {
			v.Recap = db.Recap(key)
		}
		out = append(out, v)
	}
	return out
}

func (c console) ResetConversation(network, by string) (admin.ResetView, error) {
	n, ok := c.network(network)
	if !ok {
		return admin.ResetView{}, errors.New("no such network")
	}
	session, key, err := c.channelSession(n)
	if err != nil {
		return admin.ResetView{}, err
	}
	r := commands.ResetConversation(c.cfg, c.sys, session, key, "", by, core.GetLogger())
	return admin.ResetView{PersonaCleared: r.PersonaCleared, ModelRestored: r.ModelRestored, Cancelled: r.Cancelled}, nil
}

func (c console) ClearRecap(network, by string) (bool, error) {
	n, ok := c.network(network)
	if !ok {
		return false, errors.New("no such network")
	}
	return commands.ClearRecap(core.ScopeKey(n.Name, n.Channel), by, core.GetLogger())
}

func (c console) Ignores() []admin.IgnoreView {
	out := []admin.IgnoreView{}
	for _, n := range c.nets {
		for _, e := range core.Ignores().List(n.Name) {
			out = append(out, admin.IgnoreView{Network: n.Name, Nick: e.Nick, Until: e.Expiry.Unix(),
				Kind: e.Kind, By: e.By, Reason: e.Reason})
		}
	}
	return out
}

func (c console) Ignore(network, nick string, d time.Duration, reason, by string) (time.Time, error) {
	n, ok := c.network(network)
	if !ok {
		return time.Time{}, errors.New("no such network")
	}
	until, err := commands.IgnoreNick(c.cfg, n.Name, n.Nick, nick, d, reason, by, "", core.GetLogger())
	return until, refusal(err, nick, "ignore")
}

func (c console) Unignore(network, nick, by string) bool {
	return commands.UnignoreNick(network, nick, by, core.GetLogger())
}

func (c console) Screened() admin.ScreenView {
	return admin.ScreenView{In: append([]string{}, c.cfg.Bot.ScreenNicks...), Out: append([]string{}, c.cfg.Bot.FilterNicks...)}
}

func (c console) Screen(network, nick, by string) (int, error) {
	n, ok := c.network(network)
	if !ok {
		return 0, errors.New("no such network")
	}
	session, _, err := c.channelSession(n)
	if err != nil {
		return 0, err
	}
	dropped, err := commands.ScreenNick(c.cfg, session, n.Nick, nick, by, core.GetLogger())
	return dropped, refusal(err, nick, "screen")
}

func (c console) Unscreen(nick, by string) bool {
	return commands.UnscreenNick(c.cfg, nick, by, core.GetLogger())
}

func (c console) Suspicion() ([]admin.ScoreView, float64) {
	out := []admin.ScoreView{}
	for _, n := range c.nets {
		for _, s := range core.Suspicions().Snapshot(n.Name) {
			out = append(out, admin.ScoreView{Network: n.Name, Key: s.Nick, Score: s.Score})
		}
	}
	return out, core.SuspicionQuarantine
}

func (c console) ClearSuspicion(network, key, by string) bool {
	if core.Suspicions().Score(network, key) == 0 {
		return false
	}
	core.Suspicions().Clear(network, key)
	core.GetLogger().Info("suspicion_cleared", "network", network, "key", key, "by", by)
	return true
}

// refusal words a shared command's refusal for the page.
func refusal(err error, nick, action string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, commands.ErrIsAdmin):
		return fmt.Errorf("%s is an admin; admins are exempt from %s", nick, action)
	case errors.Is(err, commands.ErrIsSelf):
		return fmt.Errorf("%s is the bot itself", nick)
	case errors.Is(err, commands.ErrAlreadyScreened):
		return fmt.Errorf("%s is already screened", nick)
	}
	return err
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

func (c console) Bots() admin.BotsView {
	b := c.cfg.Bot
	return admin.BotsView{Nicks: append([]string{}, b.BotNicks...), Prefixes: append([]string{}, b.BotPrefixes...),
		ReplyLimit: strconv.Itoa(b.BotReplyLimit), Cooldown: b.BotCooldown.String()}
}

func (c console) ChangeBots(add bool, kind, value, by string) (string, error) {
	botNick := ""
	if len(c.nets) > 0 {
		botNick = c.nets[0].Nick
	}
	stored, err := commands.ChangeBots(c.cfg, botNick, add, commands.BotKind(kind), value, by, core.GetLogger())
	switch {
	case errors.Is(err, commands.ErrAlreadyListed):
		return stored, fmt.Errorf("%s is already a bot %s", stored, kind)
	case errors.Is(err, commands.ErrNotListed):
		return stored, fmt.Errorf("%s wasn't a bot %s", stored, kind)
	}
	return stored, err
}

func (c console) Settings() []admin.SettingView {
	out := []admin.SettingView{}
	for _, s := range commands.Settings(c.cfg) {
		out = append(out, admin.SettingView{Key: s.Key, Value: s.Value, Default: s.Default, Overridden: s.Overridden, Editable: s.Editable})
	}
	return out
}

func (c console) SetSetting(key, value, by string) (admin.SettingChange, error) {
	return settingChange(key)(commands.SetSetting(c.cfg, c.sys, key, value, by, core.GetLogger()))
}

func (c console) ResetSetting(key, by string) (admin.SettingChange, error) {
	return settingChange(key)(commands.ResetSetting(c.cfg, c.sys, key, by, core.GetLogger()))
}

// settingChange sorts a saved-with-a-warning result from a refusal.
func settingChange(key string) func(string, error) (admin.SettingChange, error) {
	return func(value string, err error) (admin.SettingChange, error) {
		switch {
		case errors.Is(err, commands.ErrLLMNotUpdated):
			return admin.SettingChange{Key: key, Value: value, Warning: err.Error()}, nil
		case err != nil:
			return admin.SettingChange{}, err
		}
		return admin.SettingChange{Key: key, Value: value}, nil
	}
}
