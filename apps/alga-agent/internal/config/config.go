// Package config loads and validates Alga Agent configuration from a YAML file
// with environment variable expansion. Environment variables override YAML
// values so that secrets can be managed out-of-band from the config file.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration object for the Alga Agent.
type Config struct {
	Agent         AgentConfig         `yaml:"agent"`
	Model         ModelConfig         `yaml:"model"`
	Telegram      TelegramConfig      `yaml:"telegram"`
	Alga          AlgaConfig          `yaml:"alga"`
	Tools         ToolsConfig         `yaml:"tools"`
	AgentBehavior AgentBehaviorConfig `yaml:"agent_behavior"`
	Sessions      SessionsConfig      `yaml:"sessions"`
	Logging       LoggingConfig       `yaml:"logging"`
	Metrics       MetricsConfig       `yaml:"metrics"`
	MCP           MCPConfig           `yaml:"mcp"`
}

type AgentConfig struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type ModelConfig struct {
	Provider    string  `yaml:"provider"`
	BaseURL     string  `yaml:"base_url"`
	APIKey      string  `yaml:"api_key"`
	Model       string  `yaml:"model"`
	MaxTokens   int     `yaml:"max_tokens"`
	Temperature float64 `yaml:"temperature"`
}

type TelegramConfig struct {
	Enabled      bool     `yaml:"enabled"`
	BotToken     string   `yaml:"bot_token"`
	AllowedUsers []string `yaml:"allowed_users"`
	WebhookURL   string   `yaml:"webhook_url"`
	WebhookAddr  string   `yaml:"webhook_addr"`
	// RespondInGroups controls whether the bot replies in group chats when it
	// is not directly mentioned or replied to. Defaults to false (mention-only).
	RespondInGroups bool `yaml:"respond_in_groups"`
	// MinEditInterval bounds how often a streaming message is edited.
	MinEditInterval time.Duration `yaml:"min_edit_interval"`
}

type AlgaConfig struct {
	Enabled    bool   `yaml:"enabled"`
	ServerURL  string `yaml:"server_url"`
	AgentToken string `yaml:"agent_token"`
}

type ToolsConfig struct {
	Shell     ShellConfig     `yaml:"shell"`
	WebSearch WebSearchConfig `yaml:"web_search"`
}

type ShellConfig struct {
	Enabled         bool          `yaml:"enabled"`
	AllowedCommands []string      `yaml:"allowed_commands"`
	MaxOutputBytes  int           `yaml:"max_output_bytes"`
	Timeout         time.Duration `yaml:"timeout"`
}

type WebSearchConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Provider   string `yaml:"provider"`
	APIKey     string `yaml:"api_key"`
	MaxResults int    `yaml:"max_results"`
	// FetchContent enables fetching full page content for selected results.
	FetchContent bool `yaml:"fetch_content"`
}

type AgentBehaviorConfig struct {
	MaxIterations    int           `yaml:"max_iterations"`
	ToolTimeout      time.Duration `yaml:"tool_timeout"`
	ContextWindow    int           `yaml:"context_window"`
	SystemPromptFile string        `yaml:"system_prompt_file"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
	// File is the log file path. Empty = default <data dir>/logs/agent.log;
	// the literal "stderr" disables file logging.
	File string `yaml:"file"`
	// MaxSizeMB is the per-file rotation threshold in megabytes.
	MaxSizeMB int `yaml:"max_size_mb"`
	// BackupCount is the number of rotated log files kept.
	BackupCount int `yaml:"backup_count"`
}

// SessionsConfig controls on-disk session memory. When enabled, each
// conversation is written as a JSON file after every turn and reloaded
// lazily after restarts or idle eviction.
type SessionsConfig struct {
	Persist bool `yaml:"persist"`
	// Dir overrides the session directory (default <data dir>/sessions).
	Dir string `yaml:"dir"`
	// RetentionDays deletes session files older than this. 0 = keep forever.
	RetentionDays int `yaml:"retention_days"`
}

type MetricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Addr    string `yaml:"addr"`
}

// MCPConfig controls the Model Context Protocol server and clients attached
// to the agent. The agent acts as both an MCP server (exposing its tools to
// external MCP clients like Claude Desktop, Cursor, etc.) and an MCP client
// (consuming tools from external MCP servers — filesystem browsers, GitHub
// clients, database inspectors, custom in-house tools).
type MCPConfig struct {
	// Server, when enabled, binds an HTTP listener that serves the agent's
	// tools over the MCP Streamable HTTP transport at Path.
	Server MCPServerConfig `yaml:"server"`
	// Clients lists external MCP servers to consume tools from. Each
	// imported tool is exposed to the LLM under a namespaced name
	// ("<name>_<tool>") to avoid collisions with Alga tools.
	Clients []MCPClientConfig `yaml:"clients"`
}

// MCPServerConfig configures the MCP server the agent exposes.
type MCPServerConfig struct {
	Enabled bool   `yaml:"enabled"`
	Addr    string `yaml:"addr"`
	Path    string `yaml:"path"`
}

// MCPClientConfig describes one external MCP server to import tools from.
// Either Command (stdio transport) or URL (HTTP transport) must be set.
type MCPClientConfig struct {
	Name        string   `yaml:"name"`
	Command     string   `yaml:"command"`
	Args        []string `yaml:"args"`
	Env         []string `yaml:"env"`
	URL         string   `yaml:"url"`
	ToolPrefix  string   `yaml:"tool_prefix"`
	InitTimeout string   `yaml:"init_timeout"`
	Disabled    bool     `yaml:"disabled"`
}

// DefaultPath resolves the config file location. Resolution order:
//  1. an explicit path argument (non-empty)
//  2. $ALGA_AGENT_CONFIG
//  3. ./config.yaml in the current working directory (back-compat)
//  4. $ALGA_AGENT_HOME/config.yaml
//  5. $HOME/.alga/config.yaml
//
// The ./config.yaml candidate is only used when it exists so that a fresh
// checkout (no local config) transparently falls through to the user data
// directory used by `alga-agent setup`.
func DefaultPath(path string) string {
	if path == "" {
		path = os.Getenv("ALGA_AGENT_CONFIG")
	}
	if path == "" {
		if _, err := os.Stat("config.yaml"); err == nil {
			return "config.yaml"
		}
		path = filepath.Join(ResolveDataDir(), "config.yaml")
	}
	return path
}

// ResolveDataDir returns the agent's user data directory: $ALGA_AGENT_HOME if
// set, otherwise $HOME/.alga. The directory is not created here; callers that
// intend to write (e.g. the setup wizard) should os.MkdirAll it.
func ResolveDataDir() string {
	if v := os.Getenv("ALGA_AGENT_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".alga")
}

// Load reads configuration from the file at path (resolved via DefaultPath
// when empty). Environment variables override YAML values per SPEC §8.2.
func Load(path string) (*Config, error) {
	path = DefaultPath(path)

	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// No config file is allowed as long as env vars cover the
			// required fields; validate will catch missing values.
			applyEnvOverrides(cfg)
			if err := cfg.Validate(); err != nil {
				return nil, fmt.Errorf("config validation: %w", err)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	expanded := expandEnv(string(data))
	dec := yaml.NewDecoder(strings.NewReader(expanded))
	dec.KnownFields(true)
	// Default() pre-fills the OpenRouter URL; clear it so applyDefaults can
	// derive the canonical URL from the YAML provider unless the YAML sets
	// base_url explicitly.
	cfg.Model.BaseURL = ""
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	cfg.applyDefaults()
	applyEnvOverrides(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation: %w", err)
	}

	return cfg, nil
}

// Save marshals cfg to YAML and writes it to path (resolved via DefaultPath
// when empty). The file is created with mode 0600 since it may contain API
// keys and bot tokens. The parent directory is created if missing.
//
// Save is intended for the interactive setup wizard; the running agent never
// persists configuration. Values are written literally — secrets are not
// redacted on disk, mirroring the hermes-agent config.yaml convention.
func Save(path string, cfg *Config) error {
	path = DefaultPath(path)
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create config dir %s: %w", dir, err)
		}
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

// Default returns a Config populated with sensible defaults.
func Default() *Config {
	return &Config{
		Agent: AgentConfig{
			Name:        "Alga Agent",
			Description: "SRE assistant for the Alga platform",
		},
		Model: ModelConfig{
			Provider:    "openrouter",
			BaseURL:     "https://openrouter.ai/api/v1",
			Model:       "openrouter/free",
			MaxTokens:   4096,
			Temperature: 0.3,
		},
		Telegram: TelegramConfig{
			WebhookAddr:     "0.0.0.0:8443",
			MinEditInterval: time.Second,
		},
		Alga: AlgaConfig{ServerURL: "http://localhost:8080"},
		Tools: ToolsConfig{
			Shell: ShellConfig{
				Enabled:         true,
				AllowedCommands: []string{"ls", "cat", "grep", "head", "tail", "wc", "curl", "dig", "ping", "ps", "top"},
				MaxOutputBytes:  10240,
				Timeout:         30 * time.Second,
			},
			WebSearch: WebSearchConfig{
				Enabled:    true,
				Provider:   "duckduckgo",
				MaxResults: 5,
			},
		},
		AgentBehavior: AgentBehaviorConfig{
			MaxIterations: 30,
			ToolTimeout:   30 * time.Second,
			ContextWindow: 20,
		},
		Sessions: SessionsConfig{Persist: true},
		Logging:  LoggingConfig{Level: "info", MaxSizeMB: 5, BackupCount: 3},
		Metrics:  MetricsConfig{Enabled: false, Addr: "127.0.0.1:9101"},
	}
}

// providerBaseURLs maps known provider ids to their canonical
// OpenAI-compatible endpoints (hermes-agent provider registry).
var providerBaseURLs = map[string]string{
	"openrouter":          "https://openrouter.ai/api/v1",
	"openai":              "https://api.openai.com/v1",
	"opencode-zen":        "https://opencode.ai/zen/v1",
	"opencode-go":         "https://opencode.ai/zen/go/v1",
	"zai":                 "https://api.z.ai/api/paas/v4",
	"zai-coding-plan":     "https://api.z.ai/api/coding/paas/v4",
	"alibaba":             "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
	"alibaba-coding-plan": "https://coding-intl.dashscope.aliyuncs.com/v1",
}

// BaseURLForProvider returns the canonical endpoint for a known provider id,
// defaulting to OpenRouter for unknown or custom providers.
func BaseURLForProvider(provider string) string {
	if u, ok := providerBaseURLs[provider]; ok {
		return u
	}
	return providerBaseURLs["openrouter"]
}

// providerKeyEnvVars lists provider-specific API-key env vars (first non-empty
// wins). OpenRouter/OpenAI keys are handled generically in applyEnvOverrides.
var providerKeyEnvVars = map[string][]string{
	"opencode-zen":        {"OPENCODE_ZEN_API_KEY"},
	"opencode-go":         {"OPENCODE_GO_API_KEY"},
	"zai":                 {"ZAI_API_KEY", "GLM_API_KEY", "Z_AI_API_KEY"},
	"zai-coding-plan":     {"ZAI_API_KEY", "GLM_API_KEY", "Z_AI_API_KEY"},
	"alibaba":             {"DASHSCOPE_API_KEY"},
	"alibaba-coding-plan": {"ALIBABA_CODING_PLAN_API_KEY", "DASHSCOPE_API_KEY"},
}

// apiKeyEnvHint returns the env var name(s) to surface in the missing-key
// validation error for the given provider, defaulting to the generic
// OpenRouter/OpenAI keys for providers without specific ones.
func apiKeyEnvHint(provider string) string {
	if vars := providerKeyEnvVars[provider]; len(vars) > 0 {
		return strings.Join(vars, " / ")
	}
	return "OPENROUTER_API_KEY / OPENAI_API_KEY"
}

func (c *Config) applyDefaults() {
	if c.Agent.Name == "" {
		c.Agent.Name = "Alga Agent"
	}
	if c.Agent.Description == "" {
		c.Agent.Description = "SRE assistant for the Alga platform"
	}
	if c.Model.Provider == "" {
		c.Model.Provider = "openrouter"
	}
	if c.Model.BaseURL == "" {
		c.Model.BaseURL = BaseURLForProvider(c.Model.Provider)
	}
	if c.Model.Model == "" {
		c.Model.Model = "openrouter/free"
	}
	if c.Model.MaxTokens == 0 {
		c.Model.MaxTokens = 4096
	}
	if c.Model.Temperature == 0 {
		c.Model.Temperature = 0.3
	}
	if c.AgentBehavior.MaxIterations == 0 {
		c.AgentBehavior.MaxIterations = 30
	}
	if c.AgentBehavior.ToolTimeout == 0 {
		c.AgentBehavior.ToolTimeout = 30 * time.Second
	}
	if c.AgentBehavior.ContextWindow == 0 {
		c.AgentBehavior.ContextWindow = 20
	}
	if c.Tools.Shell.Timeout == 0 {
		c.Tools.Shell.Timeout = 30 * time.Second
	}
	if c.Tools.Shell.MaxOutputBytes == 0 {
		c.Tools.Shell.MaxOutputBytes = 10240
	}
	if c.Tools.WebSearch.Provider == "" {
		c.Tools.WebSearch.Provider = "duckduckgo"
	}
	if c.Tools.WebSearch.MaxResults == 0 {
		c.Tools.WebSearch.MaxResults = 5
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.MaxSizeMB == 0 {
		c.Logging.MaxSizeMB = 5
	}
	if c.Logging.BackupCount == 0 {
		c.Logging.BackupCount = 3
	}
	if c.Metrics.Addr == "" {
		c.Metrics.Addr = "127.0.0.1:9101"
	}
	if c.Telegram.WebhookAddr == "" {
		c.Telegram.WebhookAddr = "0.0.0.0:8443"
	}
	if c.Telegram.MinEditInterval == 0 {
		c.Telegram.MinEditInterval = time.Second
	}
	if c.Alga.ServerURL == "" {
		c.Alga.ServerURL = "http://localhost:8080"
	}
	if c.MCP.Server.Addr == "" {
		c.MCP.Server.Addr = "127.0.0.1:8085"
	}
	if c.MCP.Server.Path == "" {
		c.MCP.Server.Path = "/mcp"
	}
}

// applyEnvOverrides applies environment variables over YAML values, per SPEC §8.2.
// Environment variables always win, allowing secrets management out-of-band.
func applyEnvOverrides(c *Config) {
	// Generic OpenAI/OpenRouter keys only apply to the generic providers so a
	// stray OPENROUTER_API_KEY is never used for, say, the alibaba provider.
	switch c.Model.Provider {
	case "", "openai", "openrouter":
		if v := os.Getenv("OPENAI_API_KEY"); v != "" {
			c.Model.APIKey = v
		}
		// OPENROUTER_API_KEY wins over OPENAI_API_KEY when both are set.
		if v := os.Getenv("OPENROUTER_API_KEY"); v != "" {
			c.Model.APIKey = v
		}
	}
	// Provider-specific key env vars take precedence for their provider.
	for _, name := range providerKeyEnvVars[c.Model.Provider] {
		if v := os.Getenv(name); v != "" {
			c.Model.APIKey = v
			break
		}
	}
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
		c.Model.BaseURL = v
	}
	if v := os.Getenv("OPENAI_MODEL"); v != "" {
		c.Model.Model = v
	}
	if v := os.Getenv("TELEGRAM_BOT_TOKEN"); v != "" {
		c.Telegram.BotToken = v
	}
	if v := os.Getenv("ALGA_SERVER_URL"); v != "" {
		c.Alga.ServerURL = v
	}
	if v := os.Getenv("ALGA_AGENT_TOKEN"); v != "" {
		c.Alga.AgentToken = v
	}
	if v := os.Getenv("SEARCH_API_KEY"); v != "" {
		c.Tools.WebSearch.APIKey = v
	}
	// Booleans: non-empty value interpreted as "enable unless explicitly falsey".
	if v, ok := os.LookupEnv("ALGA_TELEGRAM_ENABLED"); ok {
		c.Telegram.Enabled = parseBoolDefault(v, c.Telegram.Enabled)
	}
	if v, ok := os.LookupEnv("ALGA_ALGA_ENABLED"); ok {
		c.Alga.Enabled = parseBoolDefault(v, c.Alga.Enabled)
	}
}

func parseBoolDefault(v string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on", "y", "t":
		return true
	case "0", "false", "no", "off", "n", "f":
		return false
	default:
		return def
	}
}

// Validate checks required configuration and returns a descriptive error for
// the first missing or invalid field. Startup errors are fatal per SPEC §8.3.
func (c *Config) Validate() error {
	var errs []string

	if c.Model.APIKey == "" {
		errs = append(errs, fmt.Sprintf("model.api_key (or %s) is required", apiKeyEnvHint(c.Model.Provider)))
	}
	if c.Model.BaseURL == "" {
		errs = append(errs, "model.base_url is required")
	}
	if c.Model.Model == "" {
		errs = append(errs, "model.model is required")
	}
	if c.AgentBehavior.MaxIterations <= 0 {
		errs = append(errs, "agent_behavior.max_iterations must be > 0")
	}
	if c.AgentBehavior.ContextWindow <= 1 {
		errs = append(errs, "agent_behavior.context_window must be > 1")
	}

	if c.Telegram.Enabled {
		if c.Telegram.BotToken == "" {
			errs = append(errs, "telegram.bot_token (or TELEGRAM_BOT_TOKEN) is required when telegram.enabled")
		}
		if c.Telegram.WebhookURL != "" {
			if c.Telegram.MinEditInterval < 0 {
				errs = append(errs, "telegram.min_edit_interval must be >= 0")
			}
		}
	}

	if c.Alga.Enabled {
		if c.Alga.ServerURL == "" {
			errs = append(errs, "alga.server_url (or ALGA_SERVER_URL) is required when alga.enabled")
		}
		if c.Alga.AgentToken == "" {
			errs = append(errs, "alga.agent_token (or ALGA_AGENT_TOKEN) is required when alga.enabled")
		}
	}

	if !c.Telegram.Enabled && !c.Alga.Enabled {
		errs = append(errs, "at least one channel (telegram or alga) must be enabled")
	}

	if c.Tools.Shell.Enabled && c.Tools.Shell.Timeout <= 0 {
		errs = append(errs, "tools.shell.timeout must be > 0 when shell enabled")
	}
	if c.Tools.Shell.Enabled && c.Tools.Shell.MaxOutputBytes <= 0 {
		errs = append(errs, "tools.shell.max_output_bytes must be > 0 when shell enabled")
	}
	if c.Tools.WebSearch.Enabled {
		switch c.Tools.WebSearch.Provider {
		case "duckduckgo", "brave", "tavily":
		default:
			errs = append(errs, "tools.web_search.provider must be one of: duckduckgo, brave, tavily")
		}
		if (c.Tools.WebSearch.Provider == "brave" || c.Tools.WebSearch.Provider == "tavily") && c.Tools.WebSearch.APIKey == "" {
			errs = append(errs, "tools.web_search.api_key (or SEARCH_API_KEY) is required for "+c.Tools.WebSearch.Provider)
		}
	}

	if c.Sessions.RetentionDays < 0 {
		errs = append(errs, "sessions.retention_days must be >= 0")
	}
	if c.Logging.MaxSizeMB < 0 {
		errs = append(errs, "logging.max_size_mb must be >= 0")
	}
	if c.Logging.BackupCount < 0 {
		errs = append(errs, "logging.backup_count must be >= 0")
	}

	if c.Metrics.Enabled && c.Metrics.Addr == "" {
		errs = append(errs, "metrics.addr is required when metrics.enabled")
	}

	// MCP config: validate each external client has either Command or URL,
	// not both, and that init_timeout (if set) parses as a duration.
	seenMCPNames := make(map[string]struct{})
	for i, mcpClient := range c.MCP.Clients {
		if mcpClient.Name == "" {
			errs = append(errs, fmt.Sprintf("mcp.clients[%d].name is required", i))
			continue
		}
		if _, dup := seenMCPNames[mcpClient.Name]; dup {
			errs = append(errs, fmt.Sprintf("mcp.clients[%d].name %q duplicates an earlier entry", i, mcpClient.Name))
			continue
		}
		seenMCPNames[mcpClient.Name] = struct{}{}
		if mcpClient.Command == "" && mcpClient.URL == "" {
			errs = append(errs, fmt.Sprintf("mcp.clients[%d] (%s): command or url is required", i, mcpClient.Name))
		}
		if mcpClient.Command != "" && mcpClient.URL != "" {
			errs = append(errs, fmt.Sprintf("mcp.clients[%d] (%s): command and url are mutually exclusive", i, mcpClient.Name))
		}
		if mcpClient.InitTimeout != "" {
			if _, err := ParseDuration(mcpClient.InitTimeout); err != nil {
				errs = append(errs, fmt.Sprintf("mcp.clients[%d] (%s): invalid init_timeout: %v", i, mcpClient.Name, err))
			}
		}
	}

	if len(errs) == 1 {
		return errors.New(errs[0])
	}
	if len(errs) > 1 {
		return fmt.Errorf("multiple config errors:\n  - %s", strings.Join(errs, "\n  - "))
	}
	return nil
}

// AllowedCommandSet returns the set of allowed shell commands for O(1) lookup.
func (sc *ShellConfig) AllowedCommandSet() map[string]struct{} {
	out := make(map[string]struct{}, len(sc.AllowedCommands))
	for _, c := range sc.AllowedCommands {
		out[c] = struct{}{}
	}
	return out
}

// ParseDuration parses a Go duration string, returning a helpful error.
func ParseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	return d, nil
}

var envVarRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandEnv replaces ${VAR} references with the value of the matching env var.
// Unknown variables expand to an empty string. The bare $VAR form is not
// supported on purpose to avoid surprising expansion in shell snippets.
func expandEnv(s string) string {
	return envVarRe.ReplaceAllStringFunc(s, func(m string) string {
		name := envVarRe.FindStringSubmatch(m)[1]
		return os.Getenv(name)
	})
}
