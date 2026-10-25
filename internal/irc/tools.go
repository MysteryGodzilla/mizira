// Copyright (C) 2023-2026 Alex Schlessinger and soulshack contributors
// Modified 2026 by BareMetal
// SPDX-License-Identifier: GPL-3.0-only

package irc

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/alexschlessinger/pollytool/schema"
	"github.com/alexschlessinger/pollytool/tools"
)

type contextKey string

const kContextKey contextKey = "irc_context"

func GetIRCContext(ctx context.Context) (ChatContextInterface, error) {
	if chatCtx, ok := ctx.Value(kContextKey).(ChatContextInterface); ok {
		return chatCtx, nil
	}
	return nil, fmt.Errorf("no IRC context available")
}

// InjectContext stores the IRC context for tools to retrieve.
// This must be used (rather than direct context.WithValue) to ensure
// the correct key type is used.
func InjectContext(ctx context.Context, chatCtx ChatContextInterface) context.Context {
	return context.WithValue(ctx, kContextKey, chatCtx)
}

func isBotOpped(ctx ChatContextInterface) bool {
	channel := ctx.GetConfig().Server.Channel
	botNick := ctx.GetBotNick()

	users := ctx.GetChannelUsers(channel)
	if users == nil {
		return false
	}

	for _, user := range users {
		if user.Nick == botNick {
			return user.IsOp
		}
	}
	return false
}

// validateAdminOp validates admin permissions and bot op status.
// Returns (ctx, "", nil) on success, (nil, denial-msg, nil) on policy denial,
// or (nil, "", err) on context/lookup error.
func validateAdminOp(ctx context.Context) (ChatContextInterface, string, error) {
	chatCtx, err := GetIRCContext(ctx)
	if err != nil {
		return nil, "", err
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if !chatCtx.IsAdmin() {
		return nil, "You are not authorized to use this tool", nil
	}
	if !isBotOpped(chatCtx) {
		return nil, "Bot does not have operator status in the channel", nil
	}
	return chatCtx, "", nil
}

func validateContext(ctx context.Context) (ChatContextInterface, error) {
	chatCtx, err := GetIRCContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return chatCtx, nil
}

// AdminOnlyTool wraps any tool (shell, native, or MCP) so it only executes
// for admins. Non-admins get a clear denial message instead of it running.
type AdminOnlyTool struct {
	tools.Tool
}

// NewAdminOnlyTool wraps tool so only recognized admins can execute it.
func NewAdminOnlyTool(tool tools.Tool) *AdminOnlyTool {
	return &AdminOnlyTool{Tool: tool}
}

// IsAdminOnly reports whether tool is currently restricted to admins.
func IsAdminOnly(tool tools.Tool) bool {
	_, ok := tool.(*AdminOnlyTool)
	return ok
}

// UnwrapAdminOnly returns the underlying unrestricted tool, or tool itself if it was never wrapped.
func UnwrapAdminOnly(tool tools.Tool) tools.Tool {
	if a, ok := tool.(*AdminOnlyTool); ok {
		return a.Tool
	}
	return tool
}

// GetSchema annotates the description with an [admin only] suffix so the
// model (and /tools listings) can see the restriction up front.
func (a *AdminOnlyTool) GetSchema() *schema.ToolSchema {
	c := a.Tool.GetSchema().Copy()
	if c == nil {
		return nil
	}
	c.Raw["description"] = c.Description() + " [admin only]"
	return c
}

func (a *AdminOnlyTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	chatCtx, err := GetIRCContext(ctx)
	if err != nil {
		return "", err
	}
	if !chatCtx.IsAdmin() {
		// Spelled out: a bare "not authorized" was read in chat as a usage limit that had run out.
		return fmt.Sprintf("Not allowed: this tool is for admins only right now and %s isn't one. "+
			"It isn't a usage limit and nothing is broken. Say so plainly; don't retry it for them.", chatCtx.GetSource()), nil
	}
	return a.Tool.Execute(ctx, args)
}

// PeopleOnlyTool wraps a tool so it refuses calls made for another bot's line: a person could
// otherwise ask a bot to ask, and get round the tool's per-person limits.
type PeopleOnlyTool struct {
	tools.Tool
}

// NewPeopleOnlyTool wraps tool so it refuses calls made for another bot's line.
func NewPeopleOnlyTool(tool tools.Tool) *PeopleOnlyTool {
	return &PeopleOnlyTool{Tool: tool}
}

func (p *PeopleOnlyTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	chatCtx, err := GetIRCContext(ctx)
	if err != nil {
		return "", err
	}
	if chatCtx.IsBotLine() {
		return fmt.Sprintf("Not allowed: this tool is only used for people, not for other bots (%s). "+
			"Say so plainly; don't retry it for them.", chatCtx.Speaker()), nil
	}
	return p.Tool.Execute(ctx, args)
}

// OncePerRequestTool wraps a tool so one request runs it once: the model asked for a picture, then
// drew a second one in the same turn and posted both.
type OncePerRequestTool struct {
	tools.Tool
	mu   sync.Mutex
	used map[string]onceCall // by request id
}

type onceCall struct {
	at        time.Time
	result    string // the first call's result, first line
	retryable bool   // that result was a fixable refusal and no retry has run yet
	retried   bool
}

// OnceRepeated starts the answer to a second call in the same request.
const OnceRepeated = "Already done:"

// OnceMerged starts the answer to a call folded into another call of the same turn, and MergedArg is
// the argument that marks it (llm's mergePictureCalls).
const (
	OnceMerged = "Merged:"
	MergedArg  = "_merged"
)

// onceFixable start a refusal that drew nothing and says how to call the tool right (an edit link on a
// new picture, a style of the wrong kind). The model may correct it once: without that, it told the
// channel the picture was done.
var onceFixable = []string{"Not done:", "Not edited:"}

// NewOncePerRequestTool wraps tool so each request may run it once.
func NewOncePerRequestTool(tool tools.Tool) *OncePerRequestTool {
	return &OncePerRequestTool{Tool: tool, used: map[string]onceCall{}}
}

func (o *OncePerRequestTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	chatCtx, err := GetIRCContext(ctx)
	if err != nil {
		return "", err
	}
	if merged, _ := args[MergedArg].(bool); merged {
		// Not a call of its own, so it mustn't take the request's one run from the call it went into.
		return OnceMerged + " this one is drawn in the same picture as the first call of this turn, which shows " +
			"them all. Reply once, with that picture's link.", nil
	}
	request := chatCtx.GetRequestID()
	if earlier, again := o.first(request, time.Now()); again {
		// After a refusal it kept retrying "with less physical implication": say what the first call
		// got, and that this turn is over for this tool.
		return fmt.Sprintf("%s this tool runs once per request, and this request already had its answer: %q. "+
			"Don't call it again or retry it differently; reply now, in a line or two, with that answer. "+
			"They can ask again if they want another try.", OnceRepeated, earlier), nil
	}
	result, err := o.Tool.Execute(ctx, args)
	o.record(request, result)
	return result, err
}

// first records the request's call; again is true, with the first call's result, when it already had
// one. Old requests are dropped.
func (o *OncePerRequestTool) first(request string, now time.Time) (earlier string, again bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for id, c := range o.used {
		if now.Sub(c.at) > time.Hour {
			delete(o.used, id)
		}
	}
	if c, seen := o.used[request]; seen {
		if c.retryable {
			c.retryable, c.retried = false, true
			o.used[request] = c
			return "", false
		}
		return c.result, true
	}
	o.used[request] = onceCall{at: now}
	return "", false
}

func (o *OncePerRequestTool) record(request, result string) {
	first, _, _ := strings.Cut(result, "\n")
	if r := []rune(first); len(r) > 200 {
		first = string(r[:200]) + "…"
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if c, ok := o.used[request]; ok {
		c.result = first
		c.retryable = !c.retried && slices.ContainsFunc(onceFixable, func(p string) bool { return strings.HasPrefix(first, p) })
		o.used[request] = c
	}
}

// RefusedFor reports whether tool will only refuse this caller: an admin-only tool for someone who
// isn't an admin, or a people-only tool for another bot's line.
func RefusedFor(tool tools.Tool, chatCtx ChatContextInterface) bool {
	for {
		switch t := tool.(type) {
		case *AdminOnlyTool:
			if !chatCtx.IsAdmin() {
				return true
			}
			tool = t.Tool
		case *PeopleOnlyTool:
			if chatCtx.IsBotLine() {
				return true
			}
			tool = t.Tool
		case *OncePerRequestTool:
			tool = t.Tool
		default:
			return false
		}
	}
}

// RegisterIRCTools registers IRC tools as native tools with polly's registry
func RegisterIRCTools(registry *tools.ToolRegistry) {
	for name, f := range nativeFactories {
		registry.RegisterNative(name, f)
	}
}

// nativeFactories are the bot's own tools, loaded by name from the tool list.
var nativeFactories = map[string]func() tools.Tool{
	"irc__op":            newIrcOpTool,
	"irc__kick":          newIrcKickTool,
	"irc__ban":           newIrcBanTool,
	"irc__topic":         newIrcTopicTool,
	"irc__action":        newIrcActionTool,
	"irc__mode_set":      newIrcModeSetTool,
	"irc__mode_query":    newIrcModeQueryTool,
	"irc__invite":        newIrcInviteTool,
	"irc__names":         newIrcNamesTool,
	"irc__whois":         newIrcWhoisTool,
	"irc__ignore":        newIrcIgnoreTool,
	"irc__slap":          newIrcSlapTool,
	"memory__remember":   newMemoryRememberTool,
	"memory__recall":     newMemoryRecallTool,
	"memory__forget":     newMemoryForgetTool,
	"memory__correct":    newMemoryCorrectTool,
	"history__search":    newHistorySearchTool,
	"task__start":        newTaskStartTool,
	"task__schedule":     newTaskScheduleTool,
	"goal__propose":      newGoalProposeTool,
	"todo__set":          newTodoSetTool,
	"todo__update":       newTodoUpdateTool,
	"task__note":         newTaskNoteTool,
	"irc__remind":        newIrcRemindTool,
	"irc__reminders":     newIrcRemindersTool,
	"profile__timezone":  newProfileTimezoneTool,
	"profile__localtime": newProfileLocalTimeTool,
}

func newIrcOpTool() tools.Tool {
	return &tools.Func{
		Name: "irc__op",
		Desc: "Grant or revoke IRC operator status for one or more users",
		Params: schema.Params{
			"users": schema.Strings("List of user nicknames to op/deop"),
			"grant": schema.Bool("true to grant op, false to revoke"),
		},
		Required: []string{"users", "grant"},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, msg, err := validateAdminOp(ctx)
			if err != nil || msg != "" {
				return msg, err
			}

			users := args.StringSlice("users")
			if len(users) == 0 {
				return "", fmt.Errorf("users must be a non-empty array of strings")
			}

			mode := "-o"
			if args.Bool("grant") {
				mode = "+o"
			}

			channel := chatCtx.GetConfig().Server.Channel
			for _, nick := range users {
				if err := ctx.Err(); err != nil {
					return "", err
				}
				chatCtx.SetMode(channel, mode, nick)
			}

			usersStr := strings.Join(users, ", ")
			chatCtx.GetLogger().Info("irc_op", "mode", mode, "users", usersStr, "channel", channel)
			return fmt.Sprintf("Set mode %s for %s", mode, usersStr), nil
		},
	}
}

func newIrcKickTool() tools.Tool {
	return &tools.Func{
		Name: "irc__kick",
		Desc: "Kick one or more users from the IRC channel",
		Params: schema.Params{
			"users":  schema.Strings("List of user nicknames to kick"),
			"reason": schema.S("The reason for kicking"),
		},
		Required: []string{"users", "reason"},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, msg, err := validateAdminOp(ctx)
			if err != nil || msg != "" {
				return msg, err
			}

			users := args.StringSlice("users")
			if len(users) == 0 {
				return "", fmt.Errorf("users must be a non-empty array of strings")
			}
			reason := args.String("reason")

			channel := chatCtx.GetConfig().Server.Channel
			for _, nick := range users {
				if err := ctx.Err(); err != nil {
					return "", err
				}
				chatCtx.Kick(channel, nick, reason)
			}

			usersStr := strings.Join(users, ", ")
			chatCtx.GetLogger().Info("irc_kick", "users", usersStr, "channel", channel, "reason", reason)
			return fmt.Sprintf("Kicked %s: %s", usersStr, reason), nil
		},
	}
}

func newIrcBanTool() tools.Tool {
	return &tools.Func{
		Name: "irc__ban",
		Desc: "Ban or unban a user from the IRC channel",
		Params: schema.Params{
			"target": schema.S("The nick or hostmask to ban/unban"),
			"ban":    schema.Bool("true to ban, false to unban"),
		},
		Required: []string{"target", "ban"},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, msg, err := validateAdminOp(ctx)
			if err != nil || msg != "" {
				return msg, err
			}

			target := args.String("target")
			ban := args.Bool("ban")

			action := "Unbanned"
			if ban {
				action = "Banned"
			}

			// If target doesn't contain wildcards or !, assume it's a nick and convert to hostmask.
			// Prefer *!ident@host (specific) over *!*@host (banning everyone on the host).
			banMask := target
			if !strings.Contains(target, "!") && !strings.Contains(target, "*") {
				if user := chatCtx.GetUser(target); user != nil {
					banMask = fmt.Sprintf("*!%s@%s", user.Ident, user.Host)
					chatCtx.GetLogger().Info("irc_ban_lookup", "target", target, "ident", user.Ident, "host", user.Host, "ban_mask", banMask, "found", true)
				} else {
					banMask = target + "!*@*"
					chatCtx.GetLogger().Info("irc_ban_lookup", "target", target, "ban_mask", banMask, "found", false)
				}
			}

			channel := chatCtx.GetConfig().Server.Channel
			if ban {
				chatCtx.Ban(channel, banMask)
			} else {
				chatCtx.Unban(channel, banMask)
			}

			chatCtx.GetLogger().Info("irc_ban", "action", action, "ban_mask", banMask, "channel", channel)
			return fmt.Sprintf("%s %s", action, banMask), nil
		},
	}
}

func newIrcTopicTool() tools.Tool {
	return &tools.Func{
		Name: "irc__topic",
		Desc: "Set the IRC channel topic",
		Params: schema.Params{
			"topic": schema.S("The new topic for the channel"),
		},
		Required: []string{"topic"},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, msg, err := validateAdminOp(ctx)
			if err != nil || msg != "" {
				return msg, err
			}

			topic := args.String("topic")
			channel := chatCtx.GetConfig().Server.Channel
			chatCtx.Topic(channel, topic)

			chatCtx.GetLogger().Info("irc_topic", "channel", channel, "topic", topic)
			return fmt.Sprintf("Set topic: %s", topic), nil
		},
	}
}

func newIrcActionTool() tools.Tool {
	return &tools.Func{
		Name: "irc__action",
		Desc: "Send an action message to the IRC channel",
		Params: schema.Params{
			"message": schema.S("The action message to send"),
		},
		Required: []string{"message"},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, err := validateContext(ctx)
			if err != nil {
				return "", err
			}

			message := args.String("message")
			channel := chatCtx.GetConfig().Server.Channel
			chatCtx.SendAction(channel, message)

			chatCtx.GetLogger().Info("irc_action", "message", message)
			return fmt.Sprintf("* %s", message), nil
		},
	}
}

func newIrcModeSetTool() tools.Tool {
	return &tools.Func{
		Name: "irc__mode_set",
		Desc: "Set or unset channel-wide modes like +m (moderated), +t (topic protection), +n (no external messages), +i (invite only), +k (channel key), +l (user limit)",
		Params: schema.Params{
			"modes": schema.S("The mode string to set, e.g., '+m', '-t', '+mnt', '+k password', '+l 50'"),
		},
		Required: []string{"modes"},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, msg, err := validateAdminOp(ctx)
			if err != nil || msg != "" {
				return msg, err
			}

			modes := args.String("modes")
			parts := strings.Fields(modes)
			if len(parts) == 0 {
				return "", fmt.Errorf("modes string cannot be empty")
			}

			modeFlags := parts[0]
			modeParams := parts[1:]

			channel := chatCtx.GetConfig().Server.Channel
			if len(modeParams) > 0 {
				chatCtx.SetMode(channel, modeFlags, modeParams...)
			} else {
				chatCtx.SetMode(channel, modeFlags)
			}

			chatCtx.GetLogger().Info("irc_mode_set", "modes", modes, "channel", channel)
			return fmt.Sprintf("Set channel mode %s on %s", modes, channel), nil
		},
	}
}

func newIrcModeQueryTool() tools.Tool {
	return &tools.Func{
		Name:   "irc__mode_query",
		Desc:   "Query the current channel modes (uses cached state, instant response)",
		Params: schema.Params{},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, err := validateContext(ctx)
			if err != nil {
				return "", err
			}

			channel := chatCtx.GetConfig().Server.Channel
			ch := chatCtx.GetChannel(channel)

			if ch == nil {
				return fmt.Sprintf("Channel %s not found in state (not joined yet?)", channel), nil
			}

			modeStr := ch.Modes
			if modeStr == "" {
				return fmt.Sprintf("Channel %s has no modes set", channel), nil
			}

			chatCtx.GetLogger().Info("irc_mode_query", "channel", channel, "modes", modeStr)
			return fmt.Sprintf("Channel %s modes: %s", channel, modeStr), nil
		},
	}
}

func newIrcInviteTool() tools.Tool {
	return &tools.Func{
		Name: "irc__invite",
		Desc: "Invite one or more users to the IRC channel",
		Params: schema.Params{
			"users": schema.Strings("List of user nicknames to invite"),
		},
		Required: []string{"users"},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, msg, err := validateAdminOp(ctx)
			if err != nil || msg != "" {
				return msg, err
			}

			users := args.StringSlice("users")
			if len(users) == 0 {
				return "", fmt.Errorf("users must be a non-empty array of strings")
			}

			channel := chatCtx.GetConfig().Server.Channel
			for _, user := range users {
				if err := ctx.Err(); err != nil {
					return "", err
				}
				chatCtx.Invite(channel, user)
			}

			usersStr := strings.Join(users, ", ")
			chatCtx.GetLogger().Info("irc_invite", "users", usersStr, "channel", channel)
			return fmt.Sprintf("Invited %s to %s", usersStr, channel), nil
		},
	}
}

func newIrcNamesTool() tools.Tool {
	return &tools.Func{
		Name:   "irc__names",
		Desc:   "List all users currently in the IRC channel (uses cached state, instant response)",
		Params: schema.Params{},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, err := validateContext(ctx)
			if err != nil {
				return "", err
			}

			channel := chatCtx.GetConfig().Server.Channel
			users := chatCtx.GetChannelUsers(channel)

			if users == nil {
				return fmt.Sprintf("Channel %s not found in state (not joined yet?)", channel), nil
			}
			if len(users) == 0 {
				return fmt.Sprintf("No users found in %s", channel), nil
			}

			nicks := make([]string, 0, len(users))
			for _, user := range users {
				prefix := ""
				if user.IsOp {
					prefix = "@"
				} else if user.IsVoice {
					prefix = "+"
				}
				nicks = append(nicks, prefix+user.Nick)
			}

			nicksStr := strings.Join(nicks, ", ")
			chatCtx.GetLogger().Info("irc_names", "channel", channel, "count", len(users), "nicks", nicksStr)
			return fmt.Sprintf("Users in %s (%d): %s", channel, len(users), nicksStr), nil
		},
	}
}

func newIrcWhoisTool() tools.Tool {
	return &tools.Func{
		Name: "irc__whois",
		Desc: "Get detailed information about a user (uses cached state, instant response)",
		Params: schema.Params{
			"nick": schema.S("The nickname to look up"),
		},
		Required: []string{"nick"},
		Run: func(ctx context.Context, args tools.Args) (string, error) {
			chatCtx, err := validateContext(ctx)
			if err != nil {
				return "", err
			}

			nick := args.String("nick")
			user := chatCtx.GetUser(nick)
			if user == nil {
				return fmt.Sprintf("User %s not found in cached state", nick), nil
			}

			var info strings.Builder
			fmt.Fprintf(&info, "User: %s\n", user.Nick)
			fmt.Fprintf(&info, "Hostmask: %s!%s@%s\n", user.Nick, user.Ident, user.Host)
			if user.RealName != "" {
				fmt.Fprintf(&info, "Real name: %s\n", user.RealName)
			}
			if user.Account != "" {
				fmt.Fprintf(&info, "Account: %s (authenticated)\n", user.Account)
			}
			if user.Away != "" {
				fmt.Fprintf(&info, "Away: %s\n", user.Away)
			}
			if len(user.Channels) > 0 {
				fmt.Fprintf(&info, "Channels (%d): %s\n", len(user.Channels), strings.Join(user.Channels, ", "))
			}

			chatCtx.GetLogger().Info("irc_whois", "nick", nick)
			return strings.TrimSpace(info.String()), nil
		},
	}
}
