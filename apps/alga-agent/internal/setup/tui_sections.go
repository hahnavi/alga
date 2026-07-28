package setup

import (
	"strconv"
	"strings"
	"time"

	"alga-agent/internal/config"
)

type stepKind int

const (
	stepChoice stepKind = iota
	stepText
	stepSecret
	stepYesNo
	stepInt
	stepFloat
	stepDuration
	stepCSV
)

type step struct {
	kind     stepKind
	label    string
	help     string
	choices  []string
	defIdx   int
	def      string
	defBool  bool
	defInt   int
	defFloat float64
	defDur   time.Duration
	defCSV   []string
	key      string
}

func modelSteps(cfg *config.Config) []step {
	defIdx := 0
	for i, p := range providerChoices {
		if p == cfg.Model.Provider {
			defIdx = i
		}
	}

	defBase := cfg.Model.BaseURL
	if cfg.Model.Provider != "custom" {
		defBase = config.BaseURLForProvider(cfg.Model.Provider)
	} else if defBase == "" {
		defBase = config.BaseURLForProvider("openrouter")
	}

	steps := []step{
		{kind: stepChoice, label: "Provider", key: "provider", choices: providerChoices, defIdx: defIdx},
		{kind: stepText, label: "Base URL", key: "base_url", def: defBase},
	}

	if p, ok := providerPresets[cfg.Model.Provider]; ok && p.keyURL != "" && cfg.Model.APIKey == "" {
		steps = append(steps, step{kind: stepSecret, label: "API key", key: "api_key", def: cfg.Model.APIKey, help: "Get one at " + p.keyURL})
	} else {
		steps = append(steps, step{kind: stepSecret, label: "API key", key: "api_key", def: cfg.Model.APIKey})
	}

	steps = append(steps,
		step{kind: stepInt, label: "Max tokens", key: "max_tokens", defInt: orNonZero(cfg.Model.MaxTokens, 4096)},
		step{kind: stepFloat, label: "Temperature", key: "temperature", defFloat: orNonZeroF(cfg.Model.Temperature, 0.3)},
	)
	return steps
}

func channelSteps(cfg *config.Config) []step {
	return []step{
		{kind: stepChoice, label: "Channel", key: "channel_menu", choices: []string{
			"Telegram " + channelBadge(cfg.Telegram.Enabled) + "  (human interface)",
			"Alga " + channelBadge(cfg.Alga.Enabled) + "  (investigation threads)",
			"Done",
		}, defIdx: 0},
	}
}

func telegramSteps(cfg *config.Config) []step {
	steps := []step{
		{kind: stepYesNo, label: "Enable Telegram channel?", key: "tg_enabled", defBool: cfg.Telegram.Enabled},
	}
	if cfg.Telegram.Enabled {
		steps = append(steps,
			step{kind: stepSecret, label: "Bot token (from @BotFather)", key: "tg_token", def: cfg.Telegram.BotToken},
			step{kind: stepText, label: "Webhook URL (empty = long polling)", key: "tg_webhook", def: cfg.Telegram.WebhookURL},
			step{kind: stepText, label: "Webhook listen address", key: "tg_addr", def: orDefault(cfg.Telegram.WebhookAddr, "0.0.0.0:8443")},
			step{kind: stepYesNo, label: "Respond in groups when not @mentioned?", key: "tg_groups", defBool: cfg.Telegram.RespondInGroups},
		)
	}
	return steps
}

func algaSteps(cfg *config.Config) []step {
	steps := []step{
		{kind: stepYesNo, label: "Enable Alga channel?", key: "alga_enabled", defBool: cfg.Alga.Enabled},
	}
	if cfg.Alga.Enabled {
		steps = append(steps,
			step{kind: stepText, label: "Alga server URL", key: "alga_url", def: orDefault(cfg.Alga.ServerURL, "http://localhost:8080")},
			step{kind: stepSecret, label: "Agent token", key: "alga_token", def: cfg.Alga.AgentToken},
		)
	}
	return steps
}

func toolsSteps(cfg *config.Config) []step {
	steps := []step{
		{kind: stepYesNo, label: "Enable shell tool?", key: "shell_enabled", defBool: cfg.Tools.Shell.Enabled,
			help: "Shell runs commands with the agent's privileges — restrict the allowlist."},
	}
	if cfg.Tools.Shell.Enabled {
		steps = append(steps,
			step{kind: stepCSV, label: "Allowed commands (comma-separated)", key: "shell_cmds", defCSV: cfg.Tools.Shell.AllowedCommands},
			step{kind: stepInt, label: "Max output bytes", key: "shell_max", defInt: orNonZero(cfg.Tools.Shell.MaxOutputBytes, 10240)},
			step{kind: stepDuration, label: "Timeout", key: "shell_timeout", defDur: orNonZeroDur(cfg.Tools.Shell.Timeout, 30*time.Second)},
		)
	}
	steps = append(steps,
		step{kind: stepYesNo, label: "Enable web search?", key: "search_enabled", defBool: cfg.Tools.WebSearch.Enabled},
	)
	if cfg.Tools.WebSearch.Enabled {
		providers := []string{"duckduckgo", "brave", "tavily"}
		defIdx := 0
		for i, p := range providers {
			if p == cfg.Tools.WebSearch.Provider {
				defIdx = i
			}
		}
		steps = append(steps,
			step{kind: stepChoice, label: "Search provider", key: "search_provider", choices: providers, defIdx: defIdx},
		)
		if cfg.Tools.WebSearch.Provider != "duckduckgo" {
			steps = append(steps, step{kind: stepSecret, label: "Search API key", key: "search_key", def: cfg.Tools.WebSearch.APIKey})
		}
		steps = append(steps,
			step{kind: stepInt, label: "Max results", key: "search_max", defInt: orNonZero(cfg.Tools.WebSearch.MaxResults, 5)},
			step{kind: stepYesNo, label: "Fetch full page content for results?", key: "search_fetch", defBool: cfg.Tools.WebSearch.FetchContent},
		)
	}
	return steps
}

func behaviorSteps(cfg *config.Config) []step {
	return []step{
		{kind: stepInt, label: "Max iterations per message", key: "max_iters", defInt: orNonZero(cfg.AgentBehavior.MaxIterations, 30)},
		{kind: stepDuration, label: "Per-tool timeout", key: "tool_timeout", defDur: orNonZeroDur(cfg.AgentBehavior.ToolTimeout, 30*time.Second)},
		{kind: stepInt, label: "Context window (messages retained)", key: "ctx_window", defInt: orNonZero(cfg.AgentBehavior.ContextWindow, 20)},
		{kind: stepText, label: "System prompt file (empty = built-in)", key: "prompt_file", def: cfg.AgentBehavior.SystemPromptFile},
	}
}

func loggingSteps(cfg *config.Config) []step {
	levels := []string{"debug", "info", "warn", "error"}
	defIdx := 0
	for i, l := range levels {
		if l == cfg.Logging.Level {
			defIdx = i
		}
	}
	steps := []step{
		{kind: stepChoice, label: "Log level", key: "log_level", choices: levels, defIdx: defIdx},
		{kind: stepText, label: "Log file (empty = default, \"stderr\" = no file)", key: "log_file", def: cfg.Logging.File},
		{kind: stepYesNo, label: "Enable Prometheus metrics endpoint?", key: "metrics_enabled", defBool: cfg.Metrics.Enabled},
	}
	if cfg.Metrics.Enabled {
		steps = append(steps, step{kind: stepText, label: "Metrics listen address", key: "metrics_addr", def: orDefault(cfg.Metrics.Addr, "127.0.0.1:9101")})
	}
	return steps
}

func channelBadge(enabled bool) string {
	if enabled {
		return "[on]"
	}
	return "[off]"
}

func applyStepResult(cfg *config.Config, s step, value string) {
	switch s.key {
	case "provider":
		prev := cfg.Model.Provider
		cfg.Model.Provider = value
		if cfg.Model.Provider != prev {
			cfg.Model.Model = ""
		}
	case "base_url":
		cfg.Model.BaseURL = value
	case "api_key":
		cfg.Model.APIKey = value
	case "max_tokens":
		if n, err := strconv.Atoi(value); err == nil {
			cfg.Model.MaxTokens = n
		}
	case "temperature":
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			cfg.Model.Temperature = f
		}
	case "tg_enabled":
		cfg.Telegram.Enabled = value == "true"
	case "tg_token":
		cfg.Telegram.BotToken = value
	case "tg_webhook":
		cfg.Telegram.WebhookURL = value
	case "tg_addr":
		cfg.Telegram.WebhookAddr = value
	case "tg_groups":
		cfg.Telegram.RespondInGroups = value == "true"
	case "alga_enabled":
		cfg.Alga.Enabled = value == "true"
	case "alga_url":
		cfg.Alga.ServerURL = value
	case "alga_token":
		cfg.Alga.AgentToken = value
	case "shell_enabled":
		cfg.Tools.Shell.Enabled = value == "true"
	case "shell_cmds":
		parts := strings.Split(value, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		cfg.Tools.Shell.AllowedCommands = out
	case "shell_max":
		if n, err := strconv.Atoi(value); err == nil {
			cfg.Tools.Shell.MaxOutputBytes = n
		}
	case "shell_timeout":
		if d, err := config.ParseDuration(value); err == nil {
			cfg.Tools.Shell.Timeout = d
		}
	case "search_enabled":
		cfg.Tools.WebSearch.Enabled = value == "true"
	case "search_provider":
		cfg.Tools.WebSearch.Provider = value
	case "search_key":
		cfg.Tools.WebSearch.APIKey = value
	case "search_max":
		if n, err := strconv.Atoi(value); err == nil {
			cfg.Tools.WebSearch.MaxResults = n
		}
	case "search_fetch":
		cfg.Tools.WebSearch.FetchContent = value == "true"
	case "max_iters":
		if n, err := strconv.Atoi(value); err == nil {
			cfg.AgentBehavior.MaxIterations = n
		}
	case "tool_timeout":
		if d, err := config.ParseDuration(value); err == nil {
			cfg.AgentBehavior.ToolTimeout = d
		}
	case "ctx_window":
		if n, err := strconv.Atoi(value); err == nil {
			cfg.AgentBehavior.ContextWindow = n
		}
	case "prompt_file":
		cfg.AgentBehavior.SystemPromptFile = value
	case "log_level":
		cfg.Logging.Level = value
	case "log_file":
		cfg.Logging.File = value
	case "metrics_enabled":
		cfg.Metrics.Enabled = value == "true"
	case "metrics_addr":
		cfg.Metrics.Addr = value
	}
}
