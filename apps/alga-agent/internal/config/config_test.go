package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoad_DefaultsApplied(t *testing.T) {
	path := writeTempConfig(t, `
model:
  api_key: "test-key"
telegram:
  enabled: true
  bot_token: "tok"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Agent.Name != "Alga Agent" {
		t.Errorf("default agent name = %q, want %q", cfg.Agent.Name, "Alga Agent")
	}
	if cfg.AgentBehavior.MaxIterations != 30 {
		t.Errorf("default max_iterations = %d, want 30", cfg.AgentBehavior.MaxIterations)
	}
	if cfg.AgentBehavior.ContextWindow != 20 {
		t.Errorf("default context_window = %d, want 20", cfg.AgentBehavior.ContextWindow)
	}
	if cfg.AgentBehavior.ToolTimeout != 30*time.Second {
		t.Errorf("default tool_timeout = %v, want 30s", cfg.AgentBehavior.ToolTimeout)
	}
	if cfg.Model.Model != "gpt-4o" {
		t.Errorf("default model = %q, want gpt-4o", cfg.Model.Model)
	}
	if !cfg.Sessions.Persist {
		t.Error("default sessions.persist should be true")
	}
	if cfg.Sessions.RetentionDays != 0 {
		t.Errorf("default sessions.retention_days = %d, want 0", cfg.Sessions.RetentionDays)
	}
	if cfg.Logging.MaxSizeMB != 5 {
		t.Errorf("default logging.max_size_mb = %d, want 5", cfg.Logging.MaxSizeMB)
	}
	if cfg.Logging.BackupCount != 3 {
		t.Errorf("default logging.backup_count = %d, want 3", cfg.Logging.BackupCount)
	}
}

func TestLoad_SessionsPersistExplicitFalse(t *testing.T) {
	path := writeTempConfig(t, `
model:
  api_key: "test-key"
telegram:
  enabled: true
  bot_token: "tok"
sessions:
  persist: false
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Sessions.Persist {
		t.Error("sessions.persist should honor explicit false")
	}
}

func TestValidate_SessionsAndLoggingBounds(t *testing.T) {
	cfg := Default()
	cfg.Model.APIKey = "k"
	cfg.Telegram.Enabled = true
	cfg.Telegram.BotToken = "tok"
	cfg.Sessions.RetentionDays = -1
	cfg.Logging.MaxSizeMB = -1
	cfg.Logging.BackupCount = -1
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, want := range []string{"sessions.retention_days", "logging.max_size_mb", "logging.backup_count"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %s, got %v", want, err)
		}
	}
}

func TestLoad_EnvExpansion(t *testing.T) {
	t.Setenv("MY_TEST_KEY", "expanded-value")
	os.Unsetenv("OPENAI_API_KEY") // avoid env override clobbering the expansion test
	path := writeTempConfig(t, `
model:
  api_key: ${MY_TEST_KEY}
  model: "gpt-4o-mini"
telegram:
  enabled: true
  bot_token: "tok"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Model.APIKey != "expanded-value" {
		t.Errorf("env-expanded api_key = %q, want %q", cfg.Model.APIKey, "expanded-value")
	}
}

func TestLoad_EnvOverridesYAML(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "override-key")
	path := writeTempConfig(t, `
model:
  api_key: "yaml-key"
  model: "gpt-4o"
telegram:
  enabled: true
  bot_token: "tok"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Model.APIKey != "override-key" {
		t.Errorf("env override api_key = %q, want %q", cfg.Model.APIKey, "override-key")
	}
}

func TestValidate_RequiresAPIKey(t *testing.T) {
	cfg := Default()
	cfg.Telegram.Enabled = true
	cfg.Telegram.BotToken = "tok"
	cfg.Model.APIKey = "" // missing
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "api_key") {
		t.Fatalf("expected api_key error, got: %v", err)
	}
}

func TestValidate_RequiresAtLeastOneChannel(t *testing.T) {
	cfg := Default()
	cfg.Model.APIKey = "k"
	cfg.Telegram.Enabled = false
	cfg.Alga.Enabled = false
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "at least one channel") {
		t.Fatalf("expected channel error, got: %v", err)
	}
}

func TestValidate_TelegramRequiresToken(t *testing.T) {
	cfg := Default()
	cfg.Model.APIKey = "k"
	cfg.Telegram.Enabled = true
	cfg.Telegram.BotToken = ""
	cfg.Alga.Enabled = false
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "bot_token") {
		t.Fatalf("expected bot_token error, got: %v", err)
	}
}

func TestValidate_AlgaRequiresCredentials(t *testing.T) {
	cfg := Default()
	cfg.Model.APIKey = "k"
	cfg.Telegram.Enabled = false
	cfg.Alga.Enabled = true
	cfg.Alga.AgentToken = ""
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "agent_token") {
		t.Fatalf("expected agent_token error, got: %v", err)
	}
}

func TestValidate_WebSearchProviderRequiresKey(t *testing.T) {
	cfg := Default()
	cfg.Model.APIKey = "k"
	cfg.Telegram.Enabled = false
	cfg.Alga.Enabled = false
	cfg.Telegram.Enabled = true
	cfg.Telegram.BotToken = "tok"
	cfg.Tools.WebSearch.Enabled = true
	cfg.Tools.WebSearch.Provider = "brave"
	cfg.Tools.WebSearch.APIKey = ""
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "api_key") {
		t.Fatalf("expected api_key error for brave, got: %v", err)
	}
}

func TestShellConfig_AllowedCommandSet(t *testing.T) {
	sc := ShellConfig{AllowedCommands: []string{"ls", "grep"}}
	set := sc.AllowedCommandSet()
	if _, ok := set["ls"]; !ok {
		t.Error("ls not in allowed set")
	}
	if _, ok := set["rm"]; ok {
		t.Error("rm should not be in allowed set")
	}
}

func TestLoad_NoFileUsesEnvVars(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-only-key")
	t.Setenv("TELEGRAM_BOT_TOKEN", "env-tok")
	t.Setenv("ALGA_TELEGRAM_ENABLED", "true")
	// Point ALGA_AGENT_HOME at an empty temp dir so neither ./config.yaml nor
	// ~/.alga/config.yaml leaks into the "no file" path.
	t.Setenv("ALGA_AGENT_HOME", t.TempDir())
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load with no file: %v", err)
	}
	if cfg.Model.APIKey != "env-only-key" {
		t.Errorf("api_key = %q, want env-only-key", cfg.Model.APIKey)
	}
	if !cfg.Telegram.Enabled {
		t.Error("telegram should be enabled via env")
	}
}

func TestLoad_NoFileNoChannelFails(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "env-only-key")
	os.Unsetenv("TELEGRAM_BOT_TOKEN")
	os.Unsetenv("ALGA_AGENT_TOKEN")
	t.Setenv("ALGA_AGENT_HOME", t.TempDir())
	_, err := Load("")
	if err == nil || !strings.Contains(err.Error(), "at least one channel") {
		t.Fatalf("expected channel error, got: %v", err)
	}
}
