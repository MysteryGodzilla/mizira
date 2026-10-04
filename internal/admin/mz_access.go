// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package admin

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// CheckListen refuses an address that would serve the console on every interface. The console is
// meant to sit behind a reverse proxy on the same machine (or a specific private address), so
// "0.0.0.0", "::" and a bare ":port" are mistakes, not choices.
func CheckListen(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("adminlisten %q: %w", addr, err)
	}
	if host == "" {
		return fmt.Errorf("adminlisten %q listens on every interface; name an address such as 127.0.0.1", addr)
	}
	if ip, err := netip.ParseAddr(host); err == nil && ip.IsUnspecified() {
		return fmt.Errorf("adminlisten %q listens on every interface; use 127.0.0.1 behind the proxy", addr)
	}
	return nil
}

// Failed sign-ins: after authFailures wrong tokens from one client within authWindow, that client is
// refused outright (even with the right token) for authLockout.
const (
	authFailures = 10
	authWindow   = 10 * time.Minute
	authLockout  = 5 * time.Minute
)

type authAttempts struct {
	mu      sync.Mutex
	clients map[string]*attempts
}

type attempts struct {
	failed      []time.Time
	lockedUntil time.Time
}

// clientOf is who is signing in. Behind the reverse proxy every request comes from loopback, so the
// proxy's X-Forwarded-For (its last entry, the one the proxy itself added) names the client. A
// loopback request with no forwarding header comes from this machine, where config.yml and its token
// already are, so it isn't throttled ("").
func clientOf(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err != nil || !addr.Unmap().IsLoopback() {
		return host
	}
	fwd := r.Header.Values("X-Forwarded-For")
	if len(fwd) == 0 {
		return ""
	}
	hops := strings.Split(fwd[len(fwd)-1], ",")
	if last := strings.TrimSpace(hops[len(hops)-1]); last != "" {
		return last
	}
	return ""
}

// throttle sits in front of authenticate. Only a wrong bearer token counts as a failure: a request
// with none is the page asking whether the auth proxy signed it in.
func (s *Server) throttle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client := clientOf(r)
		if client == "" {
			next.ServeHTTP(w, r)
			return
		}
		now := time.Now()
		if until := s.auth.locked(client, now); !until.IsZero() {
			fail(w, http.StatusTooManyRequests, fmt.Sprintf("too many wrong tokens; try again in %d minute(s)",
				int(until.Sub(now).Minutes())+1))
			return
		}
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if ok && subtle.ConstantTimeCompare([]byte(got), []byte(s.cfg.Token)) != 1 {
			if s.auth.fail(client, now) {
				s.log.Warn("admin_auth_locked", "client", client, "failures", authFailures,
					"minutes", int(authLockout.Minutes()))
			}
		} else if ok {
			s.auth.succeed(client)
		}
		next.ServeHTTP(w, r)
	})
}

func (a *authAttempts) locked(client string, now time.Time) time.Time {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c := a.clients[client]; c != nil && now.Before(c.lockedUntil) {
		return c.lockedUntil
	}
	return time.Time{}
}

// fail records a wrong token and reports whether it just locked the client out.
func (a *authAttempts) fail(client string, now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.clients == nil {
		a.clients = map[string]*attempts{}
	}
	if len(a.clients) > 1000 { // forget quiet clients rather than grow without bound
		for k, c := range a.clients {
			if now.After(c.lockedUntil) && (len(c.failed) == 0 || now.Sub(c.failed[len(c.failed)-1]) > authWindow) {
				delete(a.clients, k)
			}
		}
	}
	c := a.clients[client]
	if c == nil {
		c = &attempts{}
		a.clients[client] = c
	}
	kept := c.failed[:0]
	for _, t := range c.failed {
		if now.Sub(t) < authWindow {
			kept = append(kept, t)
		}
	}
	c.failed = append(kept, now)
	if len(c.failed) >= authFailures {
		c.failed, c.lockedUntil = nil, now.Add(authLockout)
		return true
	}
	return false
}

func (a *authAttempts) succeed(client string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.clients, client)
}

// who made a change through the console, for the audit log.
func (s *Server) who(r *http.Request) string {
	if user := s.proxyUser(r); user != "" {
		return "console:" + user
	}
	return "console:token"
}
