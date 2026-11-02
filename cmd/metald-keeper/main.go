// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

// metald-keeper runs the bot and its model server and lets a dashboard on the same machine start,
// stop and restart them. See examples/keeper.yml. On Windows build it with -ldflags -H=windowsgui so
// it opens no window when started at logon; it logs to the file its config names.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"B4reMetal/metald/internal/keeper"
)

func main() {
	path := flag.String("config", "keeper.yml", "the keeper's config file")
	flag.Parse()
	if err := run(*path); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(path string) error {
	cfg, err := keeper.LoadConfig(path)
	if err != nil {
		return err
	}
	var out io.Writer = os.Stderr
	if cfg.Log != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.Log), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(cfg.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	log := slog.New(slog.NewJSONHandler(out, nil))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	k := keeper.New(cfg, log)
	srv := &http.Server{Addr: cfg.Listen, Handler: k.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shut, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_ = srv.Shutdown(shut)
	}()
	// Listen before starting anything: a second keeper fails here instead of starting a second bot.
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	log.Info("keeper_listening", "addr", cfg.Listen, "startatlaunch", cfg.StartAtLaunch)
	if cfg.StartAtLaunch {
		if err := k.Do("start"); err != nil {
			log.Warn("keeper_start_at_launch_skipped", "reason", err.Error())
		}
	}
	err = srv.Serve(ln)
	// Stopping the keeper stops what it started, so nothing is left holding the GPU or the ports.
	k.Wait(context.Background())
	k.Shutdown()
	log.Info("keeper_stopped")
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
