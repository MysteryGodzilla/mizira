// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunPodBalanceAndWorkers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/graphql":
			fmt.Fprint(w, `{"data":{"myself":{"clientBalance":8.06,"currentSpendPerHr":0.89,"spendLimit":80}}}`)
		case "/v2/ep1/health":
			fmt.Fprint(w, `{"workers":{"idle":2,"running":1,"initializing":0}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	oldG, oldA := runpodGraphQL, runpodAPI
	runpodGraphQL, runpodAPI = srv.URL+"/graphql", srv.URL+"/v2"
	t.Cleanup(func() { runpodGraphQL, runpodAPI = oldG, oldA })

	acct, err := RunPodBalance(context.Background(), "k1")
	if err != nil || acct.Balance != 8.06 || acct.SpendPerHr != 0.89 || acct.SpendLimit != 80 {
		t.Errorf("balance = %+v, %v", acct, err)
	}
	if _, err := RunPodBalance(context.Background(), "wrong"); err == nil {
		t.Error("a refused key gave a balance")
	}
	w, err := RunPodWorkers(context.Background(), "k1", "ep1")
	if err != nil || w["idle"] != 2 || w["running"] != 1 {
		t.Errorf("workers = %v, %v", w, err)
	}
}

func TestReadImageJobsAndUses(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 18, 16, 0, 0, 0, time.Local)
	log := strings.Join([]string{
		"2026-10-18 14:32:25 [image_gen] job a done: delay_ms=1061 execution_ms=42970 gpu_seconds=43.0",
		"2026-10-18 14:32:25 [image_gen] encoder safety verdict: 'SAFE'",
		"2026-10-02 09:00:00 [image_gen] job b done: delay_ms=10 execution_ms=20000 gpu_seconds=20.0",
		"2026-09-30 09:00:00 [image_gen] job c done: delay_ms=10 execution_ms=20000 gpu_seconds=20.0",
		"2026-10-18 10:02:55 [websearch] ok 'lamb' -> 3 results, cost=$0.007",
	}, "\n")
	path := filepath.Join(dir, "tools.log")
	if err := os.WriteFile(path, []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	j, err := ReadImageJobs(path, now)
	if err != nil || j.TodayJobs != 1 || j.MonthJobs != 2 || j.TodayGPU != 43*time.Second || j.MonthGPU != 63*time.Second {
		t.Errorf("jobs = %+v, %v", j, err)
	}

	usage := filepath.Join(dir, "usage-imagegen.json")
	body := fmt.Sprintf(`{"uses": [[%d, "alice"], [%d, "bob"]]}`, now.Add(-time.Hour).Unix(), now.Add(-30*time.Hour).Unix())
	if err := os.WriteFile(usage, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if n, err := ImageUses(usage, now); err != nil || n != 1 {
		t.Errorf("uses = %d, %v", n, err)
	}
	if n, err := ImageUses(filepath.Join(dir, "none.json"), now); err != nil || n != 0 {
		t.Errorf("missing file = %d, %v", n, err)
	}
}
