// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package keeper

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Wrong keys: after lockFailures from one address within lockWindow, it is refused for lockFor.
const (
	lockFailures = 10
	lockWindow   = 10 * time.Minute
	lockFor      = 5 * time.Minute
)

type lockout struct {
	mu     sync.Mutex
	failed map[string][]time.Time
	until  map[string]time.Time
}

func (l *lockout) locked(who string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return now.Before(l.until[who])
}

func (l *lockout) fail(who string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failed == nil {
		l.failed, l.until = map[string][]time.Time{}, map[string]time.Time{}
	}
	kept := l.failed[who][:0]
	for _, t := range l.failed[who] {
		if now.Sub(t) < lockWindow {
			kept = append(kept, t)
		}
	}
	l.failed[who] = append(kept, now)
	if len(l.failed[who]) >= lockFailures {
		delete(l.failed, who)
		l.until[who] = now.Add(lockFor)
	}
}

// Handler is the keeper's API: GET /status with the bot's statustoken or the control token, and
// POST /start, /stop, /restart and /coldstart with the control token only.
func (k *Keeper) Handler() http.Handler {
	var locks lockout
	mux := http.NewServeMux()
	guard := func(control bool, next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			who, _, _ := net.SplitHostPort(r.RemoteAddr)
			now := time.Now()
			if locks.locked(who, now) {
				reply(w, http.StatusTooManyRequests, map[string]string{"error": "too many wrong keys; try again later"})
				return
			}
			got, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			ok := matches(got, k.cfg.ControlToken)
			if !ok && !control {
				s, err := readBotSettings(k.cfg.Bot.Config)
				ok = err == nil && matches(got, s.StatusToken)
			}
			if !ok {
				if got != "" {
					locks.fail(who, now)
				}
				k.log.Warn("keeper_unauthorized", "path", r.URL.Path, "remote", r.RemoteAddr)
				reply(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("GET /status", guard(false, func(w http.ResponseWriter, _ *http.Request) {
		reply(w, http.StatusOK, k.Status())
	}))
	for _, action := range Actions {
		mux.HandleFunc("POST /"+action, guard(true, func(w http.ResponseWriter, r *http.Request) {
			k.log.Info("keeper_request", "action", action, "remote", r.RemoteAddr)
			switch err := k.Do(action); {
			case errors.Is(err, ErrBusy), errors.Is(err, ErrExternal):
				reply(w, http.StatusConflict, map[string]any{"error": err.Error(), "status": k.Status()})
			case err != nil:
				reply(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			default:
				reply(w, http.StatusAccepted, map[string]any{"accepted": action})
			}
		}))
	}
	return mux
}

func matches(got, want string) bool {
	return want != "" && got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
