// Command alga-agent runs the Alga AIOps AI assistant. It connects to Telegram
// and/or Alga investigation threads, processes messages through an LLM
// tool-calling loop, and responds via the originating channel.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	alga "github.com/alga/agent-sdk-go"

	"alga-agent/internal/agent"
	"alga-agent/internal/channels"
	"alga-agent/internal/config"
	"alga-agent/internal/llm"
	"alga-agent/internal/logging"
	"alga-agent/internal/mcp"
	agentmetrics "alga-agent/internal/metrics"
	"alga-agent/internal/service"
	"alga-agent/internal/setup"
	"alga-agent/internal/tools"
	"alga-agent/internal/version"
)

const usage = `alga-agent — Alga AIOps AI assistant

Usage:
  alga-agent [command]

Commands:
  (none)           run the agent
  setup [section]  run the interactive configuration wizard
  service <verb>   manage the systemd user service
                   verbs: install [--force] [--enable=false] [--now=false],
                          uninstall, start, stop, restart, status
  version          print the version
  help             show this help
`

func main() {
	// Subcommand dispatch. The default (no subcommand) runs the agent.
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "setup":
			section := ""
			if len(os.Args) >= 3 {
				section = os.Args[2]
			}
			if err := setup.Run(section); err != nil {
				if errors.Is(err, setup.ErrAbort) {
					// User cancelled — not a failure worth a non-zero exit.
					return
				}
				fmt.Fprintf(os.Stderr, "alga-agent: %v\n", err)
				os.Exit(1)
			}
			return
		case "service":
			if err := runService(os.Args[2:]); err != nil {
				fmt.Fprintf(os.Stderr, "alga-agent: %v\n", err)
				os.Exit(1)
			}
			return
		case "version", "-v", "--version":
			fmt.Printf("alga-agent %s\n", version.Version)
			return
		case "help", "-h", "--help":
			fmt.Print(usage)
			return
		default:
			fmt.Fprintf(os.Stderr, "alga-agent: unknown command %q\n\n%s", os.Args[1], usage)
			os.Exit(1)
		}
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "alga-agent: %v\n", err)
		os.Exit(1)
	}
}

// runService dispatches `alga-agent service <verb>` to the systemd user
// service manager.
func runService(args []string) error {
	if len(args) == 0 {
		return errors.New("service: missing verb (install|uninstall|start|stop|restart|status)")
	}
	verb, rest := args[0], args[1:]
	w := os.Stdout
	switch verb {
	case "install":
		fs := flag.NewFlagSet("service install", flag.ContinueOnError)
		force := fs.Bool("force", false, "overwrite an existing, different unit file")
		enable := fs.Bool("enable", true, "enable the service (start on login)")
		now := fs.Bool("now", true, "start the service immediately")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		return service.Install(w, service.InstallOptions{Force: *force, Enable: *enable, Now: *now})
	case "uninstall":
		return service.Uninstall(w)
	case "start":
		return service.Start(w)
	case "stop":
		return service.Stop(w)
	case "restart":
		return service.Restart(w)
	case "status":
		return service.Status(w)
	default:
		return fmt.Errorf("service: unknown verb %q (install|uninstall|start|stop|restart|status)", verb)
	}
}

func run() error {
	cfg, err := config.Load("")
	if err != nil {
		return err
	}

	logCloser, err := logging.Setup(logging.Options{
		Level:       cfg.Logging.Level,
		File:        cfg.Logging.File,
		MaxSizeMB:   cfg.Logging.MaxSizeMB,
		BackupCount: cfg.Logging.BackupCount,
	})
	if err != nil {
		return fmt.Errorf("logging setup: %w", err)
	}
	defer func() {
		if logCloser != nil {
			_ = logCloser.Close()
		}
	}()

	logger := logging.Logger.With("version", version.Version)
	// Route the Alga SDK's log sink through slog.
	alga.Logf = func(format string, args ...any) {
		logger.Debug(fmt.Sprintf(format, args...))
	}

	logger.Info("starting alga-agent",
		"telegram_enabled", cfg.Telegram.Enabled,
		"alga_enabled", cfg.Alga.Enabled,
		"model", cfg.Model.Model,
		"base_url", cfg.Model.BaseURL)

	// --- Tool registry ---
	registry := tools.NewRegistry()

	// Alga tools (only when the Alga channel or Alga tools are available).
	var algaClient *alga.AlgaClient
	if cfg.Alga.Enabled {
		algaClient = alga.NewAlgaClient(cfg.Alga.ServerURL, cfg.Alga.AgentToken,
			alga.WithLogger(alga.AsLogger(logger.With("component", "sdk"))))
		tools.RegisterAlgaTools(registry, algaClient)
	}

	// Shell tool.
	tools.RegisterShellTool(registry, tools.NewShellTool(cfg.Tools.Shell))

	// Web search tool.
	tools.RegisterWebSearchTool(registry, tools.NewWebSearchTool(cfg.Tools.WebSearch))

	// --- MCP client: import external MCP servers as agent tools ---
	// Connected before the loop starts so the imported tools are visible in
	// the first message turn. Connection failures are logged and skipped — a
	// misbehaving external server must not block agent startup.
	mcpClient := mcp.NewClient(mcp.WithClientLogger(logger.With("component", "mcp_client")))
	if len(cfg.MCP.Clients) > 0 {
		importConfigs := make([]mcp.RemoteServerConfig, 0, len(cfg.MCP.Clients))
		for _, c := range cfg.MCP.Clients {
			t, _ := config.ParseDuration(c.InitTimeout)
			importConfigs = append(importConfigs, mcp.RemoteServerConfig{
				Name:        c.Name,
				Command:     c.Command,
				Args:        c.Args,
				Env:         c.Env,
				URL:         c.URL,
				ToolPrefix:  c.ToolPrefix,
				InitTimeout: t,
				Disabled:    c.Disabled,
			})
		}
		// Use a bounded-time context so a hung MCP server doesn't block startup.
		connectCtx, connectCancel := context.WithTimeout(context.Background(), 30*time.Second)
		imported, err := mcpClient.Connect(connectCtx, registry, importConfigs)
		connectCancel()
		if err != nil {
			logger.Warn("one or more MCP clients failed to connect", "err", err)
		}
		if imported > 0 {
			logger.Info("imported tools from external MCP servers", "count", imported)
		}
	}

	logger.Info("tools registered", "count", len(registry.List()))

	// --- LLM client ---
	llmClient := llm.New(cfg.Model.BaseURL, cfg.Model.APIKey, cfg.Model.Model,
		llm.WithMaxTokens(cfg.Model.MaxTokens),
		llm.WithTemperature(cfg.Model.Temperature),
		llm.WithLogger(logger.With("component", "llm")),
	)

	// --- Agent core ---
	core := agent.New(agent.Options{
		LLM:        llmClient,
		Tools:      registry,
		Behavior:   cfg.AgentBehavior,
		Agent:      cfg.Agent,
		PromptFile: cfg.AgentBehavior.SystemPromptFile,
		Logger:     logger.With("component", "agent"),
	})

	// --- Session persistence ---
	if cfg.Sessions.Persist {
		sessDir := cfg.Sessions.Dir
		if sessDir == "" {
			sessDir = filepath.Join(config.ResolveDataDir(), "sessions")
		}
		if err := os.MkdirAll(sessDir, 0o700); err != nil {
			return fmt.Errorf("create sessions dir %s: %w", sessDir, err)
		}
		core.Store().EnablePersistence(sessDir)
		logger.Info("session persistence enabled", "dir", sessDir)
	}

	// --- Metrics ---
	mtr := agentmetrics.NewStandard()
	mtr.ActiveSessions.Set(0)
	if cfg.Metrics.Enabled {
		mux := http.NewServeMux()
		mux.Handle("/metrics", mtr.Registry.HTTPHandler())
		metricsSrv := &http.Server{
			Addr:              cfg.Metrics.Addr,
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		}
		go func() {
			logger.Info("metrics server listening", "addr", cfg.Metrics.Addr)
			if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("metrics server error", "err", err)
			}
		}()
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = metricsSrv.Shutdown(ctx)
		}()
	}

	// --- Router ---
	router := channels.NewRouter(core, logger.With("component", "router"))

	// --- Channels ---
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// --- MCP server: expose agent tools to external MCP clients ---
	// Started after ctx is available so graceful shutdown cancels it cleanly.
	var mcpServer *mcp.Server
	if cfg.MCP.Server.Enabled {
		mcpServer = mcp.NewServer(registry,
			mcp.WithServerLogger(logger.With("component", "mcp_server")),
			mcp.WithServerImplementation(&mcp.Implementation{
				Name:    cfg.Agent.Name,
				Version: version.Version,
			}),
		)
		go func() {
			if err := mcpServer.Start(ctx, cfg.MCP.Server.Addr, cfg.MCP.Server.Path); err != nil {
				logger.Error("mcp server stopped", "err", err)
			}
		}()
	}

	var (
		wg       sync.WaitGroup
		chans    []channels.Channel
		telegram *channels.TelegramChannel
		algaChan *channels.AlgaChannel
	)

	if cfg.Telegram.Enabled {
		tg, err := channels.NewTelegramChannel(cfg.Telegram, router, logger.With("component", "telegram"))
		if err != nil {
			return fmt.Errorf("telegram init: %w", err)
		}
		telegram = tg
		router.RegisterSink(telegram.Name(), telegram)
		chans = append(chans, telegram)
	}

	if cfg.Alga.Enabled {
		ac, err := channels.NewAlgaChannel(cfg.Alga, router, logger.With("component", "alga"))
		if err != nil {
			return fmt.Errorf("alga init: %w", err)
		}
		algaChan = ac
		router.RegisterSink(algaChan.Name(), algaChan)
		chans = append(chans, algaChan)
	}

	// Start all channels concurrently (SPEC §9).
	for _, ch := range chans {
		wg.Add(1)
		go func(c channels.Channel) {
			defer wg.Done()
			if err := c.Start(ctx); err != nil {
				logger.Error("channel start failed", "channel", c.Name(), "err", err)
			}
		}(ch)
	}

	// --- Session eviction sweep ---
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				n := core.Store().EvictIdle(30 * time.Minute)
				if n > 0 {
					logger.Info("evicted idle sessions", "count", n)
					mtr.ActiveSessions.Set(uint64(core.Store().Size()))
				}
				if cfg.Sessions.Persist && cfg.Sessions.RetentionDays > 0 {
					retention := time.Duration(cfg.Sessions.RetentionDays) * 24 * time.Hour
					pruned, err := core.Store().PruneFiles(retention)
					if err != nil {
						logger.Warn("session retention sweep failed", "err", err)
					} else if pruned > 0 {
						logger.Info("pruned expired session files", "count", pruned)
					}
				}
			}
		}
	}()

	// --- Wait for shutdown signal ---
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("shutdown signal received", "signal", sig.String())

	// --- Graceful shutdown (SPEC §9) ---
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	cancel() // stop accepting new messages / cancel long-lived contexts

	// Stop channels concurrently.
	var stopWg sync.WaitGroup
	for _, ch := range chans {
		stopWg.Add(1)
		go func(c channels.Channel) {
			defer stopWg.Done()
			if err := c.Stop(); err != nil {
				logger.Warn("channel stop error", "channel", c.Name(), "err", err)
			}
		}(ch)
	}
	stopWg.Wait()

	// Disconnect MCP clients (external servers we were consuming).
	mcpClient.Disconnect()

	// Shut down the MCP server (external clients consuming us).
	if mcpServer != nil {
		shutdownMCPCtx, shutdownMCPCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := mcpServer.Shutdown(shutdownMCPCtx); err != nil {
			logger.Warn("mcp server shutdown error", "err", err)
		}
		shutdownMCPCancel()
	}

	// Wait for in-flight message processing to drain (bounded by shutdownCtx).
	// router.Wait() drains the per-message dispatch goroutines; wg drains the
	// long-lived goroutines (channel polling, eviction sweep).
	drainDone := make(chan struct{})
	go func() {
		router.Wait()
		wg.Wait()
		close(drainDone)
	}()
	select {
	case <-drainDone:
		logger.Info("drain complete")
	case <-shutdownCtx.Done():
		logger.Warn("drain timed out, forcing exit")
	}

	logger.Info("alga-agent stopped")
	return nil
}

// keep an import to avoid unused warnings if certain configs are off.
var _ = slog.LevelInfo
