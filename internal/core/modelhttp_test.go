// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// A connection dropped mid-request (the live EOF) is retried once, with the same body.
func TestModelPostRetriesDroppedConnection(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if calls.Add(1) == 1 {
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close() // drop it without an answer
			return
		}
		w.Write(body)
	}))
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader([]byte("verdict please")))
	resp, err := ModelPost(req, 5*time.Second)
	if err != nil {
		t.Fatalf("want the retry to succeed, got %v", err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if string(got) != "verdict please" || calls.Load() != 2 {
		t.Fatalf("got %q after %d calls", got, calls.Load())
	}
}
