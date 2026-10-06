// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
	"B4reMetal/metald/internal/llm"
)

var ErrNotPending = errors.New("isn't waiting for a decision")

// ApproveSelfNote saves a pending note as a memory about its subject (room memory, for a note about
// the bot), as worded or with the admin's edit (text non-empty), and marks it approved. It returns
// the memory's id and whether that merged into a fact already held.
func ApproveSelfNote(cfg *config.Configuration, network string, id int64, text, by string, log *slog.Logger) (int64, bool, error) {
	store, err := core.Memories()
	if err != nil {
		return 0, false, err
	}
	note, ok, err := store.SelfNote(network, id)
	if err != nil {
		return 0, false, err
	}
	if !ok || note.Status != core.SelfNotePending {
		return 0, false, ErrNotPending
	}
	if text = strings.TrimSpace(text); text == "" {
		text = note.Text
	}
	subject, author := note.Subject, by
	if bot := strings.ToLower(llm.SelfName(cfg)); subject == "" || subject == bot {
		subject = bot
	} else {
		author = "fold:" + by // a people-note: what the chat showed, approved by the operator
	}
	memID, merged, err := OperatorRemember(cfg, network, subject, text, author, log)
	if err != nil {
		return 0, false, err
	}
	if decided, err := store.DecideSelfNote(network, id, core.SelfNoteApproved, text, by, memID); err != nil || !decided {
		return memID, merged, fmt.Errorf("saved as #%d, but the note was decided meanwhile", memID)
	}
	log.Info("selfnote_approved", "id", id, "memory", memID, "edited", text != note.Text, "text", text, "by", by)
	return memID, merged, nil
}

// DenySelfNote marks a pending self-note denied. It stays recorded so it isn't proposed again.
func DenySelfNote(network string, id int64, by string, log *slog.Logger) error {
	store, err := core.Memories()
	if err != nil {
		return err
	}
	note, ok, err := store.SelfNote(network, id)
	if err != nil {
		return err
	}
	if !ok || note.Status != core.SelfNotePending {
		return ErrNotPending
	}
	if decided, err := store.DecideSelfNote(network, id, core.SelfNoteDenied, note.Text, by, 0); err != nil || !decided {
		return ErrNotPending
	}
	log.Info("selfnote_denied", "id", id, "text", note.Text, "by", by)
	return nil
}

// SelfNotesCommand handles +selfnotes: the notes the bot proposed about itself and the people it
// chats with, waiting for an admin.
// The console's Memories tab does the same with more room.
type SelfNotesCommand struct{}

func (c *SelfNotesCommand) Name() string    { return "+selfnotes" }
func (c *SelfNotesCommand) AdminOnly() bool { return true }

// selfNotesShown bounds one listing in the channel.
const selfNotesShown = 4

const selfNotesUsage = "Usage: +selfnotes | +selfnotes approve <id> [new wording] | +selfnotes deny <id>"

func (c *SelfNotesCommand) Execute(ctx irc.ChatContextInterface) {
	args := ctx.GetArgs()
	if len(args) < 2 || args[1] == "list" {
		listSelfNotes(ctx)
		return
	}
	if len(args) < 3 || (args[1] != "approve" && args[1] != "deny") {
		ctx.Reply(selfNotesUsage)
		return
	}
	id, err := strconv.ParseInt(strings.Trim(args[2], "#[]"), 10, 64)
	if err != nil {
		ctx.Reply(selfNotesUsage)
		return
	}
	if args[1] == "deny" {
		if err := DenySelfNote(ctx.GetNetwork(), id, ctx.GetSource(), ctx.GetLogger()); err != nil {
			ctx.Reply(fmt.Sprintf("Note #%d %s.", id, refusalText(err)))
			return
		}
		ctx.Reply(fmt.Sprintf("Denied note #%d.", id))
		return
	}
	memID, merged, err := ApproveSelfNote(ctx.GetConfig(), ctx.GetNetwork(), id, strings.Join(args[3:], " "),
		ctx.GetSource(), ctx.GetLogger())
	switch {
	case err != nil:
		ctx.Reply(fmt.Sprintf("Note #%d %s.", id, refusalText(err)))
	case merged:
		ctx.Reply(fmt.Sprintf("Approved #%d; I already knew that, so memory #%d keeps one copy.", id, memID))
	default:
		ctx.Reply(fmt.Sprintf("Approved #%d; saved as memory #%d.", id, memID))
	}
}

func refusalText(err error) string {
	if errors.Is(err, ErrNotPending) {
		return "isn't waiting for a decision"
	}
	return "wasn't saved: " + err.Error()
}

func listSelfNotes(ctx irc.ChatContextInterface) {
	store, err := core.Memories()
	if err != nil {
		ctx.Reply("memory is unavailable right now")
		return
	}
	notes, err := store.SelfNotes(ctx.GetNetwork(), core.SelfNotePending, 50)
	if err != nil {
		ctx.Reply("memory is unavailable right now")
		return
	}
	if len(notes) == 0 {
		ctx.Reply("No notes waiting.")
		return
	}
	for i, n := range notes {
		if i == selfNotesShown {
			ctx.Reply(fmt.Sprintf("...and %d more (the console's Memories tab lists them all)", len(notes)-i))
			break
		}
		line := fmt.Sprintf("[%d] %s", n.ID, n.Text)
		if n.Why != "" {
			line += fmt.Sprintf(" (from: %q)", n.Why)
		}
		ctx.Reply(line)
	}
	ctx.Reply(selfNotesUsage)
}
