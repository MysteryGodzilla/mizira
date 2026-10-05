// Copyright (C) 2026 BareMetal
// Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"errors"
	"fmt"
	"strings"

	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
	"B4reMetal/metald/internal/llm"
)

// recapInline bounds a recap shown in the channel when there is no paste tool.
const recapInline = 600

// RecapCommand shows, clears or folds this channel's recap: the running summary of its older
// conversation. It skips the request lock; a fold takes the model gate itself.
type RecapCommand struct{}

func (c *RecapCommand) Name() string    { return "+recap" }
func (c *RecapCommand) AdminOnly() bool { return true }

func (c *RecapCommand) Execute(ctx irc.ChatContextInterface) {
	db, err := core.Context()
	if err != nil {
		ctx.GetLogger().Error("context_db_unavailable", "error", err.Error())
		ctx.Reply("the recap is unavailable")
		return
	}
	key := ctx.GetSession().GetName()
	args := ctx.GetArgs()

	if len(args) > 1 {
		switch strings.ToLower(args[1]) {
		case "clear":
		case "fold":
			ctx.Reply("folding the older conversation into the recap...")
			ctx.Reply(foldReply(FoldConversation(ctx.GetConfig(), ctx.GetSession(), ctx.GetSource(), ctx.GetLogger())))
			return
		default:
			ctx.Reply("usage: +recap | +recap clear | +recap fold")
			return
		}
		if cleared, _ := ClearRecap(key, ctx.GetSource(), ctx.GetLogger()); cleared {
			ctx.Reply("recap cleared.")
		} else {
			ctx.Reply("there is no recap to clear.")
		}
		return
	}

	recap := db.Recap(key)
	if recap == "" {
		ctx.Reply("no recap yet: nothing in this channel has been folded.")
		return
	}
	if url := pasteRecap(ctx, recap); url != "" {
		ctx.Reply(fmt.Sprintf("recap (%d chars): %s", len(recap), url))
		return
	}
	shown := recap
	if len(shown) > recapInline {
		shown = shown[:recapInline] + fmt.Sprintf("... (%d more chars)", len(recap)-recapInline)
	}
	for _, line := range strings.Split(shown, "\n") {
		if strings.TrimSpace(line) != "" {
			ctx.Reply(line)
		}
	}
}

// foldReply says what "+recap fold" did.
func foldReply(r llm.FoldResult, err error) string {
	if err == nil {
		return fmt.Sprintf("folded %d messages into the recap (%d chars); kept the last %d.", r.Folded, r.RecapChars, r.Kept)
	}
	if why := FoldRefusal(err); why != "" {
		return why
	}
	return "the fold failed: the recap model call didn't work (see the log)."
}

// FoldRefusal says why a fold didn't run, or "" if it was tried and failed.
func FoldRefusal(err error) string {
	switch {
	case errors.Is(err, llm.ErrNothingToFold):
		return "nothing to fold: the conversation is no longer than the 6 turns a fold keeps."
	case errors.Is(err, llm.ErrFoldPersona):
		return "not folded: a custom persona is active (reset first)."
	case errors.Is(err, llm.ErrFoldBusy):
		return "a fold is already running for this channel."
	case errors.Is(err, llm.ErrFoldChanged):
		return "not folded: the conversation was reset or changed while I was summarising."
	}
	return ""
}

// pasteRecap posts the recap with the paste tool and returns its url, or "" if that is not possible.
func pasteRecap(ctx irc.ChatContextInterface, recap string) string {
	return llm.PasteText(ctx.GetSystem(), recap, ctx.GetLogger())
}
