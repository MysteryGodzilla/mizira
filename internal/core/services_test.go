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

// The log's times are local; the credit month is UTC. At UTC+11, 08:00 on the 1st is still the
// previous month for Exa, and "today" is the local day.
func TestReadSearchSpend(t *testing.T) {
	saved := time.Local
	time.Local = time.FixedZone("UTC+11", 11*3600)
	t.Cleanup(func() { time.Local = saved })

	path := filepath.Join(t.TempDir(), "tools.log")
	log := strings.Join([]string{
		"2026-09-30 23:59:00 [websearch] ok 'last month' -> 5 results, cost=$0.007",
		"2026-10-01 08:00:00 [websearch] ok 'still september in utc' -> 5 results, cost=$0.007",
		"2026-10-01 12:00:00 [websearch] ok 'october in utc' -> 5 results, cost=$0.007",
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
	if want := time.Date(2026, 9, 30, 23, 59, 0, 0, time.Local); !s.Since.Equal(want) {
		t.Errorf("since = %v, want %v", s.Since, want)
	}
	if s, err := ReadSearchSpend(filepath.Join(t.TempDir(), "none.log"), now); err != nil || s.MonthCalls != 0 {
		t.Errorf("missing log: %+v %v", s, err)
	}
}

func TestOutlook(t *testing.T) {
	ten := time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC) // 10 of October's 31 days gone
	o := Outlook(4, 10, ten)
	if fmt.Sprintf("%.1f", o.Projected) != "12.4" || o.Over ||
		!o.RunsOut.Equal(time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("heavy pace: %+v", o)
	}
	if o := Outlook(2, 10, ten); o.Projected == 0 || !o.RunsOut.IsZero() {
		t.Errorf("light pace: %+v", o)
	}
	if o := Outlook(11, 10, ten); !o.Over || !o.RunsOut.IsZero() {
		t.Errorf("over: %+v", o)
	}
	if o := Outlook(3, 10, time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)); o.Projected != 0 {
		t.Errorf("projected after 12 hours: %+v", o)
	}
}

func TestExaMonthUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Header.Get("x-api-key") != "svc" || r.URL.Path != "/key1/usage" ||
			q.Get("start_date") != "2026-10-01T00:00:00Z" || q.Get("end_date") != "2026-10-10T09:00:00Z" {
			t.Errorf("request %s %v key %q", r.URL.Path, q, r.Header.Get("x-api-key"))
		}
		w.Write([]byte(`{"total_cost_usd": 2.345, "cost_breakdown": []}`))
	}))
	defer srv.Close()
	saved := ExaUsageURL
	ExaUsageURL = srv.URL + "/%s/usage"
	t.Cleanup(func() { ExaUsageURL = saved })

	now := time.Date(2026, 10, 10, 20, 0, 0, 0, time.FixedZone("UTC+11", 11*3600))
	got, err := ExaMonthUsage(context.Background(), "svc", "key1", now)
	if err != nil || got != 2.345 {
		t.Errorf("usage = %v, %v", got, err)
	}
	ExaUsageURL = "http://127.0.0.1:1/%s/usage"
	if _, err := ExaMonthUsage(context.Background(), "svc", "key1", now); err == nil {
		t.Error("an unreachable API gave no error")
	}
}
