// SPDX-License-Identifier: GPL-3.0-only

package core

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// modelTransport never keeps a connection to the model server alive: llama-server drops idle ones,
// and a request sent down a dropped one fails with EOF, which a fail-closed check turns into a
// refusal. Connections are local, so opening one per request costs nothing.
var modelTransport = &http.Transport{DisableKeepAlives: true}

// ModelPost sends one request to the model server, retrying once if the connection was dropped
// before an answer came back. Every request it sends is safe to repeat.
func ModelPost(req *http.Request, timeout time.Duration) (*http.Response, error) {
	client := &http.Client{Timeout: timeout, Transport: modelTransport}
	resp, err := client.Do(req)
	if err == nil || !droppedConnection(err) || req.GetBody == nil {
		return resp, err
	}
	body, berr := req.GetBody()
	if berr != nil {
		return nil, err
	}
	retry := req.Clone(req.Context())
	retry.Body = body
	return client.Do(retry)
}

func droppedConnection(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset") || strings.Contains(msg, "forcibly closed")
}
