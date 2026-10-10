// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ServiceStat is one figure on a service's card in the console.
type ServiceStat struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

const serviceTimeout = 3 * time.Second

// LlamaSwapStats reads the model server's own figures: what is loaded, how fast it answers, and the
// GPU. base is the server's address without /v1. An error means it couldn't be reached at all.
func LlamaSwapStats(ctx context.Context, base, key string) ([]ServiceStat, error) {
	base = strings.TrimSuffix(strings.TrimSuffix(base, "/"), "/v1")
	get := func(path string, into any) error {
		rctx, cancel := context.WithTimeout(ctx, serviceTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(rctx, http.MethodGet, base+path, nil)
		if err != nil {
			return err
		}
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%s: http %d", path, resp.StatusCode)
		}
		return json.NewDecoder(resp.Body).Decode(into)
	}

	var running struct {
		Running []struct{ Model, State string } `json:"running"`
	}
	if err := get("/running", &running); err != nil {
		return nil, err
	}
	var stats []ServiceStat
	loaded := "none"
	if len(running.Running) > 0 {
		var parts []string
		for _, r := range running.Running {
			parts = append(parts, r.Model+" ("+r.State+")")
		}
		loaded = strings.Join(parts, ", ")
	}
	stats = append(stats, ServiceStat{"model", loaded})

	var speed struct {
		Requests int `json:"total_requests"`
		Prompt   struct {
			P50 float64 `json:"p50"`
		} `json:"prompt_histogram"`
		Gen struct {
			P50 float64 `json:"p50"`
		} `json:"gen_histogram"`
	}
	if get("/api/metrics/stats", &speed) == nil {
		if speed.Gen.P50 > 0 {
			stats = append(stats, ServiceStat{"reply speed", fmt.Sprintf("%.0f tok/s", speed.Gen.P50)})
		}
		if speed.Prompt.P50 > 0 {
			stats = append(stats, ServiceStat{"prompt speed", fmt.Sprintf("%.0f tok/s", speed.Prompt.P50)})
		}
		stats = append(stats, ServiceStat{"requests", strconv.Itoa(speed.Requests) + " since it started"})
	}

	var perf struct {
		GPU []struct {
			Name    string  `json:"name"`
			Temp    float64 `json:"temp_c"`
			Util    float64 `json:"gpu_util_pct"`
			MemUsed float64 `json:"mem_used_mb"`
			MemAll  float64 `json:"mem_total_mb"`
			Power   float64 `json:"power_draw_w"`
		} `json:"gpu_stats"`
	}
	if get("/api/performance", &perf) == nil && len(perf.GPU) > 0 {
		g := perf.GPU[len(perf.GPU)-1]
		stats = append(stats,
			ServiceStat{"VRAM", fmt.Sprintf("%.1f / %.1f GB", g.MemUsed/1024, g.MemAll/1024)},
			ServiceStat{"GPU", fmt.Sprintf("%.0f%% · %.0f°C · %.0f W", g.Util, g.Temp, g.Power)})
	}

	var version struct {
		Version string `json:"version"`
	}
	if get("/api/version", &version) == nil && version.Version != "" {
		stats = append(stats, ServiceStat{"version", version.Version})
	}
	return stats, nil
}

// SearchSpend is what the web search and fetch plugins spent, from the costs they log.
type SearchSpend struct {
	TodayCalls, MonthCalls int
	TodayCost, MonthCost   float64
}

// toolLogCost is a line the Exa plugins write after a call that worked:
// "2026-10-10 09:07:11 [websearch] ok 'orange cake recipe' -> 5 results, cost=$0.007".
var toolLogCost = regexp.MustCompile(`^(\d{4}-\d\d-\d\d) \d\d:\d\d:\d\d \[(?:websearch|webfetch)\] ok .*cost=\$([0-9.]+)\s*$`)

// ReadSearchSpend adds up the Exa calls in the tool log for today and this month (local time). A
// log that doesn't exist yet means nothing was spent.
func ReadSearchSpend(path string, now time.Time) (SearchSpend, error) {
	var s SearchSpend
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	defer f.Close()
	today, month := now.Format("2006-01-02"), now.Format("2006-01")
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := toolLogCost.FindStringSubmatch(sc.Text())
		if m == nil || !strings.HasPrefix(m[1], month) {
			continue
		}
		cost, _ := strconv.ParseFloat(m[2], 64)
		s.MonthCalls++
		s.MonthCost += cost
		if m[1] == today {
			s.TodayCalls++
			s.TodayCost += cost
		}
	}
	return s, sc.Err()
}
