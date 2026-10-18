// Copyright (C) 2026 BareMetal
// Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/alexschlessinger/pollytool/tools"
)

// ShellToolRequirements asks a shell tool for its schema and returns the
// "requires" list: environment variables it cannot work without.
// ShellToolMeta is what the bot reads from a plugin's --schema beyond the
// tool definition itself.
type ShellToolMeta struct {
	Requires []string `json:"requires"`
	// Announce false keeps "calling <tool>" out of the channel, for tools whose
	// output is itself the visible effect.
	Announce *bool `json:"announce"`
	// Requester true has the bot pass the asking nick as "requested_by".
	Requester bool `json:"requester"`
	// Announcer true has the bot run "<tool> --announce" when the tool is called and post the
	// "action: " line it prints instead of "calling <tool>".
	Announcer bool `json:"announcer"`
	// NoBots true refuses calls made for another bot's line.
	NoBots bool `json:"nobots"`
}

// ReadShellToolMeta runs a plugin's --schema and returns its metadata.
func ReadShellToolMeta(command string) (ShellToolMeta, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var meta ShellToolMeta
	out, err := tools.ScriptCommand(ctx, command, "--schema").Output()
	if err != nil {
		return meta, err
	}
	err = json.Unmarshal(out, &meta)
	return meta, err
}

var quietTools sync.Map

// SetQuietTool records that a tool's calls are not announced in the channel.
func SetQuietTool(name string) { quietTools.Store(name, true) }

// QuietTool reports whether a tool's calls are not announced in the channel.
func QuietTool(name string) bool {
	_, ok := quietTools.Load(name)
	return ok
}

var announcers sync.Map

// announceTimeout bounds a tool's --announce, which runs before the tool does.
const announceTimeout = 10 * time.Second

// SetAnnouncer records the command that prints a tool's start line.
func SetAnnouncer(name, command string) { announcers.Store(name, command) }

// Announcement runs a tool's --announce and returns what it printed; false when the tool has no
// announcer or it failed.
func Announcement(name string) (string, bool) {
	command, ok := announcers.Load(name)
	if !ok {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), announceTimeout)
	defer cancel()
	out, err := tools.ScriptCommand(ctx, command.(string), "--announce").Output()
	if err != nil {
		GetLogger().Warn("tool_announce_failed", "tool", name, "error", err)
		return "", false
	}
	return string(out), true
}

func MissingEnv(keys []string, getenv func(string) string) []string {
	if getenv == nil {
		getenv = os.Getenv
	}
	var missing []string
	for _, k := range keys {
		if strings.TrimSpace(getenv(k)) == "" {
			missing = append(missing, k)
		}
	}
	return missing
}
