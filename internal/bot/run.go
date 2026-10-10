// Copyright (C) 2023-2026 Alex Schlessinger and soulshack contributors
// Modified 2026 by BareMetal
// SPDX-License-Identifier: GPL-3.0-only

package bot

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lrstanley/girc"

	"B4reMetal/metald/internal/admin"
	"B4reMetal/metald/internal/behaviors"
	"B4reMetal/metald/internal/commands"
	"B4reMetal/metald/internal/config"
	"B4reMetal/metald/internal/core"
	"B4reMetal/metald/internal/irc"
	"B4reMetal/metald/internal/llm"
)

// Run starts the IRC bot with the given configuration
func Run(ctx context.Context, cfg *config.Configuration) error {
	level := cfg.Bot.LogLevel
	if cfg.Bot.Verbose {
		level = "debug"
	}
	// T14: the log file is best-effort. If it can't be opened, log to the console only and say so.
	var logFile *core.RotatingFile
	var logFileErr error
	logPath := core.LogFilePath(cfg.Bot.DataDir, cfg.Bot.LogFile)
	if logPath != "" {
		logFile, logFileErr = core.OpenRotatingFile(logPath, int64(cfg.Bot.LogMaxSizeMB)*1024*1024, cfg.Bot.LogKeep)
	}
	if logFile != nil {
		defer logFile.Close()
		core.InitLogger(level, cfg.Bot.LogFormat, logFile)
		core.GetLogger().Info("log_file_open", "path", logPath)
	} else {
		core.InitLogger(level, cfg.Bot.LogFormat, nil)
		if logFileErr != nil {
			core.GetLogger().Warn("log_file_unavailable", "path", logPath, "error", logFileErr.Error())
		}
	}

	if err := irc.ValidCommandPrefix(cfg.Bot.CommandPrefix); err != nil {
		return fmt.Errorf("commandprefix %q: %w", cfg.Bot.CommandPrefix, err)
	}
	core.SetDataDir(cfg.Bot.DataDir)
	// Plugins log failures and costs here, one log per bot, so the console's spend figures don't mix the
	// testbed's searches with live ones. A tool log set in the real environment wins.
	if os.Getenv("METALD_TOOL_LOG") == "" {
		os.Setenv("METALD_TOOL_LOG", core.DataPath(filepath.Join("logs", "tools.log")))
	}
	commands.OverridesPath = core.DataPath("config-overrides.json")

	// Layer persisted runtime changes (+set, +admins, +tools) over config.yml. Must run
	// before NewSystem, which reads them.
	commands.ApplyOverrides(cfg)
	checkAdminMasks(config.List(&cfg.Bot.Admins))
	core.SetConcurrency(cfg.Bot.MaxConcurrent)
	if lib := core.ExportPluginLib(cfg.Bot.PluginLib); lib != "" {
		core.GetLogger().Info("plugin_lib_exported", "path", lib)
	} else {
		core.GetLogger().Warn("plugin_lib_missing", "path", cfg.Bot.PluginLib)
	}

	sys := NewSystem(cfg)

	// Write changed conversations to disk until shutdown, then once more before returning.
	if ps, ok := sys.GetSessionStore().(*core.PersistentSessionStore); ok {
		flushCtx, stopFlush := context.WithCancel(ctx)
		flushed := make(chan struct{})
		go func() { ps.Run(flushCtx); close(flushed) }()
		defer func() { stopFlush(); <-flushed }()
	}
	go llm.RunChatLogPruner(ctx, cfg.Session.HistoryDays)

	cmdRegistry := newCommandRegistry()

	// Initialize behavior registry (order matters: passive watchers first, addressed last as fallback)
	behaviorRegistry := behaviors.NewRegistry()
	// Lifecycle behaviors
	behaviorRegistry.Register(&behaviors.ConnectedBehavior{})
	behaviorRegistry.Register(&behaviors.NickErrorBehavior{})
	behaviorRegistry.Register(&behaviors.ChannelErrorBehavior{})
	behaviorRegistry.Register(&behaviors.SendErrorBehavior{})
	// Reactive behaviors
	behaviorRegistry.Register(&behaviors.URLBehavior{})
	behaviorRegistry.Register(&behaviors.OpBehavior{})
	behaviorRegistry.Register(&behaviors.JoinBehavior{})
	behaviorRegistry.Register(&behaviors.AddressedBehavior{CmdRegistry: cmdRegistry})
	behaviorRegistry.Register(&behaviors.NonAddressedBehavior{CmdRegistry: cmdRegistry})

	nets := cfg.Networks
	if len(nets) == 0 {
		nets = []*config.ServerConfig{cfg.Server}
	}

	adoptUnscoped(nets)
	consoleUp := startAdmin(ctx, cfg, nets, sys, cmdRegistry)

	// Memories written before this bot knew about networks carry no network of their own.
	if len(nets) > 0 && nets[0].Name != "" {
		if store, err := core.Memories(); err == nil {
			if n, err := store.AdoptUnscopedMemories(nets[0].Name); err != nil {
				slog.Error("memory_migration_failed", "error", err.Error())
			} else if n > 0 {
				slog.Info("memories_assigned_to_network", "network", nets[0].Name, "count", n)
			}
		}
		if n := core.Reminders().AdoptUnscopedReminders(nets[0].Name); n > 0 {
			slog.Info("reminders_assigned_to_network", "network", nets[0].Name, "count", n)
		}
	}

	if cfg.Bot.ConsoleOnly {
		core.KeepOffline("started console-only")
	}
	if reasons := core.OfflineReasons(); len(reasons) > 0 {
		if !consoleUp {
			return fmt.Errorf("not joining IRC, and the console is off: %s", strings.Join(reasons, "; "))
		}
		slog.Warn("irc_skipped", "reasons", strings.Join(reasons, "; "), "console", cfg.Bot.AdminListen)
		<-ctx.Done()
		return nil
	}

	if len(nets) == 1 {
		return runNetwork(ctx, cfg.ForNetwork(nets[0]), sys, behaviorRegistry, cmdRegistry)
	}

	errs := make(chan error, len(nets))
	var wg sync.WaitGroup
	for _, n := range nets {
		wg.Add(1)
		go func(n *config.ServerConfig) {
			defer wg.Done()
			netCfg := cfg.ForNetwork(n)
			if err := runNetwork(ctx, netCfg, sys, behaviorRegistry, cmdRegistry); err != nil {
				slog.Error("network_failed", "network", n.Name, "error", err.Error())
				errs <- fmt.Errorf("network %s: %w", n.Name, err)
			}
		}(n)
	}
	wg.Wait()
	close(errs)

	if err, ok := <-errs; ok {
		return err
	}
	return nil
}

// runNetwork connects one network and serves it until the context ends.
func runNetwork(ctx context.Context, cfg *config.Configuration, sys core.System, behaviorRegistry *behaviors.Registry, cmdRegistry *commands.Registry) error {
	// Channel for fatal IRC errors (nick taken, channel join failures)
	fatalErr := make(chan error, 1)

	ircClient := girc.New(girc.Config{
		Server:     cfg.Server.Server,
		Port:       cfg.Server.Port,
		Nick:       cfg.Server.Nick,
		User:       cfg.Server.Nick,
		Name:       cfg.Server.Nick,
		Version:    "irc client",
		ServerPass: cfg.Server.ServerPass,
		SSL:        cfg.Server.SSL,
		TLSConfig: &tls.Config{
			ServerName:         cfg.Server.Server,
			InsecureSkipVerify: cfg.Server.TLSInsecure,
		},
		HandleNickCollide: func(oldNick string) string {
			return "" // Don't auto-retry, we handle it via ERR_NICKNAMEINUSE
		},
	})

	log := slog.Default()
	if cfg.Server.Name != "" {
		log = log.With("network", cfg.Server.Name)
	}

	if cfg.Server.SASLNick != "" && cfg.Server.SASLPass != "" {
		ircClient.Config.SASL = &girc.SASLPlain{
			User: cfg.Server.SASLNick,
			Pass: cfg.Server.SASLPass,
		}
	}

	go func() {
		<-ctx.Done()
		ircClient.Quit("Shutting down...")
		log.Info("irc_client_closed")
	}()

	// Idle conversations on this network are folded into their channel recap.
	go llm.RunIdleFolder(ctx, cfg, sys.GetSessionStore())

	// Background tasks on this network run one at a time, posting results where they were asked for.
	go llm.RunTaskRunner(ctx, cfg, sys, ircClient, fatalErr)

	// Reminders fire outside any request context, so delivery lives here rather than in the tool that
	// schedules them.
	go core.RunReminderScheduler(ctx, core.Reminders(), cfg.Server.Name, func(channel, message string) bool {
		// T15: hold reminders while paused or stopped; they're delivered (late) after +resume.
		if !ircClient.IsConnected() || core.Halted() {
			return false
		}
		// A13: never deliver into a channel the bot may not speak in. Report it as handled,
		// since retrying could never succeed.
		if !irc.ChannelAllowed(cfg, channel) {
			log.Warn("reminder_blocked_channel", "channel", channel)
			return true
		}
		message = irc.RenderIRCFormatting(message)
		if prefix := cfg.EffectiveResponsePrefix(); prefix != "" {
			message = prefix + " " + message
		}
		ircClient.Cmd.Message(channel, message)
		return true
	})

	// A13: drops events from every channel but the configured one before anything else sees them.
	gate := irc.NewChannelGate(log)

	// Single global handler routes all events through the behavior registry
	ircClient.Handlers.AddBg(girc.ALL_EVENTS, func(client *girc.Client, e girc.Event) {
		if !behaviorRegistry.Handles(e.Command) {
			return
		}
		if !gate.Admit(cfg, &e, client.GetNick(), func(channel string) { client.Cmd.Part(channel) }) {
			return
		}
		if irc.TrackLine(cfg, &e) {
			return
		}
		chatCtx, cancel := irc.NewChatContext(ctx, cfg, sys, client, &e, fatalErr)
		defer cancel()

		// Register the request so "+reset" and a mid-flight ignore can actually stop it, rather than
		// letting it finish and reply to someone who is no longer entitled to an answer.
		if e.Source != nil {
			done := core.Requests().Track(chatCtx.GetLockKey(), chatCtx.GetRequestID(), e.Source.Name, cancel)
			defer done()
		}
		behaviors.ObserveLine(chatCtx, &e, cmdRegistry)
		behaviorRegistry.Process(chatCtx, &e)
	})

	// Reconnect loop
	const maxRetries = 5
	for i := range maxRetries {
		if ctx.Err() != nil {
			return nil
		}

		log.Info("server_connecting",
			"server", ircClient.Config.Server,
			"port", ircClient.Config.Port,
			"tls", ircClient.Config.SSL,
			"sasl", ircClient.Config.SASL != nil,
		)

		gate.Reset()
		if err := ircClient.Connect(); err != nil {
			if ctx.Err() != nil {
				return nil
			}

			// Check for fatal IRC errors (nick taken, channel join failures)
			select {
			case fErr := <-fatalErr:
				return fErr
			default:
			}

			log.Error("connection_failed", "error", err)
			log.Info("connection_retry", "delay_sec", 5, "attempt", i+1, "max_attempts", maxRetries)

			select {
			case <-time.After(5 * time.Second):
				continue
			case <-ctx.Done():
				return nil
			}
		}

		// Check for fatal IRC errors after successful connection closed
		select {
		case fErr := <-fatalErr:
			return fErr
		default:
		}

		return nil
	}

	return fmt.Errorf("failed to connect after %d attempts", maxRetries)
}

// checkAdminMasks reports admin masks that will never match or deserve a second look. With no
// usable admin, nobody can run +resume or +stop, so that case is logged loudly.
func checkAdminMasks(admins []string) {
	usable := 0
	for _, mask := range admins {
		if err := irc.ValidateAdminMask(mask); err != nil {
			core.GetLogger().Error("admin_mask_ignored", "mask", mask, "reason", err.Error())
			continue
		}
		usable++
		if warning := irc.AdminMaskWarning(mask); warning != "" {
			core.GetLogger().Warn("admin_mask_broad", "mask", mask, "note", warning)
		}
	}
	if usable == 0 {
		core.GetLogger().Warn("no_admins", "hint", "set admins in config.yml, or nobody can use admin commands such as +stop")
	}
}

// startAdmin serves the operator page when it is configured; without a token it stays off. It
// reports whether the page is being served.
func startAdmin(ctx context.Context, cfg *config.Configuration, nets []*config.ServerConfig, sys core.System, cmds *commands.Registry) bool {
	if cfg.Bot.AdminListen == "" {
		return false
	}
	if cfg.Bot.AdminToken == "" {
		core.GetLogger().Warn("admin_disabled", "reason", "adminlisten is set but admintoken is empty")
		return false
	}
	if err := admin.CheckListen(cfg.Bot.AdminListen); err != nil {
		core.GetLogger().Error("admin_bind_refused", "error", err.Error())
		return false
	}
	names := make([]string, 0, len(nets))
	for _, n := range nets {
		names = append(names, n.Name)
	}
	var proxies []netip.Prefix
	for _, p := range cfg.Bot.AdminTrustedProxies {
		prefix, err := netip.ParsePrefix(p)
		if addr, aerr := netip.ParseAddr(p); aerr == nil {
			prefix, err = addr.Prefix(addr.BitLen())
		}
		if err != nil {
			core.GetLogger().Error("admin_bad_proxy", "proxy", p, "error", err.Error())
			continue
		}
		proxies = append(proxies, prefix)
	}
	srv, err := admin.NewFromCore(admin.Config{Token: cfg.Bot.AdminToken, Version: "v" + Version, Networks: names,
		Started: time.Now(), ComfyURL: os.Getenv("COMFYUI_URL"), RadioURL: os.Getenv("RADIO_API_URL"),
		TrustedProxies: proxies, UserHeader: cfg.Bot.AdminUserHeader, Users: cfg.Bot.AdminUsers}, core.GetLogger())
	if err != nil {
		core.GetLogger().Error("admin_failed", "error", err.Error())
		return false
	}
	srv.WithMizira(console{cfg: cfg, sys: sys, nets: nets, cmds: cmds})
	go func() {
		core.GetLogger().Info("admin_listening", "addr", cfg.Bot.AdminListen)
		if err := srv.Run(ctx, cfg.Bot.AdminListen); err != nil {
			core.GetLogger().Error("admin_failed", "error", err.Error())
		}
	}()
	return true
}

// newCommandRegistry registers every + command.
func newCommandRegistry() *commands.Registry {
	r := commands.NewRegistry()
	r.Register(&commands.SetCommand{})
	r.Register(&commands.GetCommand{})
	r.Register(commands.NewHelpCommand(r))
	r.Register(&commands.VersionCommand{Version: "v" + Version})
	r.Register(&commands.CompletionCommand{})
	r.Register(&commands.ToolsCommand{})
	r.Register(&commands.AdminCommand{})
	r.Register(&commands.StatsCommand{})
	r.Register(&commands.IgnoreCommand{})
	r.Register(&commands.ScreenCommand{})
	r.Register(&commands.UnscreenCommand{})
	r.Register(&commands.UnignoreCommand{})
	r.Register(&commands.ResetCommand{})
	r.Register(&commands.BackendCommand{})
	r.Register(&commands.ModelsCommand{})
	r.Register(&commands.PromptCommand{})
	r.Register(&commands.MemoriesCommand{})
	r.Register(&commands.ForgetCommand{})
	r.Register(&commands.RememberCommand{})
	r.Register(&commands.RecallCommand{})
	r.Register(&commands.BotsCommand{})
	r.Register(&commands.PauseCommand{})
	r.Register(&commands.ResumeCommand{})
	r.Register(&commands.StopCommand{})
	r.Register(&commands.RecapCommand{})
	r.Register(&commands.SelfNotesCommand{})
	r.Register(&commands.SuspicionCommand{})
	r.Register(&commands.NowPlayingCommand{})
	r.Register(&commands.SkipCommand{})
	r.Register(&commands.TaskCommand{})
	r.Register(&commands.TasksCommand{})
	r.Register(&commands.GoalCommand{})
	r.Register(&commands.ScheduleCommand{})
	return r
}
