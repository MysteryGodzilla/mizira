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
	"net/url"
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

// SearchSpend is what the web search and fetch plugins spent, from the costs they log. Today is the
// local day; the month is the credit month (CreditMonth), so it lines up with Exa's monthly reset.
type SearchSpend struct {
	TodayCalls, MonthCalls int
	TodayCost, MonthCost   float64
	// Since is the first call the log has, so a month that began before the log is shown as partial.
	Since time.Time
}

// toolLogCost is a line the Exa plugins write after a call that worked:
// "2026-10-10 09:07:11 [websearch] ok 'orange cake recipe' -> 5 results, cost=$0.007". The time is
// the machine's local time.
var toolLogCost = regexp.MustCompile(`^(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d) \[(?:websearch|webfetch)\] ok .*cost=\$([0-9.]+)\s*$`)

// ReadSearchSpend adds up the Exa calls in the tool log for today and the credit month containing
// now. A log that doesn't exist yet means nothing was spent.
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
	start, end := CreditMonth(now)
	local := now.In(time.Local)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := toolLogCost.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		at, err := time.ParseInLocation("2006-01-02 15:04:05", m[1], time.Local)
		if err != nil {
			continue
		}
		if s.Since.IsZero() {
			s.Since = at
		}
		cost, _ := strconv.ParseFloat(m[2], 64)
		if !at.Before(start) && at.Before(end) {
			s.MonthCalls++
			s.MonthCost += cost
		}
		if y, mo, d := at.Date(); y == local.Year() && mo == local.Month() && d == local.Day() {
			s.TodayCalls++
			s.TodayCost += cost
		}
	}
	return s, sc.Err()
}

// CreditMonth is the calendar month in UTC holding now: Exa resets the free credit on the first of
// each month, and its usage API speaks UTC.
func CreditMonth(now time.Time) (start, end time.Time) {
	u := now.UTC()
	start = time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, 0)
}

// ExaUsageURL is the Team Management API's per-key usage endpoint.
var ExaUsageURL = "https://admin-api.exa.ai/team-management/api-keys/%s/usage"

// ExaMonthUsage asks Exa what the API key spent this credit month. It needs a service key (Exa's
// Team Management API, enabled on request) and the API key's id.
func ExaMonthUsage(ctx context.Context, serviceKey, keyID string, now time.Time) (float64, error) {
	start, _ := CreditMonth(now)
	rctx, cancel := context.WithTimeout(ctx, serviceTimeout)
	defer cancel()
	u := fmt.Sprintf(ExaUsageURL, url.PathEscape(keyID)) + "?" + url.Values{
		"start_date": {start.Format(time.RFC3339)},
		"end_date":   {now.UTC().Format(time.RFC3339)},
	}.Encode()
	req, err := http.NewRequestWithContext(rctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("x-api-key", serviceKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("exa usage: http %d", resp.StatusCode)
	}
	var out struct {
		Total *float64 `json:"total_cost_usd"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	if out.Total == nil {
		return 0, errors.New("exa usage: no total_cost_usd")
	}
	return *out.Total, nil
}

// CreditOutlook is the month's spend against the free credit, and where the current pace ends up.
type CreditOutlook struct {
	Spent, Credit float64
	Over          bool
	// Projected is the spend at month end at this pace; zero until a day of the month has passed,
	// since a few hours say little.
	Projected float64
	// RunsOut is when the credit is used up at this pace, if that's before the month ends.
	RunsOut time.Time
}

// Outlook projects spent (so far this credit month) to the month's end.
func Outlook(spent, credit float64, now time.Time) CreditOutlook {
	o := CreditOutlook{Spent: spent, Credit: credit, Over: spent > credit}
	start, end := CreditMonth(now)
	elapsed := now.Sub(start)
	if elapsed < 24*time.Hour || spent <= 0 {
		return o
	}
	o.Projected = spent * float64(end.Sub(start)) / float64(elapsed)
	if !o.Over && o.Projected > credit {
		o.RunsOut = start.Add(time.Duration(float64(elapsed) * credit / spent))
	}
	return o
}
