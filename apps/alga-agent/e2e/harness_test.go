// Package e2e contains an opt-in, local-only full-stack test for the agent:
// real LLM endpoint, real AlgaChannel over SSE, against a locally running
// Alga backend (docker-compose.e2e.yml stack). Gated by ALGA_AGENT_E2E=1.
package e2e

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	alga "github.com/alga/agent-sdk-go"

	"alga-agent/internal/agent"
	"alga-agent/internal/channels"
	"alga-agent/internal/config"
	"alga-agent/internal/llm"
	"alga-agent/internal/tools"
)

const defaultServerURL = "http://localhost:3100"

// requireE2E gates the test on ALGA_AGENT_E2E=1 and a reachable backend,
// skipping with actionable instructions otherwise. Returns the server URL.
func requireE2E(t *testing.T) string {
	t.Helper()
	if os.Getenv("ALGA_AGENT_E2E") != "1" {
		t.Skip("skipping: set ALGA_AGENT_E2E=1 to run the full-stack E2E test")
	}
	serverURL := strings.TrimRight(os.Getenv("ALGA_E2E_SERVER_URL"), "/")
	if serverURL == "" {
		serverURL = defaultServerURL
	}
	probe := &http.Client{Timeout: 5 * time.Second}
	resp, err := probe.Get(serverURL + "/api/v1/setup/status")
	if err != nil {
		t.Skipf("skipping: backend not reachable at %s (%v)\nstart it with: docker compose -f docker-compose.e2e.yml up --build -d --wait", serverURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("skipping: backend probe %s/api/v1/setup/status returned %d\nstart it with: docker compose -f docker-compose.e2e.yml up --build -d --wait", serverURL, resp.StatusCode)
	}
	return serverURL
}

// testLogWriter forwards agent logs to t.Logf and silently discards writes
// after the test finishes (SDK goroutines may outlive cleanup briefly).
type testLogWriter struct {
	t    *testing.T
	mu   sync.Mutex
	done bool
}

func (w *testLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.done {
		w.t.Logf("[agent] %s", strings.TrimRight(string(p), "\n"))
	}
	return len(p), nil
}

func (w *testLogWriter) close() {
	w.mu.Lock()
	w.done = true
	w.mu.Unlock()
}

// startAgent assembles and starts the real agent stack in-process, mirroring
// main.go run(): Alga tools only, real LLM client from env-derived config,
// AlgaChannel over SSE. Cleanup stops the channel and drains the router.
func startAgent(t *testing.T, serverURL, agentToken string) {
	t.Helper()

	// Point config resolution at a nonexistent file so a developer's real
	// config.yaml / ~/.alga/config.yaml cannot leak into the test; only
	// defaults + env overrides (OPENAI_*, provider keys) apply.
	t.Setenv("ALGA_AGENT_CONFIG", filepath.Join(t.TempDir(), "nonexistent.yaml"))
	cfg, err := config.Parse("")
	if err != nil {
		t.Fatalf("config parse: %v", err)
	}
	cfg.Telegram.Enabled = false
	cfg.Alga = config.AlgaConfig{Enabled: true, ServerURL: serverURL, AgentToken: agentToken}
	cfg.Tools.Shell.Enabled = false
	cfg.Tools.WebSearch.Enabled = false
	cfg.Sessions.Persist = false
	if err := cfg.Validate(); err != nil {
		t.Skipf("skipping: LLM not configured (%v); set OPENAI_API_KEY or OPENROUTER_API_KEY (and optionally OPENAI_BASE_URL, OPENAI_MODEL)", err)
	}

	lw := &testLogWriter{t: t}
	logger := slog.New(slog.NewTextHandler(lw, &slog.HandlerOptions{Level: slog.LevelDebug}))

	registry := tools.NewRegistry()
	sdkClient := alga.NewAlgaClient(serverURL, agentToken)
	tools.RegisterAlgaTools(registry, sdkClient)

	llmClient := llm.New(cfg.Model.BaseURL, cfg.Model.APIKey, cfg.Model.Model,
		llm.WithMaxTokens(cfg.Model.MaxTokens),
		llm.WithTemperature(cfg.Model.Temperature),
		llm.WithLogger(logger.With("component", "llm")),
	)

	core := agent.New(agent.Options{
		LLM:      llmClient,
		Tools:    registry,
		Behavior: cfg.AgentBehavior,
		Agent:    cfg.Agent,
		Logger:   logger.With("component", "agent"),
	})

	router := channels.NewRouter(core, logger.With("component", "router"))
	ch, err := channels.NewAlgaChannel(cfg.Alga, router, logger.With("component", "alga"))
	if err != nil {
		t.Fatalf("alga channel init: %v", err)
	}
	router.RegisterSink(ch.Name(), ch)

	ctx, cancel := context.WithCancel(context.Background())
	if err := ch.Start(ctx); err != nil {
		cancel()
		t.Fatalf("alga channel start: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		if err := ch.Stop(); err != nil {
			t.Logf("alga channel stop: %v", err)
		}
		router.Wait()
		lw.close()
	})
}

// waitFor polls cond every 2s until it returns true or the timeout elapses,
// failing the test with the last observed error on timeout.
func waitFor(t *testing.T, timeout time.Duration, desc string, cond func() (bool, error)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		ok, err := cond()
		if ok {
			return
		}
		lastErr = err
		if time.Now().After(deadline) {
			msg := fmt.Sprintf("timed out after %s waiting for %s", timeout, desc)
			if lastErr != nil {
				msg += fmt.Sprintf(" (last error: %v)", lastErr)
			}
			t.Fatal(msg)
		}
		time.Sleep(2 * time.Second)
	}
}
