// Copyright (C) 2026 BareMetal
// Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeTool writes a plugin that prints schema for --schema: a shell script, or on Windows, which
// can't run one, a batch file (the schemas used here have no characters cmd treats specially).
func fakeTool(t *testing.T, schema string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "tool.sh")
	body := "#!/bin/sh\n[ \"$1\" = --schema ] && printf '%s' '" + schema + "'\n"
	if runtime.GOOS == "windows" {
		p = filepath.Join(t.TempDir(), "tool.cmd")
		body = "@echo off\r\nif \"%1\"==\"--schema\" echo " + schema + "\r\n"
	}
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestShellToolRequirementsReadsList(t *testing.T) {
	p := fakeTool(t, `{"title":"x","requires":["A_KEY","B_URL"]}`)
	got, err := readRequires(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "A_KEY" || got[1] != "B_URL" {
		t.Errorf("got %v", got)
	}
}

func TestShellToolRequirementsAbsentMeansNone(t *testing.T) {
	p := fakeTool(t, `{"title":"x","sandbox":{"allowNetwork":true}}`)
	got, err := readRequires(p)
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v", got, err)
	}
}

func TestShellToolRequirementsBadOutputIsAnError(t *testing.T) {
	p := fakeTool(t, `not json`)
	// The error must come from the output, not from failing to run the tool.
	_, err := readRequires(p)
	var syntax *json.SyntaxError
	if !errors.As(err, &syntax) {
		t.Errorf("want a JSON error from the output, got %v", err)
	}
}

func TestMissingEnvTreatsBlankAsMissing(t *testing.T) {
	env := map[string]string{"SET": "x", "BLANK": "  "}
	got := MissingEnv([]string{"SET", "BLANK", "UNSET"}, func(k string) string { return env[k] })
	if len(got) != 2 || got[0] != "BLANK" || got[1] != "UNSET" {
		t.Errorf("got %v", got)
	}
}

func readRequires(p string) ([]string, error) {
	meta, err := ReadShellToolMeta(p)
	return meta.Requires, err
}

func TestReadShellToolMetaAnnounce(t *testing.T) {
	quiet, _ := ReadShellToolMeta(fakeTool(t, `{"title":"x","announce":false}`))
	if quiet.Announce == nil || *quiet.Announce {
		t.Fatalf("announce:false not read: %+v", quiet.Announce)
	}
	plain, _ := ReadShellToolMeta(fakeTool(t, `{"title":"x"}`))
	if plain.Announce != nil {
		t.Fatal("absent announce must stay nil (announced by default)")
	}
}

func TestQuietTools(t *testing.T) {
	SetQuietTool("slap__slap")
	if !QuietTool("slap__slap") || QuietTool("websearch__web_search") {
		t.Fatal("quiet tool set is wrong")
	}
}

// announceTool writes a plugin that prints line for --announce, or fails when line is empty.
func announceTool(t *testing.T, line string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "tool.sh")
	body := "#!/bin/sh\n[ \"$1\" = --announce ] && [ -n '" + line + "' ] && echo '" + line + "' && exit 0\nexit 1\n"
	if runtime.GOOS == "windows" {
		p = filepath.Join(t.TempDir(), "tool.cmd")
		body = "@echo off\r\nif \"%1\"==\"--announce\" if not \"" + line + "\"==\"\" (echo " + line + "& exit /b 0)\r\nexit /b 1\r\n"
	}
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAnnouncementRunsTheTool(t *testing.T) {
	SetAnnouncer("announcetest__ok", announceTool(t, "action: is drawing"))
	got, ok := Announcement("announcetest__ok", `{"a":1}`)
	if !ok || strings.TrimSpace(got) != "action: is drawing" {
		t.Fatalf("got %q, %v", got, ok)
	}
}

func TestAnnouncementFailsWithoutAnnouncer(t *testing.T) {
	if _, ok := Announcement("announcetest__none", ""); ok {
		t.Fatal("a tool without an announcer announced")
	}
	SetAnnouncer("announcetest__broken", announceTool(t, ""))
	if _, ok := Announcement("announcetest__broken", ""); ok {
		t.Fatal("a failing --announce counted as an announcement")
	}
}

func TestReadShellToolMetaAnnouncer(t *testing.T) {
	meta, err := ReadShellToolMeta(fakeTool(t, `{"title":"x","announcer":true}`))
	if err != nil || !meta.Announcer {
		t.Fatalf("announcer not read: %+v, %v", meta, err)
	}
}
