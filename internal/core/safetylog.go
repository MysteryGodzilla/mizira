// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"time"
)

// SafetyKinds sorts the log's safety events into what the operator console's Safety page shows.
var SafetyKinds = map[string]string{
	"screen_denied":                   "gatekeeper",
	"message_screened_out":            "gatekeeper",
	"reply_screened_out":              "reply",
	"exchange_quarantined":            "quarantine",
	"memory_rejected_unsafe":          "memory",
	"memory_rejected_instruction":     "memory",
	"memory_rejected_room":            "memory",
	"memory_forget_denied":            "memory",
	"backlog_screened_out":            "quoted",
	"backlog_lines_dropped":           "quoted",
	"history_screened_out":            "quoted",
	"tool_refused_request":            "tool",
	"tool_call_denied_after_refusals": "tool",
	"command_denied":                  "command",
	"injection_frames_stripped":       "injection",
	"unvouched_link_dropped":          "injection",
	"ignore_added":                    "ignore",
	"flood_timeout":                   "ignore",
	"harassment_ignored":              "ignore",
	"bot_loop_limit":                  "bots",
	"admin_auth_locked":               "console",
	"console_action":                  "console",
}

// SafetyEvent is one safety event from the log file.
type SafetyEvent struct {
	Time      time.Time
	Kind      string // gatekeeper, reply, quarantine, memory, quoted, tool, command, injection, ignore, bots, console
	Event     string // the log message, e.g. screen_denied
	Who       string // the speaker it concerns (or who acted, for console and ignores)
	Channel   string
	Detail    string // the reason, or what was refused
	Suspicion string
	// Message is the screened text itself (a refused message, or a reply that was held back), shown
	// on request; the detail is the reason.
	Message string
}

// detailKeys are tried in order for an event's detail.
var detailKeys = []string{"reason", "cause", "fact", "message", "tool", "command", "action", "claims"}

// safetyDetailMax bounds one event's detail; messages and facts can be long.
const safetyDetailMax = 300

// safetyMessageMax bounds a screened message shown on request.
const safetyMessageMax = 2000

// ReadSafetyEvents reads the log file at path and its rotated copies (path.1 ... path.keep), newest
// first, and returns safety events since the given time, at most limit.
func ReadSafetyEvents(path string, keep int, since time.Time, limit int) ([]SafetyEvent, error) {
	files := []string{path}
	for i := 1; i <= keep; i++ {
		files = append(files, fmt.Sprintf("%s.%d", path, i))
	}
	var out []SafetyEvent
	for _, f := range files {
		events, oldest, err := readSafetyFile(f, since)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		slices.Reverse(events)
		out = append(out, events...)
		if len(out) >= limit || (!oldest.IsZero() && oldest.Before(since)) {
			break // older files are older still
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// readSafetyFile returns a file's safety events since the given time, oldest first, and the time
// of its first record.
func readSafetyFile(path string, since time.Time) ([]SafetyEvent, time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer f.Close()
	var out []SafetyEvent
	var first time.Time
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if first.IsZero() {
			var head struct {
				Time time.Time `json:"time"`
			}
			if json.Unmarshal(line, &head) == nil {
				first = head.Time
			}
		}
		// Cheap filter before parsing: most lines aren't safety events.
		if !bytes.Contains(line, []byte(`"msg":"`)) || !isSafetyLine(line) {
			continue
		}
		var rec map[string]any
		if json.Unmarshal(line, &rec) != nil {
			continue
		}
		e, ok := safetyEvent(rec)
		if ok && !e.Time.Before(since) {
			out = append(out, e)
		}
	}
	return out, first, sc.Err()
}

func isSafetyLine(line []byte) bool {
	for msg := range SafetyKinds {
		if bytes.Contains(line, []byte(`"msg":"`+msg+`"`)) {
			return true
		}
	}
	return false
}

func safetyEvent(rec map[string]any) (SafetyEvent, bool) {
	str := func(k string) string {
		if v, ok := rec[k]; ok && v != nil {
			if s, ok := v.(string); ok {
				return s
			}
			return fmt.Sprint(v)
		}
		return ""
	}
	msg := str("msg")
	kind, ok := SafetyKinds[msg]
	if !ok {
		return SafetyEvent{}, false
	}
	t, _ := time.Parse(time.RFC3339Nano, str("time"))
	e := SafetyEvent{Time: t, Kind: kind, Event: msg, Who: cmp.Or(str("speaker"), str("source")), Channel: str("channel"), Suspicion: str("suspicion")}
	switch msg {
	case "ignore_added":
		e.Who, e.Detail = str("nick"), fmt.Sprintf("for %s by %s", str("duration"), str("by"))
		if r := str("reason"); r != "" {
			e.Detail += ": " + r
		}
	case "console_action":
		e.Who = str("by")
		e.Detail = str("action")
		for _, k := range []string{"nick", "key", "subject", "tool", "to", "id"} {
			if v := str(k); v != "" {
				e.Detail += " " + k + "=" + v
			}
		}
	case "admin_auth_locked":
		e.Who, e.Detail = str("client"), "locked out after wrong tokens"
	default:
		for _, k := range detailKeys {
			if v := str(k); v != "" {
				e.Detail = v
				break
			}
		}
		if e.Who == "" {
			e.Who = str("author")
		}
	}
	if m := cmp.Or(str("message"), str("reply")); m != "" && m != e.Detail && (e.Kind == "gatekeeper" || e.Kind == "reply") {
		if r := []rune(m); len(r) > safetyMessageMax {
			m = string(r[:safetyMessageMax]) + "…"
		}
		e.Message = m
	}
	if r := []rune(e.Detail); len(r) > safetyDetailMax {
		e.Detail = string(r[:safetyDetailMax]) + "…"
	}
	return e, true
}
