// Command alga-agent runs the Alga AIOps AI assistant. It connects to Telegram
// and/or Alga investigation threads, processes messages through an LLM
// tool-calling loop, and responds via the originating channel.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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
	"alga-agent/internal/setup"
	"alga-agent/internal/tools"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	// Subcommand dispatch. The default (no subcommand) runs the agent;
	// `setup [section]` runs the interactive configuration wizard.
	if len(os.Args) >= 2 && os.Args[1] == "setup" {
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
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "alga-agent: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load("")
	if err != nil {
		return err
	}

	logCloser, err := logging.Setup(cfg.Logging.Level, cfg.Logging.File)
	if err != nil {
		return fmt.Errorf("logging setup: %w", err)
	}
	defer func() {
		if logCloser != nil {
			_ = logCloser.Close()
		}
	}()

	logger := logging.Logger.With("version", version)
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
				Version: version,
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
