// SPDX-License-Identifier: GPL-3.0-only

package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/urfave/cli/v3"
)

func TestCheckSampling(t *testing.T) {
	ok := map[string]float64{"top_p": 0.95, "top_k": 64, "min_p": 0, "presence_penalty": -1.5, "repeat_penalty": 1.1,
		"dry_multiplier": 0.8, "dry_base": 1.75, "dry_allowed_length": 2, "dry_penalty_last_n": -1}
	for k, v := range ok {
		if err := CheckSampling(k, v); err != nil {
			t.Errorf("CheckSampling(%s, %g) = %v", k, v, err)
		}
	}
	bad := map[string]float64{"top_p": 1.5, "top_k": 2.5, "min_p": -0.1, "repeat_penalty": 3, "typical_p": 0.5,
		"dry_allowed_length": 2.5, "dry_base": 0.5}
	for k, v := range bad {
		if CheckSampling(k, v) == nil {
			t.Errorf("CheckSampling(%s, %g) accepted", k, v)
		}
	}
}

func TestFormatSampling(t *testing.T) {
	if got := FormatSampling(nil); got != "server defaults" {
		t.Errorf("got %q", got)
	}
	if got := FormatSampling(map[string]float64{"top_p": 0.9, "min_p": 0.05}); got != "min_p=0.05 top_p=0.9" {
		t.Errorf("got %q", got)
	}
}

// A key left out of config.yml is not set, so it is never sent; one written there is.
func TestReadSamplingFromYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("top_p: 0.9\ntop_k: 64\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("METALD_CONFIG", path)

	var got map[string]float64
	cmd := &cli.Command{Flags: GetFlags(), Action: func(_ context.Context, c *cli.Command) error {
		got = readSampling(c)
		return nil
	}}
	if err := cmd.Run(context.Background(), []string{"mizira"}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["top_p"] != 0.9 || got["top_k"] != 64 {
		t.Fatalf("readSampling = %v", got)
	}
}
