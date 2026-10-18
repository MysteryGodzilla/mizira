// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"
)

// RunPod endpoints, variables so tests can point them at a local server.
var (
	runpodGraphQL = "https://api.runpod.io/graphql"
	runpodAPI     = "https://api.runpod.ai/v2"
)

// RunPodAccount is the account's credit and spending, from RunPod's own API.
type RunPodAccount struct {
	Balance    float64 // dollars left
	SpendPerHr float64 // what running workers cost right now
	SpendLimit float64
}

func runpodDo(ctx context.Context, method, url, key string, body []byte, into any) error {
	rctx, cancel := context.WithTimeout(ctx, serviceTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, method, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	// RunPod sits behind Cloudflare, which may refuse a library's default user agent.
	req.Header.Set("User-Agent", "metald-console/1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

// RunPodBalance reads the account's credit.
func RunPodBalance(ctx context.Context, key string) (RunPodAccount, error) {
	var out struct {
		Data struct {
			Myself *struct {
				ClientBalance     float64 `json:"clientBalance"`
				CurrentSpendPerHr float64 `json:"currentSpendPerHr"`
				SpendLimit        float64 `json:"spendLimit"`
			} `json:"myself"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	body := []byte(`{"query":"query { myself { clientBalance currentSpendPerHr spendLimit } }"}`)
	if err := runpodDo(ctx, http.MethodPost, runpodGraphQL, key, body, &out); err != nil {
		return RunPodAccount{}, err
	}
	if len(out.Errors) > 0 {
		return RunPodAccount{}, errors.New(out.Errors[0].Message)
	}
	if out.Data.Myself == nil {
		return RunPodAccount{}, errors.New("no account in the answer")
	}
	m := out.Data.Myself
	return RunPodAccount{Balance: m.ClientBalance, SpendPerHr: m.CurrentSpendPerHr, SpendLimit: m.SpendLimit}, nil
}

// RunPodWorkers reads an endpoint's worker counts by state (idle, running, initializing, ...).
func RunPodWorkers(ctx context.Context, key, endpoint string) (map[string]int, error) {
	var out struct {
		Workers map[string]int `json:"workers"`
	}
	if err := runpodDo(ctx, http.MethodGet, runpodAPI+"/"+endpoint+"/health", key, nil, &out); err != nil {
		return nil, err
	}
	return out.Workers, nil
}

// ImageJobs counts the pictures made and the GPU time they billed, from the image tool's log lines.
// Today is the local day; the month is the local calendar month.
type ImageJobs struct {
	TodayJobs, MonthJobs int
	TodayGPU, MonthGPU   time.Duration
}

// imageJobLine is what the image tool writes after a RunPod job:
// "2026-10-18 14:32:25 [image_gen] job 82e9... done: delay_ms=1061 execution_ms=42970 gpu_seconds=43.0".
var imageJobLine = regexp.MustCompile(`^(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d) \[image_gen\] job \S+ done: .*gpu_seconds=([0-9.]+)`)

// ReadImageJobs counts the image jobs in the tool log at path.
func ReadImageJobs(path string, now time.Time) (ImageJobs, error) {
	var j ImageJobs
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return j, nil
	}
	if err != nil {
		return j, err
	}
	defer f.Close()
	local := now.In(time.Local)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := imageJobLine.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		at, err := time.ParseInLocation("2006-01-02 15:04:05", m[1], time.Local)
		if err != nil {
			continue
		}
		secs, _ := strconv.ParseFloat(m[2], 64)
		gpu := time.Duration(secs * float64(time.Second))
		if at.Year() == local.Year() && at.Month() == local.Month() {
			j.MonthJobs++
			j.MonthGPU += gpu
			if at.Day() == local.Day() {
				j.TodayJobs++
				j.TodayGPU += gpu
			}
		}
	}
	return j, sc.Err()
}

// ImageUses counts the pictures the image tool's caps have counted in the last 24 hours, from its
// usage file (usage-imagegen.json under the data folder).
func ImageUses(path string, now time.Time) (int, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var state struct {
		Uses [][2]json.RawMessage `json:"uses"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return 0, err
	}
	n := 0
	for _, u := range state.Uses {
		var at float64
		if json.Unmarshal(u[0], &at) == nil && now.Sub(time.Unix(int64(at), 0)) < 24*time.Hour {
			n++
		}
	}
	return n, nil
}
