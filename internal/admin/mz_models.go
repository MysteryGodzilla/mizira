// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package admin

import (
	"errors"
	"net/http"
)

// The Settings page's Model card: what the model server offers, and switching between them, as
// ~models does.

type ModelView struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// ErrNoSuchModel is a switch to a model the server doesn't offer; ErrSameModel one to the current model.
var (
	ErrNoSuchModel = errors.New("the model server doesn't offer that model")
	ErrSameModel   = errors.New("she's already on that model")
)

func (s *Server) mzModelRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /models", s.models)
	api.HandleFunc("PUT /models", s.switchModel)
}

func (s *Server) models(w http.ResponseWriter, r *http.Request) {
	current, list, err := s.mz.Models(r.Context())
	if err != nil {
		fail(w, http.StatusBadGateway, "couldn't get the model list from the model server")
		return
	}
	respond(w, http.StatusOK, map[string]any{"current": current, "models": list})
}

func (s *Server) switchModel(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Model string `json:"model"`
	}
	if !decode(w, r, &in) || in.Model == "" {
		fail(w, http.StatusBadRequest, `send {"model": "<id>"}`)
		return
	}
	by := s.who(r)
	model, err := s.mz.SwitchModel(r.Context(), in.Model, by)
	switch {
	case errors.Is(err, ErrNoSuchModel), errors.Is(err, ErrSameModel):
		fail(w, http.StatusConflict, err.Error())
	case err != nil:
		fail(w, http.StatusBadGateway, err.Error())
	default:
		s.log.Info("console_action", "action", "model_switch", "to", model, "by", by)
		respond(w, http.StatusOK, map[string]any{"model": model})
	}
}
