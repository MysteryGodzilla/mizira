// Copyright (C) 2026 MysteryGodzilla
// Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
// SPDX-License-Identifier: GPL-3.0-only

package admin

import (
	"errors"
	"net/http"
	"strings"
)

// The Bots, Settings and Tools pages. Each change is the matching ~ command's own code.

type BotsView struct {
	Nicks      []string `json:"nicks"`
	Prefixes   []string `json:"prefixes"`
	ReplyLimit string   `json:"replyLimit"` // botreplylimit, as ~get shows it
	Cooldown   string   `json:"cooldown"`   // botcooldown
}

// SettingView is one ~set key. Credentials are never in the list.
type SettingView struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	Default    string `json:"default"` // config.yml's value
	Overridden bool   `json:"overridden"`
	Editable   bool   `json:"editable"`
}

// ToolView is one entry of the tool list: a tool of the bot's own, a plugin, or an MCP config.
type ToolView struct {
	Spec        string   `json:"spec"`
	Kind        string   `json:"kind"` // native, work (background work), plugin or mcp
	Description string   `json:"description"`
	Loaded      bool     `json:"loaded"`
	Names       []string `json:"names"` // the tools it loaded
	InConfig    bool     `json:"inConfig"`
	Switched    bool     `json:"switched"` // switched from the console, so it differs from config.yml
	AdminOnly   bool     `json:"adminOnly"`
}

// SettingChange is a setting's new value, and a warning if it was saved but not fully applied.
type SettingChange struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Warning string `json:"warning,omitempty"`
}

const maxSettingValue = 2000

func (s *Server) mzSettingsRoutes(api *http.ServeMux) {
	api.HandleFunc("GET /bots", s.bots)
	api.HandleFunc("POST /bots", s.addBot)
	api.HandleFunc("DELETE /bots", s.removeBot)
	api.HandleFunc("GET /settings", s.settings)
	api.HandleFunc("PUT /settings/{key}", s.setSetting)
	api.HandleFunc("DELETE /settings/{key}", s.resetSetting)
	api.HandleFunc("POST /settings/export", s.exportSettings)
	api.HandleFunc("POST /settings/reset-all", s.resetAllSettings)
	api.HandleFunc("GET /tools", s.tools)
	api.HandleFunc("PUT /tools", s.switchTool)
	api.HandleFunc("DELETE /tools/switches", s.resetTools)
	api.HandleFunc("PUT /tools/restrict", s.restrictTool)
}

func (s *Server) bots(w http.ResponseWriter, _ *http.Request) {
	respond(w, http.StatusOK, s.mz.Bots())
}

func validBotEntry(w http.ResponseWriter, kind, value string) bool {
	if kind != "nick" && kind != "prefix" {
		fail(w, http.StatusBadRequest, `kind is "nick" or "prefix"`)
		return false
	}
	if strings.TrimSpace(value) == "" || len(value) > maxNick {
		fail(w, http.StatusBadRequest, "give a nick or prefix")
		return false
	}
	return true
}

func (s *Server) addBot(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if !decode(w, r, &in) || !validBotEntry(w, in.Kind, in.Value) {
		return
	}
	s.changeBot(w, r, true, in.Kind, in.Value)
}

func (s *Server) removeBot(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if !validBotEntry(w, q.Get("kind"), q.Get("value")) {
		return
	}
	s.changeBot(w, r, false, q.Get("kind"), q.Get("value"))
}

func (s *Server) changeBot(w http.ResponseWriter, r *http.Request, add bool, kind, value string) {
	by := s.who(r)
	stored, err := s.mz.ChangeBots(add, kind, value, by)
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	s.log.Info("console_action", "action", "bots", "add", add, "kind", kind, "value", stored, "by", by)
	respond(w, http.StatusOK, map[string]any{"value": stored})
}

func (s *Server) settings(w http.ResponseWriter, _ *http.Request) {
	respond(w, http.StatusOK, map[string]any{"settings": s.mz.Settings(), "lists": s.mz.ListOverrides()})
}

// ExportView is where an export was written and what it changed; the file itself stays on the PC.
type ExportView struct {
	Path    string         `json:"path"`
	Changes []ExportChange `json:"changes"`
}

type ExportChange struct {
	Key  string `json:"key"`
	From string `json:"from"`
	To   string `json:"to"`
}

// ResetAllView is what Reset all put back now, and what goes back at the next restart.
type ResetAllView struct {
	Reset     []string `json:"reset"`
	OnRestart []string `json:"onRestart"`
}

// ErrNothingToExport: nothing differs from config.yml.
var ErrNothingToExport = errors.New("nothing differs from config.yml")

func (s *Server) exportSettings(w http.ResponseWriter, r *http.Request) {
	by := s.who(r)
	out, err := s.mz.ExportConfig()
	switch {
	case errors.Is(err, ErrNothingToExport):
		fail(w, http.StatusConflict, err.Error())
	case err != nil:
		fail(w, http.StatusInternalServerError, "export failed: "+err.Error())
	default:
		s.log.Info("console_action", "action", "settings_export", "to", out.Path, "changes", len(out.Changes), "by", by)
		respond(w, http.StatusOK, out)
	}
}

func (s *Server) resetAllSettings(w http.ResponseWriter, r *http.Request) {
	by := s.who(r)
	out, err := s.mz.ResetAll(by)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.log.Info("console_action", "action", "settings_reset_all", "by", by)
	respond(w, http.StatusOK, out)
}

func (s *Server) setSetting(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Value *string `json:"value"`
	}
	if !decode(w, r, &in) || in.Value == nil {
		fail(w, http.StatusBadRequest, `send {"value": "..."}`)
		return
	}
	if len(*in.Value) > maxSettingValue {
		fail(w, http.StatusBadRequest, "the value is too long")
		return
	}
	key, by := r.PathValue("key"), s.who(r)
	change, err := s.mz.SetSetting(key, *in.Value, by)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.log.Info("console_action", "action", "set", "key", key, "value", change.Value, "by", by)
	respond(w, http.StatusOK, change)
}

func (s *Server) resetSetting(w http.ResponseWriter, r *http.Request) {
	key, by := r.PathValue("key"), s.who(r)
	change, err := s.mz.ResetSetting(key, by)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.log.Info("console_action", "action", "reset_setting", "key", key, "value", change.Value, "by", by)
	respond(w, http.StatusOK, change)
}

func (s *Server) tools(w http.ResponseWriter, _ *http.Request) {
	respond(w, http.StatusOK, map[string]any{"tools": s.mz.ToolCatalog()})
}

func (s *Server) switchTool(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Spec string `json:"spec"`
		On   *bool  `json:"on"`
	}
	if !decode(w, r, &in) || in.Spec == "" || in.On == nil {
		fail(w, http.StatusBadRequest, `send {"spec": "...", "on": true|false}`)
		return
	}
	by := s.who(r)
	if err := s.mz.SwitchTool(in.Spec, *in.On, by); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	s.log.Info("console_action", "action", "tool", "tool", in.Spec, "on", *in.On, "by", by)
	respond(w, http.StatusOK, map[string]any{"spec": in.Spec, "on": *in.On})
}

func (s *Server) restrictTool(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Spec      string `json:"spec"`
		AdminOnly *bool  `json:"adminOnly"`
	}
	if !decode(w, r, &in) || in.Spec == "" || in.AdminOnly == nil {
		fail(w, http.StatusBadRequest, `send {"spec": "...", "adminOnly": true|false}`)
		return
	}
	by := s.who(r)
	if err := s.mz.RestrictTool(in.Spec, *in.AdminOnly, by); err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	s.log.Info("console_action", "action", "tool_restrict", "tool", in.Spec, "to", *in.AdminOnly, "by", by)
	respond(w, http.StatusOK, map[string]any{"spec": in.Spec, "adminOnly": *in.AdminOnly})
}

func (s *Server) resetTools(w http.ResponseWriter, r *http.Request) {
	by := s.who(r)
	err := s.mz.ResetToolSwitches(by)
	s.log.Info("console_action", "action", "tools_reset", "by", by)
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	respond(w, http.StatusOK, map[string]any{"reset": true})
}
