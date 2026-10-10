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

// Services lists the model server and, when a search tool is loaded, web search.
func (c console) Services(ctx context.Context) []admin.ServiceView {
	return append([]admin.ServiceView{c.modelServer(ctx)}, c.webSearch(ctx)...)
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
