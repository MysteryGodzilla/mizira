// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package bot

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"B4reMetal/metald/internal/admin"
	"B4reMetal/metald/internal/core"
)

// exaDashboard is where the Exa balance and usage live; Exa has no API for them.
const exaDashboard = "https://dashboard.exa.ai"

// Services lists the model server and, when their tools are loaded, web search and pictures.
func (c console) Services(ctx context.Context) []admin.ServiceView {
	out := append([]admin.ServiceView{c.modelServer(ctx)}, c.webSearch(ctx)...)
	return append(out, c.pictures(ctx)...)
}

// lowCredit is the RunPod balance, in dollars, below which the card warns.
const lowCredit = 2.0

// pictures is image generation on RunPod: the account's credit, the endpoint's workers, and the
// pictures and GPU time from the tool log.
func (c console) pictures(ctx context.Context) []admin.ServiceView {
	if _, ok := c.sys.GetToolRegistry().Get("imagegen__picture"); !ok || os.Getenv("IMAGE_GEN_BACKEND") != "runpod" {
		return nil
	}
	key, endpoint := os.Getenv("RUNPOD_API_KEY"), os.Getenv("RUNPOD_ENDPOINT_ID")
	v := admin.ServiceView{ID: "pictures", Name: "Pictures (RunPod)",
		Link: "https://console.runpod.io/serverless/user/endpoint/" + endpoint}
	if acct, err := core.RunPodBalance(ctx, key); err != nil {
		v.Stats = append(v.Stats, core.ServiceStat{Label: "credit", Value: "RunPod didn't answer: " + err.Error()})
	} else {
		credit := fmt.Sprintf("$%.2f", acct.Balance)
		if acct.Balance < lowCredit {
			credit += " (low: top up soon)"
		}
		v.Stats = append(v.Stats, core.ServiceStat{Label: "credit", Value: credit})
		if acct.SpendPerHr > 0 {
			v.Stats = append(v.Stats, core.ServiceStat{Label: "spending now", Value: fmt.Sprintf("$%.2f/hr", acct.SpendPerHr)})
		}
	}
	if w, err := core.RunPodWorkers(ctx, key, endpoint); err == nil {
		v.Stats = append(v.Stats, core.ServiceStat{Label: "workers",
			Value: fmt.Sprintf("%d running · %d ready · %d starting", w["running"], w["idle"], w["initializing"])})
	}
	now := time.Now()
	if jobs, err := core.ReadImageJobs(os.Getenv("METALD_TOOL_LOG"), now); err == nil {
		v.Stats = append(v.Stats,
			core.ServiceStat{Label: "pictures today", Value: fmt.Sprintf("%d · %.1f GPU min", jobs.TodayJobs, jobs.TodayGPU.Minutes())},
			core.ServiceStat{Label: "pictures this month", Value: fmt.Sprintf("%d · %.1f GPU min", jobs.MonthJobs, jobs.MonthGPU.Minutes())})
	}
	v.Stats = append(v.Stats, billedStats(ctx, key, endpoint, now)...)
	if n, err := core.ImageUses(core.DataPath("usage-imagegen.json"), now); err == nil {
		used := strconv.Itoa(n)
		if total := os.Getenv("IMAGE_GEN_DAY_TOTAL"); total != "" && total != "0" {
			used += " of " + total
		}
		v.Stats = append(v.Stats, core.ServiceStat{Label: "last 24h", Value: used})
	}
	return []admin.ServiceView{v}
}

// billedStats is what RunPod billed the endpoint, by UTC day as RunPod bills: today and this month,
// with the billed minutes (workers running, idle time included). The bill covers every job on the
// endpoint, the other bot's and tests too, so no cost per picture from this bot's count.
func billedStats(ctx context.Context, key, endpoint string, now time.Time) []core.ServiceStat {
	utc := now.UTC()
	month := time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
	days, err := core.RunPodBilling(ctx, key, endpoint, month)
	if err != nil {
		return []core.ServiceStat{{Label: "billed", Value: "RunPod didn't answer: " + err.Error()}}
	}
	var today, total float64
	var todayTime, totalTime time.Duration
	for _, d := range days {
		total += d.Amount
		totalTime += d.Billed
		if d.Day.Format("2006-01-02") == utc.Format("2006-01-02") {
			today, todayTime = d.Amount, d.Billed
		}
	}
	return []core.ServiceStat{
		{Label: "billed today (UTC)", Value: fmt.Sprintf("$%.2f · %.0f min", today, todayTime.Minutes())},
		{Label: "billed this month", Value: fmt.Sprintf("$%.2f · %.0f min", total, totalTime.Minutes())},
	}
}

func (c console) modelServer(ctx context.Context) admin.ServiceView {
	base := strings.TrimSuffix(strings.TrimSuffix(c.cfg.API.OpenAIURL, "/"), "/v1")
	v := admin.ServiceView{ID: "model", Name: "Model server"}
	if base == "" {
		v.Error = "no openaiurl set"
		return v
	}
	stats, err := core.LlamaSwapStats(ctx, base, c.cfg.API.OpenAIKey)
	if err != nil {
		v.Error = "not reachable: " + err.Error()
		return v
	}
	v.Stats, v.Link = stats, base+"/ui/"
	return v
}

// exaFreeCredit is Exa's free monthly grant in dollars; EXA_MONTHLY_CREDIT overrides it.
const exaFreeCredit = 10.0

func (c console) webSearch(ctx context.Context) []admin.ServiceView {
	reg := c.sys.GetToolRegistry()
	if _, ok := reg.Get("websearch__web_search"); !ok {
		if _, ok := reg.Get("webfetch__fetch"); !ok {
			return nil
		}
	}
	v := admin.ServiceView{ID: "websearch", Name: "Web search (Exa)", Link: exaDashboard}
	now := time.Now()
	spend, err := core.ReadSearchSpend(os.Getenv("METALD_TOOL_LOG"), now)
	if err != nil {
		v.Error = "couldn't read the tool log: " + err.Error()
		return []admin.ServiceView{v}
	}
	v.Stats = []core.ServiceStat{
		{Label: "today", Value: fmt.Sprintf("%d · $%.3f", spend.TodayCalls, spend.TodayCost)},
		{Label: "this month", Value: fmt.Sprintf("%d · $%.3f", spend.MonthCalls, spend.MonthCost)},
	}
	v.Meter = exaMeter(ctx, spend, now)
	return []admin.ServiceView{v}
}

// exaMeter is the month's spend against the free credit. Exa's own figure when a service key and
// the API key's id are set; otherwise the costs her searches logged.
func exaMeter(ctx context.Context, spend core.SearchSpend, now time.Time) *admin.Meter {
	credit := exaFreeCredit
	if f, err := strconv.ParseFloat(os.Getenv("EXA_MONTHLY_CREDIT"), 64); err == nil && f > 0 {
		credit = f
	}
	start, end := core.CreditMonth(now)
	used, source := spend.MonthCost, "counted from her searches"
	if spend.Since.After(start) {
		source += " since " + spend.Since.Format("2 Jan")
	}
	if svc, id := os.Getenv("EXA_SERVICE_KEY"), os.Getenv("EXA_API_KEY_ID"); svc != "" && id != "" {
		if u, err := core.ExaMonthUsage(ctx, svc, id, now); err == nil {
			used, source = u, "from Exa"
		} else {
			source += " (Exa didn't answer: " + err.Error() + ")"
		}
	}
	o := core.Outlook(used, credit, now)
	m := &admin.Meter{
		Label: fmt.Sprintf("free credit, %s", start.Format("January")),
		Used:  used, Limit: credit, Over: o.Over,
		Note: fmt.Sprintf("$%.2f of $%.2f · %s · resets %s", used, credit, source,
			end.In(time.Local).Format("Mon 2 Jan 15:04")),
	}
	switch {
	case o.Over:
		m.Warning = fmt.Sprintf("over the free $%.2f: searches now use paid credit", credit)
	case !o.RunsOut.IsZero():
		m.Warning = fmt.Sprintf("on pace for $%.2f this month: the free credit runs out around %s",
			o.Projected, o.RunsOut.In(time.Local).Format("Mon 2 Jan"))
	case o.Projected > 0:
		m.Note += fmt.Sprintf(" · on pace for $%.2f", o.Projected)
	}
	return m
}
