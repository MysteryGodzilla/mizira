// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package commands

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
)

var ErrNoSuchMemory = errors.New("no such memory")

// maxFact bounds one fact typed into the console, like a line typed in IRC.
const maxFact = 400

// OperatorRemember saves a fact the operator wrote in the console. It is an admin's own words, so
// like an admin's behaviour notes it skips the instruction check and the memory policy, but the
// per-subject cap and the merge of repeats apply as they do in chat.
func OperatorRemember(cfg *config.Configuration, network, subject, fact, by string, log *slog.Logger) (id int64, merged bool, err error) {
	subject, fact = strings.TrimSpace(subject), strings.TrimSpace(fact)
	if subject == "" || fact == "" {
		return 0, false, errors.New("give a subject and a fact")
	}
	if len(fact) > maxFact {
		return 0, false, fmt.Errorf("the fact is too long (%d characters, limit %d)", len(fact), maxFact)
	}
	store, err := core.Memories()
	if err != nil {
		return 0, false, err
	}
	_, repeat, _ := store.Similar(network, subject, fact)
	if limit := cfg.Bot.MemoryPerSubject; limit > 0 && !repeat {
		if n, err := store.CountSubject(network, subject); err == nil && n >= int64(limit) {
			return 0, false, fmt.Errorf("%s already has %d memories; forget or compact one first", subject, n)
		}
	}
	id, merged, err = store.RememberMerged(network, subject, fact, by, cfg.Server.Channel)
	if err != nil {
		return 0, false, err
	}
	event := "memory_remembered"
	if merged {
		event = "memory_merged"
	}
	log.Info(event, "id", id, "subject", subject, "author", by, "fact", fact)
	return id, merged, nil
}

// EditMemory rewrites one memory's fact.
func EditMemory(network string, id int64, fact, by string, log *slog.Logger) error {
	fact = strings.TrimSpace(fact)
	if fact == "" || len(fact) > maxFact {
		return fmt.Errorf("a fact is 1 to %d characters", maxFact)
	}
	store, err := core.Memories()
	if err != nil {
		return err
	}
	old, ok, err := store.Get(network, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoSuchMemory
	}
	if _, err := store.Update(network, id, fact); err != nil {
		return err
	}
	log.Info("memory_edited", "id", id, "subject", old.Subject, "from", old.Fact, "to", fact, "by", by)
	return nil
}

// ForgetMemory deletes one memory, as "+memories forget <id>" does for an admin.
func ForgetMemory(network string, id int64, by string, log *slog.Logger) error {
	store, err := core.Memories()
	if err != nil {
		return err
	}
	old, ok, err := store.Get(network, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoSuchMemory
	}
	if store.IsLocked(network, id) {
		return core.ErrMemoryLocked
	}
	if _, err := store.Forget(network, id); err != nil {
		return err
	}
	log.Info("memory_forgotten", "id", id, "subject", old.Subject, "fact", old.Fact, "by", by)
	return nil
}

// LockMemory locks or unlocks one memory. Locked, it survives every forget from IRC, and the
// console forgets it only once unlocked.
func LockMemory(network string, id int64, locked bool, by string, log *slog.Logger) error {
	store, err := core.Memories()
	if err != nil {
		return err
	}
	old, ok, err := store.Get(network, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNoSuchMemory
	}
	event := "memory_unlocked"
	if locked {
		event = "memory_locked"
		_, err = store.Lock(network, id, by)
	} else {
		_, err = store.Unlock(network, id)
	}
	if err != nil {
		return err
	}
	log.Info(event, "id", id, "subject", old.Subject, "fact", old.Fact, "by", by)
	return nil
}

// ApplyCompaction replaces every memory about subject with the operator's reviewed list, if the
// subject still holds exactly the memories the preview was based on. The list is the operator's own
// words now, held to the same per-subject cap and fact length as any save.
func ApplyCompaction(cfg *config.Configuration, network, subject string, basedOn []int64, facts []string, by string, log *slog.Logger) (int, error) {
	var kept []string
	for _, f := range facts {
		if f = strings.TrimSpace(f); f == "" {
			continue
		}
		if len(f) > maxFact {
			return 0, fmt.Errorf("a fact is too long (%d characters, limit %d)", len(f), maxFact)
		}
		kept = append(kept, f)
	}
	if len(kept) == 0 {
		return 0, errors.New("the list is empty; forget the memories instead if that's what you want")
	}
	if limit := cfg.Bot.MemoryPerSubject; limit > 0 && len(kept) > limit {
		return 0, fmt.Errorf("%d facts is over the limit of %d", len(kept), limit)
	}
	store, err := core.Memories()
	if err != nil {
		return 0, err
	}
	stored, err := store.ReplaceSubject(network, subject, basedOn, kept, "compaction:"+by, cfg.Server.Channel)
	if err != nil {
		return 0, err
	}
	log.Info("memory_compacted", "subject", subject, "before", len(basedOn), "after", stored, "by", by)
	return stored, nil
}
