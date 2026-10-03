// SPDX-License-Identifier: GPL-3.0-only

package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// SamplingKeys are the optional sampling settings. One that is not set is not sent at all, so the
// model server's own default applies; models differ, so each stays changeable with +set.
var SamplingKeys = []string{"top_p", "top_k", "min_p", "presence_penalty", "repeat_penalty"}

var samplingRange = map[string][2]float64{
	"top_p":            {0, 1},
	"top_k":            {0, 1000},
	"min_p":            {0, 1},
	"presence_penalty": {-2, 2},
	"repeat_penalty":   {0, 2},
}

// CheckSampling reports whether v is an accepted value for key.
func CheckSampling(key string, v float64) error {
	r, ok := samplingRange[key]
	if !ok {
		return fmt.Errorf("unknown sampling setting %q", key)
	}
	if v < r[0] || v > r[1] {
		return fmt.Errorf("%s must be between %g and %g", key, r[0], r[1])
	}
	if key == "top_k" && v != float64(int(v)) {
		return fmt.Errorf("top_k must be a whole number")
	}
	return nil
}

// FormatSampling lists the set values, "key=value" sorted by key, or "server defaults".
func FormatSampling(s map[string]float64) string {
	if len(s) == 0 {
		return "server defaults"
	}
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + strconv.FormatFloat(s[k], 'f', -1, 64)
	}
	return strings.Join(parts, " ")
}
