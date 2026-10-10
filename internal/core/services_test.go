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

func TestLlamaSwapStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("%s without the key", r.URL.Path)
		}
		switch r.URL.Path {
		case "/running":
			w.Write([]byte(`{"running":[{"model":"modelA","state":"ready"}]}`))
		case "/api/metrics/stats":
			w.Write([]byte(`{"total_requests":4,"prompt_histogram":{"p50":250.2},"gen_histogram":{"p50":67.7}}`))
		case "/api/performance":
			w.Write([]byte(`{"gpu_stats":[{"temp_c":40,"gpu_util_pct":5,"mem_used_mb":2048,"mem_total_mb":12288,"power_draw_w":30},` +
				`{"temp_c":54,"gpu_util_pct":20,"mem_used_mb":11264,"mem_total_mb":12288,"power_draw_w":137.27}]}`))
		case "/api/version":
			w.Write([]byte(`{"version":"v1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	stats, err := LlamaSwapStats(context.Background(), srv.URL+"/v1/", "k")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range stats {
		got[s.Label] = s.Value
	}
	want := map[string]string{"model": "modelA (ready)", "reply speed": "68 tok/s", "prompt speed": "250 tok/s",
		"requests": "4 since it started", "VRAM": "11.0 / 12.0 GB", "GPU": "20% · 54°C · 137 W", "version": "v1"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

// A plain llama-server (no llama-swap) still shows what it can; an unreachable one is an error.
func TestLlamaSwapStatsPartialAndDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/running" {
			w.Write([]byte(`{"running":[]}`))
			return
		}
		http.NotFound(w, r)
	}))
	stats, err := LlamaSwapStats(context.Background(), srv.URL, "")
	if err != nil || len(stats) != 1 || stats[0].Value != "none" {
		t.Errorf("partial: %v %v", stats, err)
	}
	srv.Close()
	if _, err := LlamaSwapStats(context.Background(), "http://127.0.0.1:1", ""); err == nil {
		t.Error("an unreachable server gave no error")
	}
}

func TestReadSearchSpend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools.log")
	log := strings.Join([]string{
		"2026-09-30 23:59:00 [websearch] ok 'last month' -> 5 results, cost=$0.007",
		"2026-10-01 08:00:00 [websearch] ok 'early' -> 5 results, cost=$0.007",
		"2026-10-09 12:00:00 [webfetch] ok 'https://example.com' -> 900 chars, cost=$0.001",
		"2026-10-10 09:07:11 [websearch] ok 'orange cake recipe' -> 5 results, cost=$0.007",
		"2026-10-10 09:08:00 [websearch] exa http 429: slow down",
		"2026-10-10 09:09:53 [websearch] ok 'vanilla blondies recipe' -> 5 results, cost=$0.007",
		"2026-10-10 09:10:00 [websearch] ok 'no cost reported' -> 5 results, cost=$None",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 10, 20, 0, 0, 0, time.Local)
	s, err := ReadSearchSpend(path, now)
	if err != nil {
		t.Fatal(err)
	}
	if s.TodayCalls != 2 || s.MonthCalls != 4 || fmt.Sprintf("%.3f", s.TodayCost) != "0.014" ||
		fmt.Sprintf("%.3f", s.MonthCost) != "0.022" {
		t.Errorf("spend = %+v", s)
	}
	if s, err := ReadSearchSpend(filepath.Join(t.TempDir(), "none.log"), now); err != nil || s.MonthCalls != 0 {
		t.Errorf("missing log: %+v %v", s, err)
	}
}
