// Copyright (C) 2023-2026 Alex Schlessinger and soulshack contributors
// Modified 2026 by BareMetal
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"regexp"

	"github.com/alexschlessinger/pollytool/sessions"
	"github.com/lrstanley/girc"

	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
)

// ChatContextInterface provides all context needed for handling IRC messages
type ChatContextInterface = core.ChatContextInterface

type ChatContext struct {
	context.Context
	Sys       core.System
	Session   sessions.Session
	Config    *config.Configuration
	client    *girc.Client
	event     *girc.Event
	args      []string
	isCommand bool // the message is a command for this bot (see CommandWords)
	logger    *slog.Logger
	requestID string
	fatalCh   chan<- error
}

var _ ChatContextInterface = (*ChatContext)(nil)

func NewChatContext(parentctx context.Context, config *config.Configuration, system core.System, ircclient *girc.Client, e *girc.Event, fatalCh chan<- error) (ChatContextInterface, context.CancelFunc) {
	timedctx, cancel := context.WithTimeout(parentctx, config.API.Timeout)

	// Generate a unique request ID for correlation
	requestID := generateRequestID()

	// Ensure Source is not nil for events like CONNECTED
	if e.Source == nil {
		e.Source = &girc.Source{
			Name: config.Server.Channel,
		}
	}

	// Get channel safely
	channel := config.Server.Channel
	if len(e.Params) > 0 {
		channel = e.Params[0]
	}

	// A command addressed by name ("Mizira +memories") loses the name here, so commands see their
	// own name first, as before.
	args, isCommand := CommandWords(config, ircclient.GetNick(), e.Last())

	ctx := ChatContext{
		Context:   timedctx,
		Config:    config,
		Sys:       system,
		client:    ircclient,
		event:     e,
		args:      args,
		isCommand: isCommand,
		requestID: requestID,
		fatalCh:   fatalCh,
		logger: slog.Default().With(
			"request_id", requestID,
			"channel", channel,
			"source", e.Source.Name,
		),
	}

	key := channel
	if !girc.IsValidChannel(key) {
		key = e.Source.Name
	}
	key = scopeKey(config.Server.Name, key)

	session, err := ctx.Sys.GetSessionStore().Get(key)
	if err != nil {
		slog.Error("failed to get session for key", "key", key, "error", err)
		os.Exit(1)
	}
	ctx.Session = session
	return &ctx, cancel
}

func (c ChatContext) GetSystem() core.System {
	return c.Sys
}

func (c ChatContext) GetConfig() *config.Configuration {
	return c.Config
}

func (c ChatContext) GetLogger() *slog.Logger {
	return c.logger
}

func (c ChatContext) Oper(channel, nick string) bool {
	if !c.mayTarget(channel, "oper") {
		return false
	}
	c.client.Cmd.Oper(channel, nick)
	return true
}

func (c ChatContext) Kick(channel, nick, reason string) bool {
	if !c.mayTarget(channel, "kick") {
		return false
	}
	c.client.Cmd.Kick(channel, nick, reason)
	return true
}

func (c ChatContext) Topic(channel, topic string) bool {
	if !c.mayTarget(channel, "topic") {
		return false
	}
	c.client.Cmd.Topic(channel, topic)
	return true
}

// IsAddressed returns true if the message activates the bot.
func (s ChatContext) IsAddressed() bool {
	trigger := s.Config.Bot.Trigger
	if trigger == "" {
		trigger = s.client.GetNick()
	}
	return CheckAddressed(s.event.Last(), trigger)
}

func (c ChatContext) Nick(nickname string) bool {
	c.client.Cmd.Nick(nickname)
	return true
}

func (c ChatContext) Join(channel string) bool {
	if !c.mayTarget(channel, "join") {
		return false
	}
	c.client.Cmd.Join(channel)
	return true
}

func (c ChatContext) JoinWithKey(channel, key string) bool {
	if !c.mayTarget(channel, "join") {
		return false
	}
	c.client.Cmd.Join(channel, key)
	return true
}

func (c ChatContext) FatalError(err error) {
	select {
	case c.fatalCh <- err:
	default:
	}
	c.client.Close()
}

func (c ChatContext) GetArgs() []string {
	return c.args
}

func (c ChatContext) GetSession() sessions.Session {
	return c.Session
}

// GetNetwork names the network this request came from, for scoping data that must not cross
// networks.
func (c ChatContext) GetNetwork() string {
	if c.Config == nil || c.Config.Server == nil {
		return ""
	}
	return c.Config.Server.Name
}

func (c ChatContext) GetBotNick() string {
	return c.client.GetNick()
}

// GetRequestID is the bot's own id for this request, used so a command can
// exclude itself when cancelling in-flight work.
func (c ChatContext) GetRequestID() string { return c.requestID }

func (c ChatContext) GetSource() string {
	return c.event.Source.Name
}

func (c ChatContext) IsAdmin() bool {
	hostmask := c.event.Source.String()
	c.logger.Debug("admin_check", "hostmask", hostmask)
	isAdmin := CheckAdmin(hostmask, c.Config.Bot.Admins)
	if isAdmin {
		c.logger.Debug("admin_verified", "hostmask", hostmask)
	}
	return isAdmin
}

// leakedToolCall matches the shapes a model uses when it writes a tool call out as prose instead of
// emitting a structured one.
var leakedToolCall = regexp.MustCompile(
	`(?i)(</?tool_call|</?function_call|</?function\b|</?parameter\b|</?invoke\b|"tool_calls"\s*:)`)

// LooksLikeToolCall reports whether text contains tool-call wire format.
func LooksLikeToolCall(s string) bool { return leakedToolCall.MatchString(s) }

func (c ChatContext) Reply(message string) {
	// Last line of defence against replying to someone who was ignored
	if reason, quiet := c.silenced(); quiet {
		c.logger.Debug("reply_suppressed", "reason", reason, "message", message)
		return
	}

	if !c.mayReplyHere("reply") {
		return
	}

	// Never let the model's own tool-call wire format reach the channel.
	if leakedToolCall.MatchString(message) {
		c.logger.Warn("reply_suppressed_tool_syntax", "message", message)
		return
	}

	// Same idea for a bare reasoning marker.
	if bareThinkMarker.MatchString(message) {
		c.logger.Warn("reply_suppressed_think_marker", "message", message)
		return
	}

	c.logger.Info("reply_sent", "message", message)

	c.client.Cmd.Reply(*c.event, c.withPrefix(RenderIRCFormatting(message)))
}

// withPrefix marks a line as the bot's. On a shared account an unmarked line, an action above
// all ("* alice slaps bob"), reads as the account owner's own.
func (c ChatContext) withPrefix(message string) string {
	if prefix := c.Config.EffectiveResponsePrefix(); prefix != "" {
		return prefix + " " + message
	}
	return message
}

// mayReplyHere is the A13 send-side check for replies: the place the event happened must be
// allowed. The entry gate (ChannelGate) should already have dropped anything else; this
// catches a code path that skips it.
func (c ChatContext) mayReplyHere(action string) bool {
	target, gated := eventTarget(c.event)
	if !gated {
		return true
	}
	return c.mayTarget(target, action)
}

// mayTarget is the last-line A13 check on everything the bot sends or does to a channel:
// any channel but the configured one is refused and logged.
func (c ChatContext) mayTarget(target, action string) bool {
	if ChannelAllowed(c.Config, target) {
		return true
	}
	c.logger.Warn("send_blocked_channel", "target", target, "action", action)
	return false
}

// silenced reports whether this request may write to the channel at all, and why not.
func (c ChatContext) silenced() (string, bool) {
	if c.suppressed() {
		return "source_ignored", true
	}
	if errors.Is(c.Err(), context.Canceled) {
		return "request_cancelled", true
	}
	return "", false
}

// suppressed reports whether output for this request should be dropped.
func (c ChatContext) suppressed() bool {
	if c.event == nil || c.event.Source == nil {
		return false
	}
	if !core.Ignores().IsIgnored(c.GetNetwork(), c.event.Source.Name) {
		return false
	}
	return !c.IsAdmin()
}

func (c ChatContext) SendAction(target, message string) {
	if !c.mayTarget(target, "action") {
		return
	}
	c.client.Cmd.Action(target, c.withPrefix(message))
}

func (c ChatContext) ReplyAction(message string) {
	// Same two gates as Reply.
	if reason, quiet := c.silenced(); quiet {
		c.logger.Debug("action_suppressed", "reason", reason, "message", message)
		return
	}
	if !c.mayReplyHere("action") {
		return
	}

	target := c.event.Params[0]
	if !girc.IsValidChannel(target) {
		// For PMs, send a regular message instead of an action
		c.client.Cmd.Message(c.event.Source.Name, c.withPrefix(message))
		return
	}
	c.client.Cmd.Action(target, c.withPrefix(message))
}

func (c ChatContext) SetMode(target, flags string, args ...string) bool {
	if !c.mayTarget(target, "mode") {
		return false
	}
	c.client.Cmd.Mode(target, flags, args...)
	return true
}

func (c ChatContext) Ban(channel, target string) bool {
	if !c.mayTarget(channel, "ban") {
		return false
	}
	c.client.Cmd.Ban(channel, target)
	return true
}

func (c ChatContext) Unban(channel, target string) bool {
	if !c.mayTarget(channel, "unban") {
		return false
	}
	c.client.Cmd.Unban(channel, target)
	return true
}

func (c ChatContext) Invite(channel, nick string) bool {
	if !c.mayTarget(channel, "invite") {
		return false
	}
	c.client.Cmd.Invite(channel, nick)
	return true
}

func (c ChatContext) GetUser(nick string) *core.UserInfo {
	user := c.client.LookupUser(nick)
	if user == nil {
		return nil
	}
	return &core.UserInfo{
		Nick:     user.Nick,
		Ident:    user.Ident,
		Host:     user.Host,
		RealName: user.Extras.Name,
		Account:  user.Extras.Account,
		Away:     user.Extras.Away,
		Channels: user.ChannelList,
	}
}

func (c ChatContext) GetChannel(name string) *core.ChannelInfo {
	ch := c.client.LookupChannel(name)
	if ch == nil {
		return nil
	}
	return &core.ChannelInfo{
		Name:  ch.Name,
		Modes: ch.Modes.String(),
		Topic: ch.Topic,
	}
}

func (c ChatContext) GetChannelUsers(channel string) []core.ChannelUser {
	ch := c.client.LookupChannel(channel)
	if ch == nil {
		return nil
	}

	client := c.client
	users := ch.Users(client)
	admins := ch.Admins(client)
	trusted := ch.Trusted(client)

	adminMap := make(map[string]bool)
	for _, admin := range admins {
		adminMap[admin.Nick] = true
	}
	trustedMap := make(map[string]bool)
	for _, tu := range trusted {
		trustedMap[tu.Nick] = true
	}

	var result []core.ChannelUser
	for _, user := range users {
		result = append(result, core.ChannelUser{
			Nick:    user.Nick,
			IsOp:    adminMap[user.Nick],
			IsVoice: trustedMap[user.Nick],
		})
	}
	return result
}

// GetLockKey identifies the conversation this request belongs to, for serializing requests and for
// "+reset".
func (c ChatContext) GetLockKey() string {
	return scopeKey(c.Config.Server.Name, c.unscopedLockKey())
}

func (c ChatContext) unscopedLockKey() string {
	if len(c.event.Params) > 0 && girc.IsValidChannel(c.event.Params[0]) {
		return c.Config.Server.Channel
	}
	if c.event.Source != nil {
		return c.event.Source.Name
	}
	return c.Config.Server.Channel
}

// scopeKey prefixes a key with its network.
func scopeKey(network, key string) string {
	if network == "" {
		return key
	}
	return network + "/" + key
}

func (c ChatContext) IsOp(channel, nick string) bool {
	user := c.client.LookupUser(nick)
	if user == nil {
		return false
	}
	perms, ok := user.Perms.Lookup(channel)
	return ok && perms.IsAdmin()
}

func (c ChatContext) IsPrivate() bool {
	if len(c.event.Params) == 0 {
		return false
	}
	return CheckPrivate(c.event.Params[0])
}

func (c ChatContext) IsBotLine() bool {
	return ClassifyLine(c.Config, c.event.Source.Name, c.event.Last()) == BotLine
}

// GetCommand returns the command this message runs, or "" for ordinary chat. A "+word" that
// isn't a command for this bot (e.g. no name when commandsneedname is on) is chat, so a line
// that merely mentions the bot can't sneak a command past the name rule.
func (c ChatContext) GetCommand() string {
	if !c.isCommand || len(c.args) == 0 {
		return ""
	}
	return CanonicalCommand(c.args[0], c.Config.Bot.CommandPrefix)
}

// generateRequestID creates a unique 8-character request ID for correlation
func generateRequestID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}
