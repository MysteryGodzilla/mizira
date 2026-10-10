// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package bot

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"B4reMetal/metald/internal/admin"
	"B4reMetal/metald/internal/core"
)

// exaDashboard is where the Exa balance and usage live; Exa has no API for them.
const exaDashboard = "https://dashboard.exa.ai"

// Services lists the model server and, when a search tool is loaded, web search.
func (c console) Services(ctx context.Context) []admin.ServiceView {
	return append([]admin.ServiceView{c.modelServer(ctx)}, c.webSearch()...)
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

func (c console) webSearch() []admin.ServiceView {
	reg := c.sys.GetToolRegistry()
	if _, ok := reg.Get("websearch__web_search"); !ok {
		if _, ok := reg.Get("webfetch__fetch"); !ok {
			return nil
		}
	}
	v := admin.ServiceView{ID: "websearch", Name: "Web search (Exa)", Link: exaDashboard}
	spend, err := core.ReadSearchSpend(os.Getenv("METALD_TOOL_LOG"), time.Now())
	if err != nil {
		v.Error = "couldn't read the tool log: " + err.Error()
		return []admin.ServiceView{v}
	}
	v.Stats = []core.ServiceStat{
		{Label: "today", Value: fmt.Sprintf("%d · $%.3f", spend.TodayCalls, spend.TodayCost)},
		{Label: "this month", Value: fmt.Sprintf("%d · $%.3f", spend.MonthCalls, spend.MonthCost)},
		{Label: "balance", Value: "on Exa's dashboard"},
	}
	return []admin.ServiceView{v}
}
