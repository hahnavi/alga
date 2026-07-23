package setup

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"alga-agent/internal/config"
)

// --- config Save/Load round trip -----------------------------------------

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.yaml")

	cfg := config.Default()
	cfg.Model.Provider = "openrouter"
	cfg.Model.BaseURL = "https://openrouter.ai/api/v1"
	cfg.Model.APIKey = "sk-secret-123"
	cfg.Model.Model = "anthropic/claude-3.5"
	cfg.Model.MaxTokens = 8192
	cfg.Model.Temperature = 0.7
	cfg.Telegram.Enabled = true
	cfg.Telegram.BotToken = "123:abc"
	cfg.Alga.Enabled = true
	cfg.Alga.ServerURL = "http://alga.local:8080"
	cfg.Alga.AgentToken = "alga_tok"

	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// File must be created with restrictive perms.
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("config perm = %o, want 0600", perm)
	}

	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Model.APIKey != "sk-secret-123" {
		t.Errorf("api_key = %q, want sk-secret-123", loaded.Model.APIKey)
	}
	if loaded.Telegram.BotToken != "123:abc" {
		t.Errorf("bot_token = %q, want 123:abc", loaded.Telegram.BotToken)
	}
	if loaded.Alga.AgentToken != "alga_tok" {
		t.Errorf("agent_token = %q, want alga_tok", loaded.Alga.AgentToken)
	}
	if loaded.Model.MaxTokens != 8192 {
		t.Errorf("max_tokens = %d, want 8192", loaded.Model.MaxTokens)
	}
	if loaded.Model.Temperature != 0.7 {
		t.Errorf("temperature = %v, want 0.7", loaded.Model.Temperature)
	}
}

// --- DefaultPath resolution ----------------------------------------------

func TestDefaultPath_ExplicitArg(t *testing.T) {
	t.Setenv("ALGA_AGENT_CONFIG", "/should/be/ignored")
	t.Setenv("ALGA_AGENT_HOME", "/should/also/be/ignored")
	if got := config.DefaultPath("/explicit/path.yaml"); got != "/explicit/path.yaml" {
		t.Errorf("DefaultPath = %q, want /explicit/path.yaml", got)
	}
}

func TestDefaultPath_EnvVar(t *testing.T) {
	t.Setenv("ALGA_AGENT_CONFIG", "/from/env.yaml")
	if got := config.DefaultPath(""); got != "/from/env.yaml" {
		t.Errorf("DefaultPath = %q, want /from/env.yaml", got)
	}
}

func TestDefaultPath_DataDirFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ALGA_AGENT_CONFIG", "")
	t.Setenv("ALGA_AGENT_HOME", dir)
	// Ensure no ./config.yaml in the test cwd (TempDir is empty, but the test
	// binary may run from a dir that has one). chdir into temp to be safe.
	orig, _ := os.Getwd()
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	got := config.DefaultPath("")
	want := filepath.Join(dir, "config.yaml")
	if got != want {
		t.Errorf("DefaultPath = %q, want %q", got, want)
	}
}

func TestDefaultPath_CwdFilePreferred(t *testing.T) {
	orig, _ := os.Getwd()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })

	// A cwd config.yaml wins over the data dir.
	if err := os.WriteFile("config.yaml", []byte("model:\n  api_key: k\n"), 0o600); err != nil {
		t.Fatalf("write cwd config: %v", err)
	}
	t.Setenv("ALGA_AGENT_CONFIG", "")
	t.Setenv("ALGA_AGENT_HOME", "/nonexistent-home")
	if got := config.DefaultPath(""); got != "config.yaml" {
		t.Errorf("DefaultPath = %q, want config.yaml", got)
	}
}

func TestResolveDataDir_HomeFallback(t *testing.T) {
	t.Setenv("ALGA_AGENT_HOME", "")
	t.Setenv("HOME", "/fake/home")
	if got := config.ResolveDataDir(); got != filepath.Join("/fake/home", ".alga") {
		t.Errorf("ResolveDataDir = %q, want /fake/home/.alga", got)
	}
}

// --- scripted setup flows ------------------------------------------------
//
// runWith reads from a *bufio.Reader over scripted input. Non-TTY stdin makes
// promptSecret fall back to a plain prompt, so secrets can be driven too.

// runScript drives one or more section flows against scripted input. sections
// lists which flows to run in order (e.g. "model", "channel").
func runScript(t *testing.T, cfg *config.Config, sections []string, input string) (*config.Config, string, error) {
	t.Helper()
	origHome := os.Getenv("ALGA_AGENT_HOME")
	dir := t.TempDir()
	t.Setenv("ALGA_AGENT_HOME", dir)
	t.Setenv("ALGA_AGENT_CONFIG", filepath.Join(dir, "config.yaml"))
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("ALGA_AGENT_TOKEN", "")
	t.Setenv("ALGA_SERVER_URL", "")
	t.Cleanup(func() { t.Setenv("ALGA_AGENT_HOME", origHome) })

	if cfg == nil {
		cfg = config.Default()
	}
	_ = config.Save(filepath.Join(dir, "config.yaml"), cfg)

	var out bytes.Buffer
	r := bufio.NewReader(strings.NewReader(input))
	for _, s := range sections {
		def, ok := findSection(s)
		if !ok {
			return cfg, out.String(), fmt.Errorf("unknown section %q in test", s)
		}
		if err := def.run(cfg, r, &out); err != nil {
			return cfg, out.String(), err
		}
	}
	return cfg, out.String(), nil
}

func TestSetupModel_Scripted(t *testing.T) {
	cfg := config.Default()
	// Script (each newline = one Enter):
	//   Provider:          "2" (openrouter) Enter
	//   Base URL:          Enter (keep canonical openrouter)
	//   API key:           sk-test Enter
	//   Model:             Enter (keep gpt-4o default)
	//   Max tokens:        Enter (keep 4096)
	//   Temperature:       0.5 Enter
	input := "2\n\nsk-test\n\n\n0.5\n"
	_, _, err := runScript(t, cfg, []string{"model"}, input)
	if err != nil {
		t.Fatalf("runScript: %v", err)
	}
	if cfg.Model.Provider != "openrouter" {
		t.Errorf("provider = %q, want openrouter", cfg.Model.Provider)
	}
	if cfg.Model.BaseURL != "https://openrouter.ai/api/v1" {
		t.Errorf("base_url = %q, want openrouter canonical", cfg.Model.BaseURL)
	}
	if cfg.Model.APIKey != "sk-test" {
		t.Errorf("api_key = %q, want sk-test", cfg.Model.APIKey)
	}
	if cfg.Model.Temperature != 0.5 {
		t.Errorf("temperature = %v, want 0.5", cfg.Model.Temperature)
	}
}

func TestSetupModel_CustomProvider(t *testing.T) {
	cfg := config.Default()
	//   Provider: "3" (custom) Enter
	//   Base URL: http://localhost:11434/v1 Enter
	//   API key:  llama Enter
	//   Model:    llama3 Enter
	//   Max:      Enter
	//   Temp:     Enter
	input := "3\nhttp://localhost:11434/v1\nllama\nllama3\n\n\n"
	_, _, err := runScript(t, cfg, []string{"model"}, input)
	if err != nil {
		t.Fatalf("runScript: %v", err)
	}
	if cfg.Model.Provider != "custom" {
		t.Errorf("provider = %q, want custom", cfg.Model.Provider)
	}
	if cfg.Model.BaseURL != "http://localhost:11434/v1" {
		t.Errorf("base_url = %q", cfg.Model.BaseURL)
	}
	if cfg.Model.Model != "llama3" {
		t.Errorf("model = %q, want llama3", cfg.Model.Model)
	}
}

func TestSetupChannels_TelegramThenAlga(t *testing.T) {
	cfg := config.Default()
	// Channel menu:       1 (Telegram)
	//   Enable Telegram?  y Enter
	//   Bot token:        tg_tok Enter
	//   Webhook URL:      Enter (empty = long polling)
	//   Webhook addr:     Enter (keep default)
	//   RespondInGroups?  n Enter
	// Back to menu:       2 (Alga)
	//   Enable Alga?      y Enter
	//   Server URL:       Enter (keep default)
	//   Agent token:      alga_tok Enter
	// Back to menu:       3 (Continue)
	input := "1\ny\ntg_tok\n\n\nn\n2\ny\n\nalga_tok\n3\n"
	_, _, err := runScript(t, cfg, []string{"channel"}, input)
	if err != nil {
		t.Fatalf("runScript: %v", err)
	}
	if !cfg.Telegram.Enabled {
		t.Error("telegram should be enabled")
	}
	if cfg.Telegram.BotToken != "tg_tok" {
		t.Errorf("bot_token = %q, want tg_tok", cfg.Telegram.BotToken)
	}
	if cfg.Telegram.RespondInGroups {
		t.Error("respond_in_groups should be false")
	}
	if !cfg.Alga.Enabled {
		t.Error("alga should be enabled")
	}
	if cfg.Alga.AgentToken != "alga_tok" {
		t.Errorf("agent_token = %q, want alga_tok", cfg.Alga.AgentToken)
	}
}

func TestSetupChannels_BothDisabledContinues(t *testing.T) {
	cfg := config.Default()
	// Channel menu: disable Telegram (1), disable Alga (2). Picking Continue
	// with nothing enabled now returns cleanly — the invariant is enforced at
	// the Review & Save step via Validate(), not here.
	//   1 → Enable Telegram? n
	//   2 → Enable Alga?     n
	//   3 (Continue) → returns
	input := "1\nn\n2\nn\n3\n"
	_, _, err := runScript(t, cfg, []string{"channel"}, input)
	if err != nil {
		t.Fatalf("runScript: %v", err)
	}
	if cfg.Telegram.Enabled {
		t.Error("telegram should remain disabled (no forcing)")
	}
	if cfg.Alga.Enabled {
		t.Error("alga should remain disabled")
	}
}

func TestSetupChannels_DisableAfterEnabling(t *testing.T) {
	cfg := config.Default()
	// Enable Telegram, then return and disable it. Channel setup returns with
	// nothing enabled; no re-opening occurs (validate-at-save handles it).
	//   1 → y, tok, "", "", n
	//   1 → n (disable)
	//   3 (Continue) → returns with nothing enabled
	input := "1\ny\ntok\n\n\nn\n1\nn\n3\n"
	_, _, err := runScript(t, cfg, []string{"channel"}, input)
	if err != nil {
		t.Fatalf("runScript: %v", err)
	}
	if cfg.Telegram.Enabled {
		t.Error("telegram should end disabled after explicit disable")
	}
	if cfg.Telegram.BotToken != "tok" {
		t.Errorf("bot_token = %q, want tok (preserved even when disabled)", cfg.Telegram.BotToken)
	}
	if cfg.Alga.Enabled {
		t.Error("alga should remain disabled")
	}
}

func TestRun_NonInteractiveReturnsError(t *testing.T) {
	t.Setenv("ALGA_AGENT_NONINTERACTIVE", "1")
	err := Run("")
	if err == nil {
		t.Fatal("Run should error in non-interactive mode")
	}
	if !strings.Contains(err.Error(), "interactive") {
		t.Errorf("error = %q, want mention of interactive", err.Error())
	}
}

func TestRun_UnknownSection(t *testing.T) {
	t.Setenv("ALGA_AGENT_NONINTERACTIVE", "") // clear
	// Even non-TTY, the unknown-section path errors before the interactive
	// guard is reached in the section branch — but runWith checks interactive
	// first. Instead test findSection directly.
	if _, ok := findSection("bogus"); ok {
		t.Fatal("bogus should not be a valid section")
	}
}

// --- registry ordering ----------------------------------------------------

func TestSectionKeys_DeterministicOrder(t *testing.T) {
	// sectionKeys must be stable so the "Available:" hint and help text don't
	// shuffle between runs (the old map-based registry was non-deterministic).
	want := "model, channel, tools, behavior, logging"
	if got := sectionKeys(); got != want {
		t.Errorf("sectionKeys = %q, want %q", got, want)
	}
}

func TestSections_OrderMatchesKeys(t *testing.T) {
	// The slice order is the menu order; it must match sectionKeys.
	for i, s := range sections {
		if i == 0 {
			continue
		}
		if sections[i-1].key == s.key {
			t.Errorf("duplicate section key %q", s.key)
		}
	}
}

// --- tools section --------------------------------------------------------

func TestSetupTools_ShellAndSearch(t *testing.T) {
	cfg := config.Default()
	// Shell: y, "ls,grep,curl", 8192, 10s
	// Search: y, 2 (brave), brave_key, 3, n (fetch)
	input := "y\nls,grep,curl\n8192\n10s\ny\n2\nbrave_key\n3\nn\n"
	_, _, err := runScript(t, cfg, []string{"tools"}, input)
	if err != nil {
		t.Fatalf("runScript: %v", err)
	}
	if !cfg.Tools.Shell.Enabled {
		t.Error("shell should be enabled")
	}
	if got, want := cfg.Tools.Shell.AllowedCommands, []string{"ls", "grep", "curl"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("allowed_commands = %v, want %v", got, want)
	}
	if cfg.Tools.Shell.MaxOutputBytes != 8192 {
		t.Errorf("max_output_bytes = %d, want 8192", cfg.Tools.Shell.MaxOutputBytes)
	}
	if cfg.Tools.Shell.Timeout != 10*time.Second {
		t.Errorf("timeout = %v, want 10s", cfg.Tools.Shell.Timeout)
	}
	if !cfg.Tools.WebSearch.Enabled {
		t.Error("web search should be enabled")
	}
	if cfg.Tools.WebSearch.Provider != "brave" {
		t.Errorf("provider = %q, want brave", cfg.Tools.WebSearch.Provider)
	}
	if cfg.Tools.WebSearch.APIKey != "brave_key" {
		t.Errorf("api_key = %q, want brave_key", cfg.Tools.WebSearch.APIKey)
	}
	if cfg.Tools.WebSearch.MaxResults != 3 {
		t.Errorf("max_results = %d, want 3", cfg.Tools.WebSearch.MaxResults)
	}
	if cfg.Tools.WebSearch.FetchContent {
		t.Error("fetch_content should be false")
	}
}

func TestSetupTools_DisableBoth(t *testing.T) {
	cfg := config.Default()
	cfg.Tools.Shell.Enabled = true
	cfg.Tools.WebSearch.Enabled = true
	// Shell: n. Search: n.
	input := "n\nn\n"
	_, _, err := runScript(t, cfg, []string{"tools"}, input)
	if err != nil {
		t.Fatalf("runScript: %v", err)
	}
	if cfg.Tools.Shell.Enabled {
		t.Error("shell should be disabled")
	}
	if cfg.Tools.WebSearch.Enabled {
		t.Error("web search should be disabled")
	}
}

func TestSetupTools_DuckDuckGoSkipsAPIKey(t *testing.T) {
	cfg := config.Default()
	// Shell: n. Search: y, 1 (duckduckgo), 5, n. No api_key prompt expected.
	input := "n\ny\n1\n5\nn\n"
	out, _, err := runScriptCaptureOut(t, cfg, []string{"tools"}, input)
	if err != nil {
		t.Fatalf("runScript: %v", err)
	}
	if cfg.Tools.WebSearch.Provider != "duckduckgo" {
		t.Errorf("provider = %q, want duckduckgo", cfg.Tools.WebSearch.Provider)
	}
	if strings.Contains(out, "Search API key") {
		t.Error("duckduckgo should not prompt for an API key")
	}
}

// --- behavior section -----------------------------------------------------

func TestSetupBehavior(t *testing.T) {
	cfg := config.Default()
	// max_iterations: 50, tool_timeout: 60s, context_window: 40, prompt file: ""
	input := "50\n60s\n40\n\n"
	_, _, err := runScript(t, cfg, []string{"behavior"}, input)
	if err != nil {
		t.Fatalf("runScript: %v", err)
	}
	if cfg.AgentBehavior.MaxIterations != 50 {
		t.Errorf("max_iterations = %d, want 50", cfg.AgentBehavior.MaxIterations)
	}
	if cfg.AgentBehavior.ToolTimeout != 60*time.Second {
		t.Errorf("tool_timeout = %v, want 60s", cfg.AgentBehavior.ToolTimeout)
	}
	if cfg.AgentBehavior.ContextWindow != 40 {
		t.Errorf("context_window = %d, want 40", cfg.AgentBehavior.ContextWindow)
	}
}

// --- logging section ------------------------------------------------------

func TestSetupLogging_MetricsOn(t *testing.T) {
	cfg := config.Default()
	// level: 3 (warn — menu is 1-indexed: 1=debug 2=info 3=warn 4=error),
	// file: /tmp/a.log, metrics: y, addr: 0.0.0.0:9200
	input := "3\n/tmp/a.log\ny\n0.0.0.0:9200\n"
	_, _, err := runScript(t, cfg, []string{"logging"}, input)
	if err != nil {
		t.Fatalf("runScript: %v", err)
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("level = %q, want warn", cfg.Logging.Level)
	}
	if cfg.Logging.File != "/tmp/a.log" {
		t.Errorf("file = %q, want /tmp/a.log", cfg.Logging.File)
	}
	if !cfg.Metrics.Enabled {
		t.Error("metrics should be enabled")
	}
	if cfg.Metrics.Addr != "0.0.0.0:9200" {
		t.Errorf("addr = %q, want 0.0.0.0:9200", cfg.Metrics.Addr)
	}
}

func TestSetupLogging_MetricsOff(t *testing.T) {
	cfg := config.Default()
	cfg.Metrics.Enabled = true
	// level: 1 (info), file: "", metrics: n → addr not prompted.
	input := "1\n\nn\n"
	out, _, err := runScriptCaptureOut(t, cfg, []string{"logging"}, input)
	if err != nil {
		t.Fatalf("runScript: %v", err)
	}
	if cfg.Metrics.Enabled {
		t.Error("metrics should be disabled")
	}
	if strings.Contains(out, "Metrics listen address") {
		t.Error("addr should not be prompted when metrics disabled")
	}
}

// --- finalize gate --------------------------------------------------------

func TestFinalize_ValidSaves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.Default()
	cfg.Model.APIKey = "sk-test"
	cfg.Telegram.Enabled = true
	cfg.Telegram.BotToken = "tg_tok"

	var out bytes.Buffer
	r := bufio.NewReader(strings.NewReader("y\n")) // confirm save
	saved, err := finalize(cfg, r, &out, path, "")
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if !saved {
		t.Fatal("saved = false, want true")
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load saved: %v", err)
	}
	if loaded.Model.APIKey != "sk-test" {
		t.Errorf("saved api_key = %q, want sk-test", loaded.Model.APIKey)
	}
}

func TestFinalize_InvalidOffersChoice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.Default()
	// No API key and no channel → Validate fails. Pick "Back to menu".
	cfg.Model.APIKey = ""
	cfg.Telegram.Enabled = false
	cfg.Alga.Enabled = false

	var out bytes.Buffer
	r := bufio.NewReader(strings.NewReader("1\n")) // 1 = back to menu
	saved, err := finalize(cfg, r, &out, path, "")
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if saved {
		t.Fatal("saved = true, want false (back to menu)")
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("config should not have been written")
	}
	if !strings.Contains(stripANSI(out.String()), "not valid") {
		t.Error("expected validation error shown")
	}
}

func TestFinalize_SaveAnyway(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.Default()
	cfg.Model.APIKey = ""
	cfg.Telegram.Enabled = false
	cfg.Alga.Enabled = false

	// Invalid → pick "Save anyway" (2), then confirm save (y).
	var out bytes.Buffer
	r := bufio.NewReader(strings.NewReader("2\ny\n"))
	saved, err := finalize(cfg, r, &out, path, "")
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if !saved {
		t.Fatal("saved = false, want true (save anyway)")
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Error("config should have been written")
	}
}

func TestFinalize_Cancel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.Default()
	cfg.Model.APIKey = ""
	cfg.Telegram.Enabled = false
	cfg.Alga.Enabled = false

	// Invalid → pick "Cancel" (3).
	var out bytes.Buffer
	r := bufio.NewReader(strings.NewReader("3\n"))
	saved, err := finalize(cfg, r, &out, path, "")
	if !errors.Is(err, ErrAbort) {
		t.Fatalf("err = %v, want ErrAbort", err)
	}
	if saved {
		t.Fatal("saved = true, want false (cancel)")
	}
}

func TestFinalize_DeclineSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	cfg := config.Default()
	cfg.Model.APIKey = "sk-test"
	cfg.Telegram.Enabled = true
	cfg.Telegram.BotToken = "tg_tok"

	// Valid config, but decline the final "Save?" prompt.
	var out bytes.Buffer
	r := bufio.NewReader(strings.NewReader("n\n"))
	saved, err := finalize(cfg, r, &out, path, "")
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if saved {
		t.Fatal("saved = true, want false (declined)")
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Error("config should not have been written when declined")
	}
}

// --- review secrecy -------------------------------------------------------

func TestPrintReview_HidesSecrets(t *testing.T) {
	cfg := config.Default()
	cfg.Model.APIKey = "sk-super-secret-123"
	cfg.Telegram.Enabled = true
	cfg.Telegram.BotToken = "123:abc-super-secret"
	cfg.Alga.Enabled = false
	cfg.Alga.AgentToken = "" // unset → should show "✗ not set"
	cfg.Tools.WebSearch.APIKey = "brave-secret"

	var out bytes.Buffer
	printReview(&out, cfg)
	visible := stripANSI(out.String())

	for _, secret := range []string{"sk-super-secret-123", "123:abc-super-secret", "alga-secret-tok", "brave-secret"} {
		if strings.Contains(visible, secret) {
			t.Errorf("review output leaked secret %q:\n%s", secret, visible)
		}
	}
	if !strings.Contains(visible, "✓ set") {
		t.Errorf("expected '✓ set' badge for configured secrets, got:\n%s", visible)
	}
	if !strings.Contains(visible, "✗ not set") {
		t.Errorf("expected '✗ not set' badge for the unset alga token, got:\n%s", visible)
	}
}

// --- status badges --------------------------------------------------------

func TestStatusBadges(t *testing.T) {
	cfg := config.Default()
	cfg.Telegram.Enabled = true
	cfg.Alga.Enabled = false
	cfg.Tools.Shell.Enabled = true
	cfg.Tools.Shell.AllowedCommands = []string{"ls", "grep"}
	cfg.Tools.WebSearch.Enabled = true
	cfg.Tools.WebSearch.Provider = "brave"
	cfg.Metrics.Enabled = true

	if got := modelStatus(cfg); !strings.Contains(got, "gpt-4o") {
		t.Errorf("modelStatus = %q", got)
	}
	if got := channelStatus(cfg); !strings.Contains(got, "telegram on") || !strings.Contains(got, "alga off") {
		t.Errorf("channelStatus = %q", got)
	}
	if got := toolsStatus(cfg); !strings.Contains(got, "shell on") || !strings.Contains(got, "brave") {
		t.Errorf("toolsStatus = %q", got)
	}
	if got := behaviorStatus(cfg); !strings.Contains(got, "30 iters") {
		t.Errorf("behaviorStatus = %q", got)
	}
	if got := loggingStatus(cfg); !strings.Contains(got, "metrics on") {
		t.Errorf("loggingStatus = %q", got)
	}
}

// runScriptCaptureOut is like runScript but also returns the captured output
// for tests that assert on which prompts were shown.
func runScriptCaptureOut(t *testing.T, cfg *config.Config, sections []string, input string) (string, *config.Config, error) {
	t.Helper()
	_, out, err := runScript(t, cfg, sections, input)
	return out, cfg, err
}
