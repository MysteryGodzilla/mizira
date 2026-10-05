// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

// The ~ commands' work without the chat, shared with the operator console so both do exactly the
// same thing. Each takes what a command would read from its chat context as plain arguments.

import (
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/alexschlessinger/pollytool/sessions"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/llm"
)

var (
	ErrIsAdmin         = errors.New("is an admin")
	ErrIsSelf          = errors.New("is the bot itself")
	ErrAlreadyScreened = errors.New("is already screened")
)

// isAdmin reports whether nick belongs to a configured admin, by matching the nick portion of each
// admin hostmask (nick!ident@host).
func isAdmin(cfg *config.Configuration, nick string) bool {
	for _, mask := range cfg.Bot.Admins {
		adminNick, _, found := strings.Cut(mask, "!")
		if !found {
			adminNick = mask
		}
		if strings.EqualFold(adminNick, nick) {
			return true
		}
	}
	return false
}

// IgnoreNick is "+ignore <nick> [duration] [reason]": mute nick on network until the returned time
// and cancel what they have running (except keepRequest, the request asking). Admins are exempt
// from the ignore filter, so an entry for one is refused rather than silently doing nothing.
func IgnoreNick(cfg *config.Configuration, network, botNick, nick string, d time.Duration, reason, by, keepRequest string, log *slog.Logger) (time.Time, error) {
	if isAdmin(cfg, nick) {
		return time.Time{}, ErrIsAdmin
	}
	if strings.EqualFold(nick, botNick) {
		return time.Time{}, ErrIsSelf
	}
	expiry := core.Ignores().AddWithInfo(network, nick, d, core.IgnoreInfo{Kind: core.IgnoreByAdmin, By: by, Reason: reason})
	// An ignore that leaves their current request running would answer them one more time after
	// being told they were ignored.
	core.Requests().CancelSource(nick, keepRequest)
	log.Info("ignore_added", "nick", nick, "duration", d.String(), "until", expiry.UTC(), "reason", reason, "by", by)
	return expiry, nil
}

// UnignoreNick is "+ignore remove <nick>"; false if nick wasn't ignored.
func UnignoreNick(network, nick, by string, log *slog.Logger) bool {
	if !core.Ignores().Remove(network, nick) {
		return false
	}
	log.Info("ignore_removed", "nick", nick, "by", by)
	return true
}

// ScreenNick is "+screen <nick>": gate nick's messages and check the replies to them. Their earlier
// turns in session were never screened, so they are dropped; the count is returned.
func ScreenNick(cfg *config.Configuration, session sessions.Session, botNick, nick, by string, log *slog.Logger) (int, error) {
	if isAdmin(cfg, nick) {
		return 0, ErrIsAdmin
	}
	if strings.EqualFold(nick, botNick) {
		return 0, ErrIsSelf
	}
	configMu.Lock()
	bot := cfg.Bot
	addedIn := addNick(&bot.ScreenNicks, nick)
	addedOut := addNick(&bot.FilterNicks, nick)
	if addedIn || addedOut {
		PersistScreening(bot.ScreenNicks, bot.FilterNicks)
	}
	configMu.Unlock()
	if !addedIn && !addedOut {
		return 0, ErrAlreadyScreened
	}
	// Nothing already planted keeps steering the model.
	dropped := 0
	if session != nil {
		dropped = core.QuarantineSpeaker(session, nick)
	}
	log.Info("screen_added", "nick", nick, "quarantined", dropped, "by", by)
	return dropped, nil
}

// UnscreenNick is "+screen remove <nick>"; false if nick wasn't screened.
func UnscreenNick(cfg *config.Configuration, nick, by string, log *slog.Logger) bool {
	configMu.Lock()
	bot := cfg.Bot
	removedIn := removeNick(&bot.ScreenNicks, nick)
	removedOut := removeNick(&bot.FilterNicks, nick)
	if removedIn || removedOut {
		PersistScreening(bot.ScreenNicks, bot.FilterNicks)
	}
	configMu.Unlock()
	if !removedIn && !removedOut {
		return false
	}
	log.Info("screen_removed", "nick", nick, "by", by)
	return true
}

// ResetResult is what ResetConversation did.
type ResetResult struct {
	PersonaCleared bool
	ModelRestored  string // the model now in use, if a +models switch was undone
	Cancelled      int
	LockWasHeld    bool
}

// ResetConversation is "+reset" for the conversation in session (lock key lockKey): the persona
// goes, history and backlog are cleared, the model returns to config.yml's, and requests running
// in it are cancelled (except keepRequest). The recap survives: it can hold days of context, and
// only an admin clears it (+recap clear).
func ResetConversation(cfg *config.Configuration, sys core.System, session sessions.Session, lockKey, keepRequest, by string, log *slog.Logger) ResetResult {
	var r ResetResult
	// Before Clear, so the rebuilt history starts from the operator's prompt.
	r.PersonaCleared = restoreOperatorPrompt(cfg, session, lockKey, by, log)
	session.Clear()
	core.Backlog().Clear(session.GetName())
	// Back to the default model as well as a clean history, so one command returns the bot to a
	// known-good state.
	r.ModelRestored = restoreDefaultModel(cfg, sys, by, log)
	r.Cancelled = core.Requests().CancelKey(lockKey, keepRequest)
	r.LockWasHeld = core.ResetRequestLock(lockKey)
	log.Info("session_reset", "source", by, "lock_key", lockKey, "lock_was_held", r.LockWasHeld)
	return r
}

// ClearRecap is "+recap clear"; false if there was none.
func ClearRecap(key, by string, log *slog.Logger) (bool, error) {
	db, err := core.Context()
	if err != nil {
		return false, err
	}
	if !db.ClearRecap(key) {
		return false, nil
	}
	log.Info("recap_cleared", "key", key, "by", by)
	return true, nil
}

// FoldConversation is "+recap fold": the older turns go into the recap now, without waiting for the
// conversation to go idle or outgrow maxcontext.
func FoldConversation(cfg *config.Configuration, session sessions.Session, by string, log *slog.Logger) (llm.FoldResult, error) {
	log.Info("recap_fold_requested", "key", session.GetName(), "by", by)
	return llm.FoldNow(cfg, session)
}
