// Copyright (C) 2026 BareMetal
// Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportPluginLibPrependsOnce(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PYTHONPATH", "/existing")
	if got := ExportPluginLib(dir); got != dir {
		t.Fatalf("got %q, want %q", got, dir)
	}
	ExportPluginLib(dir)
	want := dir + string(os.PathListSeparator) + "/existing"
	if got := os.Getenv("PYTHONPATH"); got != want {
		t.Fatalf("PYTHONPATH = %q, want %q", got, want)
	}
}

func TestExportPluginLibMissingDirIsANoop(t *testing.T) {
	t.Setenv("PYTHONPATH", "/existing")
	if got := ExportPluginLib(filepath.Join(t.TempDir(), "nope")); got != "" {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(os.Getenv("PYTHONPATH"), "nope") {
		t.Fatal("missing dir must not be exported")
	}
}

func TestExportModelEnv(t *testing.T) {
	for _, k := range []string{"METALD_MODEL_URL", "METALD_MODEL", "METALD_MODEL_KEY"} {
		t.Setenv(k, "")
	}
	ExportModelEnv("http://127.0.0.1:8081/v1/", "openai/gemma-small", "k")
	if os.Getenv("METALD_MODEL_URL") != "http://127.0.0.1:8081/v1" || os.Getenv("METALD_MODEL") != "gemma-small" ||
		os.Getenv("METALD_MODEL_KEY") != "k" {
		t.Errorf("exported %q %q %q", os.Getenv("METALD_MODEL_URL"), os.Getenv("METALD_MODEL"), os.Getenv("METALD_MODEL_KEY"))
	}
}
