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
	"B4reMetal/metald/internal/irc"
	"B4reMetal/metald/internal/llm"
)

// console is the bot side of the operator console's own pages (admin.Mizira): every action runs the
// code the matching ~ command runs.
type console struct {
	cfg  *config.Configuration
	sys  core.System
	nets []*config.ServerConfig
	cmds *commands.Registry
}

func (c console) Commands() admin.CheatsheetView {
	v := admin.CheatsheetView{Name: llm.SelfName(c.cfg), NeedName: c.cfg.Bot.CommandsNeedName, Groups: commands.CheatGroups,
		Commands: []admin.CommandView{}}
	if c.cmds == nil {
		return v
	}
	for _, e := range commands.Cheatsheet(c.cmds, c.cfg.Bot.CommandPrefix) {
		v.Commands = append(v.Commands, admin.CommandView{Name: e.Name, Admin: e.Admin, Group: e.Group, Usage: e.Usage,
			Text: e.Text, Feature: e.Feature})
	}
	return v
}

// recentLines is how much of the conversation the page shows; lineChars clips each line.
const (
	recentLines = 30
	lineChars   = 500
)

func (c console) RunState() string { return core.State().String() }

func (c console) Offline() []string { return core.OfflineReasons() }

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
		v := admin.ConversationView{Network: n.Name, Channel: n.Channel,
			MaxContext: c.cfg.Session.MaxContext, LastUsed: session.GetLastUsed().Unix(), Recent: []admin.LineView{}}
		for _, m := range history {
			if m.Role != messages.MessageRoleSystem { // counted and measured as the fold measures them
				v.Messages++
				v.Tokens += core.EstimateTokens(m)
			}
		}
		for _, m := range history[max(0, len(history)-recentLines):] {
			if m.Role != messages.MessageRoleSystem {
				v.Recent = append(v.Recent, admin.LineView{Role: m.Role, Text: clip(m.Content, lineChars), At: core.MessageTime(m)})
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

func (c console) FoldConversation(network, by string) (admin.FoldView, error) {
	n, ok := c.network(network)
	if !ok {
		return admin.FoldView{}, errors.New("no such network")
	}
	session, _, err := c.channelSession(n)
	if err != nil {
		return admin.FoldView{}, err
	}
	r, err := commands.FoldConversation(c.cfg, session, by, core.GetLogger())
	if why := commands.FoldRefusal(err); why != "" {
		return admin.FoldView{}, admin.FoldRefused{Reason: why}
	}
	return admin.FoldView{Folded: r.Folded, Kept: r.Kept, RecapChars: r.RecapChars}, err
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
	return admin.ScreenView{In: append([]string{}, config.List(&c.cfg.Bot.ScreenNicks)...), Out: append([]string{}, config.List(&c.cfg.Bot.FilterNicks)...)}
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
	return commands.ClearSuspicion(network, key, by, core.GetLogger())
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
	return c.clearIfNeeded(settingChange(key)(commands.SetSetting(c.cfg, c.sys, key, value, by, core.GetLogger())))
}

func (c console) ResetSetting(key, by string) (admin.SettingChange, error) {
	return c.clearIfNeeded(settingChange(key)(commands.ResetSetting(c.cfg, c.sys, key, by, core.GetLogger())))
}

// clearIfNeeded wipes each channel conversation after a change that ~set would wipe it for.
func (c console) clearIfNeeded(change admin.SettingChange, err error) (admin.SettingChange, error) {
	if err != nil || !commands.ClearsHistory(change.Key) {
		return change, err
	}
	for _, n := range c.nets {
		if session, _, serr := c.channelSession(n); serr == nil {
			session.Clear()
		}
	}
	return change, err
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

// botNick is the network's configured nick, for the room-memory check.
func (c console) botNick(network string) string {
	if n, ok := c.network(network); ok {
		return n.Nick
	}
	return ""
}

func (c console) MemorySubjects(network string) ([]admin.SubjectView, int, error) {
	store, err := core.Memories()
	if err != nil {
		return nil, 0, err
	}
	counts, err := store.SubjectCounts(network)
	if err != nil {
		return nil, 0, err
	}
	out := []admin.SubjectView{}
	for _, s := range counts {
		out = append(out, admin.SubjectView{Subject: s.Subject, Count: s.Count,
			Room: irc.IsRoomSubject(c.cfg, c.botNick(network), s.Subject)})
	}
	return out, c.cfg.Bot.MemoryPerSubject, nil
}

// memoryPageLimit bounds one listing; a subject is capped well below it.
const memoryPageLimit = 300

func (c console) Memories(network, subject, query string) ([]admin.MemoryView, error) {
	store, err := core.Memories()
	if err != nil {
		return nil, err
	}
	var list []core.Memory
	switch {
	case subject != "":
		list, err = store.Recall(network, subject, memoryPageLimit)
	case query != "":
		list, err = store.SearchWords(network, query, memoryPageLimit)
	default:
		list, err = store.List(network, 100)
	}
	if err != nil {
		return nil, err
	}
	locked, err := store.LockedIDs(network)
	if err != nil {
		return nil, err
	}
	edited, err := store.EditedAt(network)
	if err != nil {
		return nil, err
	}
	out := []admin.MemoryView{}
	for i, m := range list {
		v := admin.MemoryView{ID: m.ID, Subject: m.Subject, Fact: m.Fact, Author: m.Author, Created: m.Created.Unix(),
			Locked: locked[m.ID]}
		if at, ok := edited[m.ID]; ok {
			v.Edited = at.Unix()
		}
		// Repeats saved before merging existed: point each at an older one saying the same.
		for _, older := range list[i+1:] {
			if older.Subject == m.Subject && core.SameFact(m.Subject, m.Fact, older.Fact) {
				v.SameAs = older.ID
				break
			}
		}
		out = append(out, v)
	}
	return out, nil
}

func (c console) AddMemory(network, subject, fact, by string) (int64, bool, error) {
	return commands.OperatorRemember(c.cfg, network, subject, fact, by, core.GetLogger())
}

func (c console) EditMemory(network string, id int64, fact, by string) error {
	return memoryErr(commands.EditMemory(network, id, fact, by, core.GetLogger()))
}

func (c console) ForgetMemory(network string, id int64, by string) error {
	return memoryErr(commands.ForgetMemory(network, id, by, core.GetLogger()))
}

func (c console) LockMemory(network string, id int64, locked bool, by string) error {
	return memoryErr(commands.LockMemory(network, id, locked, by, core.GetLogger()))
}

func memoryErr(err error) error {
	switch {
	case errors.Is(err, commands.ErrNoSuchMemory):
		return admin.ErrNoSuchMemory
	case errors.Is(err, core.ErrMemoryLocked):
		return admin.ErrMemoryLocked
	}
	return err
}

func (c console) SelfNotes(network, status, query string, offset, limit int) ([]admin.SelfNoteView, bool, error) {
	store, err := core.Memories()
	if err != nil {
		return nil, false, err
	}
	notes, more, err := store.SelfNotesPage(network, status, query, offset, limit)
	if err != nil {
		return nil, false, err
	}
	out := []admin.SelfNoteView{}
	for _, n := range notes {
		subject := n.Subject
		if subject == "" {
			subject = strings.ToLower(llm.SelfName(c.cfg))
		}
		v := admin.SelfNoteView{ID: n.ID, Subject: subject, Text: n.Text, Why: n.Why, Status: n.Status, Created: n.Created.Unix(),
			DecidedBy: n.DecidedBy, MemoryID: n.MemoryID}
		if !n.DecidedAt.IsZero() {
			v.DecidedAt = n.DecidedAt.Unix()
		}
		if why, order := irc.LooksLikeInstruction(subject, n.Text); order {
			v.Order = why
		}
		out = append(out, v)
	}
	return out, more, nil
}

func (c console) ApproveSelfNote(network string, id int64, text, by string) (int64, bool, error) {
	memID, merged, err := commands.ApproveSelfNote(c.cfg, network, id, text, by, core.GetLogger())
	return memID, merged, selfNoteErr(err)
}

func (c console) DenySelfNote(network string, id int64, by string) error {
	return selfNoteErr(commands.DenySelfNote(network, id, by, core.GetLogger()))
}

func selfNoteErr(err error) error {
	if errors.Is(err, commands.ErrNotPending) {
		return admin.ErrNotPending
	}
	return err
}

func (c console) CompactPreview(network, subject string) ([]string, []int64, error) {
	n, ok := c.network(network)
	if !ok {
		return nil, nil, errors.New("no such network")
	}
	return llm.CompactPreview(c.cfg.ForNetwork(n), network, subject)
}

func (c console) CompactApply(network, subject string, basedOn []int64, facts []string, by string) (int, error) {
	n, ok := c.network(network)
	if !ok {
		return 0, errors.New("no such network")
	}
	stored, err := commands.ApplyCompaction(c.cfg.ForNetwork(n), network, subject, basedOn, facts, by, core.GetLogger())
	if errors.Is(err, core.ErrMemoriesChanged) {
		return 0, admin.ErrMemoriesChanged
	}
	return stored, err
}

func (c console) SafetyEvents(days, limit int) ([]admin.SafetyEventView, error) {
	out := []admin.SafetyEventView{}
	path := core.LogFilePath(c.cfg.Bot.DataDir, c.cfg.Bot.LogFile)
	if path == "" {
		return out, nil
	}
	events, err := core.ReadSafetyEvents(path, c.cfg.Bot.LogKeep, time.Now().AddDate(0, 0, -days), limit)
	if err != nil {
		return nil, err
	}
	for _, e := range events {
		out = append(out, admin.SafetyEventView{Time: e.Time.Unix(), Kind: e.Kind, Event: e.Event, Who: e.Who,
			Channel: e.Channel, Detail: e.Detail, Suspicion: e.Suspicion, Message: e.Message})
	}
	return out, nil
}
