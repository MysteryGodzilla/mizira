// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"sort"
	"sync"
	"time"
)

// ActiveTool is a tool call running now.
type ActiveTool struct {
	Name  string
	Since time.Time
}

var activity struct {
	sync.Mutex
	seq      uint64
	tools    map[uint64]ActiveTool
	lastDone time.Time
}

// ToolStarted records a tool call; pass its id to ToolEnded when it returns.
func ToolStarted(name string) uint64 {
	activity.Lock()
	defer activity.Unlock()
	if activity.tools == nil {
		activity.tools = map[uint64]ActiveTool{}
	}
	activity.seq++
	activity.tools[activity.seq] = ActiveTool{Name: name, Since: time.Now()}
	return activity.seq
}

func ToolEnded(id uint64) {
	activity.Lock()
	defer activity.Unlock()
	delete(activity.tools, id)
}

// ActiveTools is every tool call running now, oldest first.
func ActiveTools() []ActiveTool {
	activity.Lock()
	out := make([]ActiveTool, 0, len(activity.tools))
	for _, t := range activity.tools {
		out = append(out, t)
	}
	activity.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Since.Before(out[j].Since) })
	return out
}

// LastActive is when a request last finished, or zero before the first.
func LastActive() time.Time {
	activity.Lock()
	defer activity.Unlock()
	return activity.lastDone
}

func requestDone() {
	activity.Lock()
	activity.lastDone = time.Now()
	activity.Unlock()
}
