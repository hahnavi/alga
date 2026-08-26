package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"

	algacrypto "alga/crypto"
	"alga/logger"
)

// Config holds all configuration for Alga.
type Config struct {
	Port                               string `yaml:"port"`
	LogLevel                           string `yaml:"log_level"`
	LogFile                            string `yaml:"log_file"`
	LogFormat                          string `yaml:"log_format"`
	Environment                        string `yaml:"environment"`
	MattermostURL                      string `yaml:"mattermost_url"`
	MattermostWebhookSecret            string `yaml:"mattermost_webhook_secret"`
	MattermostTeam                     string `yaml:"mattermost_team"`
	MattermostDefaultChannel           string `yaml:"mattermost_default_channel"`
	MattermostDisabled                 bool   `yaml:"mattermost_disabled"`
	SlackBotToken                      string `yaml:"slack_bot_token"`
	SlackDefaultChannel                string `yaml:"slack_default_channel"`
	SlackDisabled                      bool   `yaml:"slack_disabled"`
	SlackClientID                      string `yaml:"slack_client_id"`
	SlackClientSecret                  string `yaml:"slack_client_secret"`
	SlackOAuthRedirectURL              string `yaml:"slack_oauth_redirect_url"`
	SlackWorkspaceName                 string `yaml:"slack_workspace_name"`
	SlackWorkspaceID                   string `yaml:"slack_workspace_id"`
	SlackIncidentChannelsEnabled       bool   `yaml:"slack_incident_channels_enabled"`
	SlackIncidentChannelVisibility     string `yaml:"slack_incident_channel_visibility"`
	SlackIncidentChannelTriggerStatus  string `yaml:"slack_incident_channel_trigger_status"`
	SlackIncidentChannelArchiveOnClose bool   `yaml:"slack_incident_channel_archive_on_close"`
	GoogleMeetEnabled                  bool   `yaml:"google_meet_enabled"`
	GoogleMeetCredentialsPath          string `yaml:"google_meet_credentials_path"`
	GoogleMeetAutoCreate               bool   `yaml:"google_meet_auto_create"`
	AlgaBaseURL                        string `yaml:"alga_base_url"`
	PostgresDSN                        string `yaml:"postgres_dsn"`
	PostgresAutoMigrate                bool   `yaml:"postgres_auto_migrate"`
	// PostgreSQL connection pool tuning. Zero/negative values fall back to
	// db.DefaultPoolConfig() at connection time.
	PostgresMaxOpenConns    int           `yaml:"postgres_max_open_conns"`
	PostgresMaxIdleConns    int           `yaml:"postgres_max_idle_conns"`
	PostgresConnMaxIdleTime time.Duration `yaml:"postgres_conn_max_idle_time"`
	PostgresConnMaxLifetime time.Duration `yaml:"postgres_conn_max_lifetime"`
	ConfigPath              string        `yaml:"-"`
	SessionExpiryHrs        int           `yaml:"session_expiry_hours"`
	// SessionMaxLifetime is the absolute (max) session lifetime regardless of
	// activity. Enforced in addition to the sliding idle expiry
	// (SessionExpiryHrs): a session is invalid once now > CreatedAt +
	// SessionMaxLifetime even if it was refreshed recently (ASVS V3.2/V3.3).
	SessionMaxLifetime time.Duration `yaml:"session_max_lifetime"`
	SecureCookies      bool          `yaml:"secure_cookies"`
	// ServerReadHeaderTimeout bounds the time spent reading request headers,
	// neutralizing slowloris-style attacks. Must be the smallest server timeout.
	ServerReadHeaderTimeout time.Duration `yaml:"server_read_header_timeout"`
	// ServerReadTimeout bounds the time spent reading the entire request
	// (headers + body). Must exceed the longest legitimate request body.
	ServerReadTimeout time.Duration `yaml:"server_read_timeout"`
	// ServerWriteTimeout bounds the time spent writing the response. It applies
	// to the whole request lifecycle including streaming; long-lived handlers
	// (SSE) clear their own write deadline per-request via DisableWriteDeadline.
	ServerWriteTimeout time.Duration `yaml:"server_write_timeout"`
	// ServerIdleTimeout bounds how long an idle keep-alive connection is held.
	ServerIdleTimeout time.Duration `yaml:"server_idle_timeout"`
	// ServerMaxHeaderBytes bounds the total request-header size in bytes.
	ServerMaxHeaderBytes int      `yaml:"server_max_header_bytes"`
	ValkeyAddr           string   `yaml:"valkey_addr"`
	ValkeyPassword       string   `yaml:"valkey_password"`
	ValkeyDB             int      `yaml:"valkey_db"`
	RabbitMQURI          string   `yaml:"rabbitmq_uri"`
	TrustedProxies       []string `yaml:"trusted_proxies"`

	CorrelationWindow      time.Duration `yaml:"correlation_window"`
	CorrelationCooldownTTL time.Duration `yaml:"correlation_cooldown_ttl"`
	// SchedulerLeaderTTL is the lease duration used by the InvestigationScheduler
	// for HA leader election. Renewed on each scheduler tick.
	SchedulerLeaderTTL time.Duration `yaml:"scheduler_leader_ttl"`
	// AgentPresenceTTL is how long a Valkey-tracked SSE session
	// survives without a renewal (renewed via heartbeat or SSE keepalive).
	AgentPresenceTTL time.Duration `yaml:"agent_presence_ttl"`
	// CancelSetTTL is how long a deleted entity's cancel marker lives in
	// Valkey so queue workers/schedulers skip work for it without hitting
	// Postgres. Must exceed the max RabbitMQ retry/DLQ horizon.
	CancelSetTTL time.Duration `yaml:"cancel_set_ttl"`
	// IdempotencyEnabled turns on Idempotency-Key replay for the opted-in
	// retry-safe write endpoints (create incident, ingest alert webhook,
	// notification send, agent state-changing actions). Requires Valkey; when
	// Valkey is not configured the wrapper is a no-op regardless of this flag.
	IdempotencyEnabled bool `yaml:"idempotency_enabled"`
	// IdempotencyTTL is how long a cached response is replayed for a repeated
	// Idempotency-Key before the key expires. Keep it short: it only needs to
	// span a client's retry window.
	IdempotencyTTL time.Duration `yaml:"idempotency_ttl"`
	// AgentDisconnectGrace is how long the scheduler waits after losing an
	// SSE connection before resetting investigations the agent had picked up.
	AgentDisconnectGrace        time.Duration `yaml:"agent_disconnect_grace"`
	InvestigationTimeout        time.Duration `yaml:"investigation_timeout"`
	MaxConcurrentInvestigations int           `yaml:"max_concurrent_investigations"`
	MaxConcurrentTriage         int           `yaml:"max_concurrent_triage"`
	MaxConcurrentCommand        int           `yaml:"max_concurrent_command"`
	TriageTimeout               time.Duration `yaml:"triage_timeout"`

	// IncidentSummaryEnabled enables periodic incident summary postings.
	IncidentSummaryEnabled bool `yaml:"incident_summary_enabled"`
	// IncidentSummaryInterval is the default cadence for incident summaries.
	IncidentSummaryInterval time.Duration `yaml:"incident_summary_interval"`
	// IncidentSummaryIntervals overrides the cadence per incident severity.
	IncidentSummaryIntervals map[string]time.Duration `yaml:"incident_summary_intervals"`
	StatusUpdateInterval     time.Duration            `yaml:"status_update_interval"`
	TwilioAccountSID         string                   `yaml:"twilio_account_sid"`
	TwilioAuthToken          string                   `yaml:"twilio_auth_token"`
	TwilioFromNumber         string                   `yaml:"twilio_from_number"`
	TwilioToNumbers          []string                 `yaml:"twilio_to_numbers"`
	TwilioDisabled           bool                     `yaml:"twilio_disabled"`
	VoiceProvider            string                   `yaml:"voice_provider"` // empty means "twilio"
	TelnyxAPIKey             string                   `yaml:"telnyx_api_key"`
	TelnyxConnectionID       string                   `yaml:"telnyx_connection_id"`
	TelnyxFromNumber         string                   `yaml:"telnyx_from_number"`
	TelnyxPublicKey          string                   `yaml:"telnyx_public_key"`
	TelnyxDisabled           bool                     `yaml:"telnyx_disabled"`
	TelnyxTTSVoice           string                   `yaml:"telnyx_tts_voice"`
	TelnyxTTSLanguage        string                   `yaml:"telnyx_tts_language"`
	TelnyxTTSAPIKeyRef       string                   `yaml:"telnyx_tts_api_key_ref"`
	InvestigationChannel     string                   `yaml:"investigation_channel"`
	CriticalSeverityLabels   []string                 `yaml:"critical_severity_labels"`

	HermesPlatformURL   string `yaml:"hermes_platform_url"`
	HermesPlatformToken string `yaml:"hermes_platform_token"`

	// SlackSigningSecret verifies Slack Events API requests (optional).
	SlackSigningSecret     string `yaml:"slack_signing_secret"`
	GoogleClientID         string `yaml:"google_client_id"`
	GoogleClientSecret     string `yaml:"google_client_secret"`
	GoogleOAuthRedirectURL string `yaml:"google_oauth_redirect_url"`
	GoogleOAuthEnabled     bool   `yaml:"google_oauth_enabled"`

	// AgentSSEAllowedOrigins is a comma-separated list of allowed Origin headers for the agent SSE endpoint; empty allows all.
	AgentSSEAllowedOrigins string `yaml:"agent_sse_allowed_origins"`

	// WebhookAllowQueryToken re-enables the legacy `?token=` query-parameter
	// authentication on POST /webhooks/alerts. Credentials in URLs leak via
	// proxy/access logs, Referer headers, and browser history, so this defaults
	// to false and only header authentication is accepted. Removal of the flag
	// (and the fallback) is planned one minor release after introduction.
	WebhookAllowQueryToken bool `yaml:"webhook_allow_query_token"`
	// AgentSSEAllowQueryToken re-enables the legacy `?token=` fallback on the
	// agent SSE endpoint for pure-EventSource consumers that cannot set an
	// Authorization header. Defaults to false; fetch()-based SSE with a Bearer
	// header is the blessed pattern.
	AgentSSEAllowQueryToken bool `yaml:"agent_sse_allow_query_token"`

	// RateLimitGeneralPerMinute caps public-surface requests per IP per
	// minute (webhooks, discovery, callbacks). Both limiter backends (memory
	// and Valkey) share this ceiling; default 20 matches the historical
	// Valkey contract.
	RateLimitGeneralPerMinute int `yaml:"rate_limit_general_per_minute"`
	// RateLimitAgentPerMinute caps agent-bearer requests per token per
	// minute across both limiter backends. Default 120.
	RateLimitAgentPerMinute int `yaml:"rate_limit_agent_per_minute"`

	// StaleAlertThreshold is the minimum age a firing alert must reach before
	// the scheduler considers it "stale" (no investigation). Alerts younger
	// than this are assumed to still be in the correlator pipeline.
	StaleAlertThreshold time.Duration `yaml:"stale_alert_threshold"`
	// StaleAlertSweepInterval is how often the scheduler sweeps for stale
	// alerts and publishes investigation jobs for them.
	StaleAlertSweepInterval time.Duration `yaml:"stale_alert_sweep_interval"`

	// StuckInvestigationEscalationMultiplier is the multiple of
	// InvestigationTimeout after which an in-progress alert investigation is
	// considered "stuck" and triggers an ops-team page. Default 2 (i.e. 2x the
	// investigation timeout). Set to 0 to disable the stuck-investigation
	// escalation path entirely.
	StuckInvestigationEscalationMultiplier int `yaml:"stuck_investigation_escalation_multiplier"`
	// StuckInvestigationEscalationTickInterval is how often the stuck
	// investigation escalation worker runs.
	StuckInvestigationEscalationTickInterval time.Duration `yaml:"stuck_investigation_escalation_tick_interval"`
	// OpsTeamName is the name of the team whose schedule is treated as the
	// human-on-call target for stuck-investigation and forced-channel
	// escalations. Defaults to "ops-team".
	OpsTeamName string `yaml:"ops_team_name"`

	DataRetentionDays int `yaml:"data_retention_days"`

	// MemoryEnabled enables the shared agent memory system.
	MemoryEnabled bool `yaml:"memory_enabled"`
	// MemoryEmbeddingURL is the OpenAI-compatible embedding API URL.
	MemoryEmbeddingURL string `yaml:"memory_embedding_url"`
	// MemoryEmbeddingAPIKey is the API key for the embedding service.
	MemoryEmbeddingAPIKey string `yaml:"memory_embedding_api_key"`
	// MemoryEmbeddingModel is the embedding model name.
	MemoryEmbeddingModel string `yaml:"memory_embedding_model"`
	// MemoryLLMURL is the OpenAI-compatible LLM API URL for memory extraction.
	MemoryLLMURL string `yaml:"memory_llm_url"`
	// MemoryLLMAPIKey is the API key for the extraction LLM.
	MemoryLLMAPIKey string `yaml:"memory_llm_api_key"`
	// MemoryLLMModel is the LLM model for memory extraction.
	MemoryLLMModel string `yaml:"memory_llm_model"`
	// MemoryAutoExtract enables automatic memory extraction on investigation completion.
	MemoryAutoExtract bool `yaml:"memory_auto_extract"`
	// MemoryMaxPerInvestigation caps the number of memories extracted per investigation.
	MemoryMaxPerInvestigation int `yaml:"memory_max_per_investigation"`
	// MemorySimilarityThreshold is the minimum cosine similarity for search results.
	MemorySimilarityThreshold float64 `yaml:"memory_similarity_threshold"`

	SMTPHost          string `yaml:"smtp_host"`
	SMTPPort          int    `yaml:"smtp_port"`
	SMTPUser          string `yaml:"smtp_user"`
	SMTPPassword      string `yaml:"smtp_password"`
	SMTPFrom          string `yaml:"smtp_from"`
	SMTPSkipTLSVerify bool   `yaml:"smtp_skip_tls_verify"`

	TriageEnabled                   bool    `yaml:"triage_enabled"`
	TriageLLMURL                    string  `yaml:"triage_llm_url"`
	TriageLLMAPIKey                 string  `yaml:"triage_llm_api_key"`
	TriageLLMModel                  string  `yaml:"triage_llm_model"`
	TriageMaxConcurrent             int     `yaml:"triage_max_concurrent"`
	TriageConfidenceThreshold       float64 `yaml:"triage_confidence_threshold"`
	TriageAutoResolveEnabled        bool    `yaml:"triage_auto_resolve_enabled"`
	TriageSuppressEnabled           bool    `yaml:"triage_suppress_enabled"`
	TriageContextEpisodicLimit      int     `yaml:"triage_context_episodic_limit"`
	TriageContextNotesLimit         int     `yaml:"triage_context_notes_limit"`
	TriageContextMemoriesLimit      int     `yaml:"triage_context_memories_limit"`
	TriageAutoPromoteConfirmedCount int     `yaml:"triage_auto_promote_confirmed_count"`

	// OTELTracingEnabled is Alga's master switch for OpenTelemetry tracing
	// (env: ALGA_OTEL_ENABLED). Tracing also turns on when an OTLP endpoint is
	// configured. When neither is set, a noop tracer provider is installed and
	// tracing has zero runtime cost.
	OTELTracingEnabled bool `yaml:"otel_tracing_enabled"`
	// OTELExporterOTLPEndpoint is the OTLP/HTTP collector endpoint used when the
	// standard OTEL_EXPORTER_OTLP_ENDPOINT / OTEL_EXPORTER_OTLP_TRACES_ENDPOINT
	// env vars are not set (env: OTEL_EXPORTER_OTLP_ENDPOINT).
	OTELExporterOTLPEndpoint string `yaml:"otel_exporter_otlp_endpoint"`
	// OTELSampleRatio is the fraction (0.0..1.0) of new traces sampled via a
	// ParentBased(TraceIDRatioBased) sampler (env: ALGA_OTEL_SAMPLE_RATIO).
	// Defaults to 1.0 when tracing is enabled.
	OTELSampleRatio float64 `yaml:"otel_sample_ratio"`
	// OutboxRetention is how long a successfully-published outbox row is kept
	// before the outbox worker prunes it (env: ALGA_OUTBOX_RETENTION). Keeping
	// published rows for a window lets operators audit recent event delivery;
	// the retention sweep bounds table growth. Set to 0 to disable pruning.
	OutboxRetention time.Duration `yaml:"outbox_retention"`
}

// RouteTarget is a single notification destination (integration + channel).
type RouteTarget struct {
	Provider string `yaml:"provider,omitempty" json:"provider,omitempty"`
	Channel  string `yaml:"channel" json:"channel"`
}

// RouteConfig defines a routing rule
type RouteConfig struct {
	MatchMode  string           `yaml:"match_mode,omitempty" json:"match_mode,omitempty"`
	Conditions []RouteCondition `yaml:"conditions,omitempty" json:"conditions,omitempty"`
	Targets    []RouteTarget    `yaml:"targets,omitempty" json:"targets,omitempty"`
	Silenced   bool             `yaml:"silenced,omitempty" json:"silenced,omitempty"`
}

// RouteCondition defines a single alert rule matcher.
type RouteCondition struct {
	Source   string `yaml:"source" json:"source"`
	Field    string `yaml:"field" json:"field"`
	Operator string `yaml:"operator" json:"operator"`
	Value    string `yaml:"value,omitempty" json:"value,omitempty"`
}

// Defaults returns a Config populated with the built-in safe defaults. It is
// the single source of truth for default values so Load() and tests agree.
func Defaults() *Config {
	return &Config{
		Port:                "8080",
		LogLevel:            "info",
		ConfigPath:          "./config/config.yaml",
		PostgresAutoMigrate: false,
		// PostgreSQL pool defaults mirror db.DefaultPoolConfig().
		PostgresMaxOpenConns:    25,
		PostgresMaxIdleConns:    5,
		PostgresConnMaxIdleTime: 5 * time.Minute,
		PostgresConnMaxLifetime: 30 * time.Minute,
		SessionExpiryHrs:        24,
		// Absolute (max) session lifetime (ASVS V3.2/V3.3). A session is
		// rejected once it is older than this even if the sliding idle expiry
		// was just refreshed, bounding a stolen cookie to one max window.
		SessionMaxLifetime: 12 * time.Hour,
		// HTTP server hardening defaults (V12.1). ReadHeaderTimeout alone
		// neutralizes slowloris; the body/total timeouts cap slow-body
		// attacks; MaxHeaderBytes bounds header memory. SSE handlers clear
		// their own write deadline via DisableWriteDeadline so the 30s value
		// does not truncate long-lived streams.
		ServerReadHeaderTimeout:                  10 * time.Second,
		ServerReadTimeout:                        30 * time.Second,
		ServerWriteTimeout:                       30 * time.Second,
		ServerIdleTimeout:                        120 * time.Second,
		ServerMaxHeaderBytes:                     1 << 20, // 1 MiB
		CorrelationWindow:                        0,
		CorrelationCooldownTTL:                   30 * time.Minute,
		SchedulerLeaderTTL:                       15 * time.Second,
		AgentPresenceTTL:                         90 * time.Second,
		CancelSetTTL:                             7 * 24 * time.Hour,
		IdempotencyEnabled:                       true,
		IdempotencyTTL:                           24 * time.Hour,
		AgentDisconnectGrace:                     45 * time.Second,
		InvestigationTimeout:                     10 * time.Minute,
		MaxConcurrentInvestigations:              3,
		MaxConcurrentTriage:                      5,
		MaxConcurrentCommand:                     3,
		TriageTimeout:                            2 * time.Minute,
		StatusUpdateInterval:                     15 * time.Minute,
		StaleAlertThreshold:                      15 * time.Minute,
		StaleAlertSweepInterval:                  5 * time.Minute,
		StuckInvestigationEscalationMultiplier:   2,
		StuckInvestigationEscalationTickInterval: 30 * time.Second,
		OpsTeamName:                              "ops-team",
		DataRetentionDays:                        90,
		SMTPPort:                                 587,
		TriageMaxConcurrent:                      3,
		TriageConfidenceThreshold:                0.7,
		TriageContextEpisodicLimit:               3,
		TriageContextNotesLimit:                  3,
		TriageContextMemoriesLimit:               5,
		TriageAutoPromoteConfirmedCount:          3,
		SlackIncidentChannelVisibility:           "private",
		SlackIncidentChannelTriggerStatus:        "active",
		SlackIncidentChannelArchiveOnClose:       true,
		GoogleOAuthEnabled:                       true,
		// Outbox retention: keep published outbox rows for 7 days so operators
		// can audit recent event delivery, then the outbox worker prunes them.
		OutboxRetention: 7 * 24 * time.Hour,
	}
}

// envBool reads an environment variable as a bool. On parse failure it falls
// back to fallback and logs a warning so a typo doesn't silently mask the
// intended value.
func envBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		logger.Warn("invalid bool env var; falling back to default", "key", key, "value", v, "default", fallback, "error", err)
		return fallback
	}
	return b
}

// envInt reads an environment variable as an int with the same fallback+warn
// semantics as envBool.
func envInt(key string, fallback int) int {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		logger.Warn("invalid int env var; falling back to default", "key", key, "value", v, "default", fallback, "error", err)
		return fallback
	}
	return n
}

// envFloat reads an environment variable as a float64 with the same
// fallback+warn semantics as envBool.
func envFloat(key string, fallback float64) float64 {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		logger.Warn("invalid float env var; falling back to default", "key", key, "value", v, "default", fallback, "error", err)
		return fallback
	}
	return f
}

// Load reads configuration from file and environment variables
func Load() (*Config, error) {
	loadDotEnv()

	// Set defaults
	cfg := Defaults()

	if v := os.Getenv("CONFIG_PATH"); v != "" {
		cfg.ConfigPath = v
	}

	// Load from YAML file (file values are applied first)
	data, err := os.ReadFile(cfg.ConfigPath)
	if err != nil {
		// If file doesn't exist, that's okay - we'll use defaults and env vars
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	// Override from environment variables (env vars always take final precedence)
	if v := os.Getenv("PORT"); v != "" {
		cfg.Port = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("LOG_FILE"); v != "" {
		cfg.LogFile = v
	}
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		cfg.LogFormat = v
	}
	if v := os.Getenv("ENVIRONMENT"); v != "" {
		cfg.Environment = v
	}
	// Default SecureCookies to true in production
	if (cfg.Environment == "production" || cfg.Environment == "prod") && !cfg.SecureCookies {
		// Only override if not explicitly set
		if os.Getenv("SECURE_COOKIES") == "" {
			cfg.SecureCookies = true
		}
	}
	if v := os.Getenv("MATTERMOST_SERVER_URL"); v != "" {
		cfg.MattermostURL = v
	}
	if v := os.Getenv("MATTERMOST_WEBHOOK_SECRET"); v != "" {
		cfg.MattermostWebhookSecret = v
	}
	if v := os.Getenv("MATTERMOST_TEAM"); v != "" {
		cfg.MattermostTeam = v
	}
	if v := os.Getenv("MATTERMOST_DEFAULT_CHANNEL"); v != "" {
		cfg.MattermostDefaultChannel = v
	}
	if v := os.Getenv("MATTERMOST_DISABLED"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.MattermostDisabled = parsed
		}
	}
	if v := os.Getenv("POSTGRES_DSN"); v != "" {
		cfg.PostgresDSN = v
	}
	if v := os.Getenv("POSTGRES_AUTO_MIGRATE"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err == nil {
			cfg.PostgresAutoMigrate = parsed
		}
	}
	if v := os.Getenv("POSTGRES_MAX_OPEN_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.PostgresMaxOpenConns = n
		}
	}
	if v := os.Getenv("POSTGRES_MAX_IDLE_CONNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.PostgresMaxIdleConns = n
		}
	}
	if v := os.Getenv("POSTGRES_CONN_MAX_IDLE_TIME"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.PostgresConnMaxIdleTime = d
		}
	}
	if v := os.Getenv("POSTGRES_CONN_MAX_LIFETIME"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.PostgresConnMaxLifetime = d
		}
	}
	if v := os.Getenv("SLACK_BOT_TOKEN"); v != "" {
		cfg.SlackBotToken = v
	}
	if v := os.Getenv("SLACK_DEFAULT_CHANNEL"); v != "" {
		cfg.SlackDefaultChannel = v
	}
	if v := os.Getenv("SLACK_DISABLED"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err == nil {
			cfg.SlackDisabled = parsed
		}
	}
	if v := os.Getenv("TRUSTED_PROXIES"); v != "" {
		cfg.TrustedProxies = strings.Split(v, ",")
	}
	if v := os.Getenv("SESSION_EXPIRY_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.SessionExpiryHrs = n
		}
	}
	if v := os.Getenv("SESSION_MAX_LIFETIME"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.SessionMaxLifetime = d
		}
	}
	if v := os.Getenv("SECURE_COOKIES"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err == nil {
			cfg.SecureCookies = parsed
		}
	}

	// HTTP server hardening overrides (V12.1). Zero/invalid keeps defaults.
	if v := os.Getenv("SERVER_READ_HEADER_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.ServerReadHeaderTimeout = d
		}
	}
	if v := os.Getenv("SERVER_READ_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.ServerReadTimeout = d
		}
	}
	if v := os.Getenv("SERVER_WRITE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.ServerWriteTimeout = d
		}
	}
	if v := os.Getenv("SERVER_IDLE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.ServerIdleTimeout = d
		}
	}
	if v := os.Getenv("SERVER_MAX_HEADER_BYTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.ServerMaxHeaderBytes = n
		}
	}

	if v := os.Getenv("VALKEY_ADDR"); v != "" {
		cfg.ValkeyAddr = v
	}
	if v := os.Getenv("VALKEY_PASSWORD"); v != "" {
		cfg.ValkeyPassword = v
	}
	if v := os.Getenv("VALKEY_DB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.ValkeyDB = n
		}
	}
	if v := os.Getenv("RABBITMQ_URI"); v != "" {
		cfg.RabbitMQURI = v
	}

	if v := os.Getenv("CORRELATION_WINDOW"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.CorrelationWindow = d
		}
	}
	if v := os.Getenv("CORRELATION_COOLDOWN_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.CorrelationCooldownTTL = d
		}
	}
	if v := os.Getenv("SCHEDULER_LEADER_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.SchedulerLeaderTTL = d
		}
	}
	if v := os.Getenv("AGENT_PRESENCE_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.AgentPresenceTTL = d
		}
	}
	if v := os.Getenv("CANCEL_SET_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.CancelSetTTL = d
		}
	}
	if v := os.Getenv("IDEMPOTENCY_ENABLED"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.IdempotencyEnabled = parsed
		}
	}
	if v := os.Getenv("IDEMPOTENCY_TTL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.IdempotencyTTL = d
		}
	}
	if v := os.Getenv("AGENT_DISCONNECT_GRACE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.AgentDisconnectGrace = d
		}
	}
	if v := os.Getenv("INVESTIGATION_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.InvestigationTimeout = d
		}
	}
	if v := os.Getenv("MAX_CONCURRENT_INVESTIGATIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxConcurrentInvestigations = n
		}
	}
	if v := os.Getenv("MAX_CONCURRENT_TRIAGE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxConcurrentTriage = n
		}
	}
	if v := os.Getenv("MAX_CONCURRENT_COMMAND"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MaxConcurrentCommand = n
		}
	}
	if v := os.Getenv("TRIAGE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.TriageTimeout = d
		}
	}
	if v := os.Getenv("STATUS_UPDATE_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.StatusUpdateInterval = d
		}
	}
	if v := os.Getenv("TWILIO_ACCOUNT_SID"); v != "" {
		cfg.TwilioAccountSID = v
	}
	if v := os.Getenv("TWILIO_AUTH_TOKEN"); v != "" {
		cfg.TwilioAuthToken = v
	}
	if v := os.Getenv("TWILIO_FROM_NUMBER"); v != "" {
		cfg.TwilioFromNumber = v
	}
	if v := os.Getenv("TWILIO_TO_NUMBERS"); v != "" {
		cfg.TwilioToNumbers = strings.Split(v, ",")
	}
	if v := os.Getenv("TWILIO_DISABLED"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.TwilioDisabled = parsed
		}
	}
	if v := os.Getenv("VOICE_PROVIDER"); v != "" {
		cfg.VoiceProvider = v
	}
	cfg.VoiceProvider = NormalizeVoiceProvider(cfg.VoiceProvider)
	if v := os.Getenv("TELNYX_API_KEY"); v != "" {
		cfg.TelnyxAPIKey = v
	}
	if v := os.Getenv("TELNYX_CONNECTION_ID"); v != "" {
		cfg.TelnyxConnectionID = v
	}
	if v := os.Getenv("TELNYX_FROM_NUMBER"); v != "" {
		cfg.TelnyxFromNumber = v
	}
	if v := os.Getenv("TELNYX_PUBLIC_KEY"); v != "" {
		cfg.TelnyxPublicKey = v
	}
	if v := os.Getenv("TELNYX_DISABLED"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.TelnyxDisabled = parsed
		}
	}
	if v := os.Getenv("TELNYX_TTS_VOICE"); v != "" {
		cfg.TelnyxTTSVoice = v
	}
	if v := os.Getenv("TELNYX_TTS_LANGUAGE"); v != "" {
		cfg.TelnyxTTSLanguage = v
	}
	if v := os.Getenv("TELNYX_TTS_API_KEY_REF"); v != "" {
		cfg.TelnyxTTSAPIKeyRef = v
	}
	if v := os.Getenv("INVESTIGATION_CHANNEL"); v != "" {
		cfg.InvestigationChannel = v
	}
	if v := os.Getenv("CRITICAL_SEVERITY_LABELS"); v != "" {
		cfg.CriticalSeverityLabels = strings.Split(v, ",")
	}
	if v := os.Getenv("HERMES_PLATFORM_URL"); v != "" {
		cfg.HermesPlatformURL = v
	}
	if v := os.Getenv("HERMES_PLATFORM_TOKEN"); v != "" {
		cfg.HermesPlatformToken = v
	}
	if v := os.Getenv("SLACK_SIGNING_SECRET"); v != "" {
		cfg.SlackSigningSecret = v
	}
	if v := os.Getenv("SLACK_CLIENT_ID"); v != "" {
		cfg.SlackClientID = v
	}
	if v := os.Getenv("SLACK_CLIENT_SECRET"); v != "" {
		cfg.SlackClientSecret = v
	}
	if v := os.Getenv("SLACK_OAUTH_REDIRECT_URL"); v != "" {
		cfg.SlackOAuthRedirectURL = v
	}
	if v := os.Getenv("ALGA_BASE_URL"); v != "" {
		cfg.AlgaBaseURL = v
	}
	if v := os.Getenv("GOOGLE_CLIENT_ID"); v != "" {
		cfg.GoogleClientID = v
	}
	if v := os.Getenv("GOOGLE_CLIENT_SECRET"); v != "" {
		cfg.GoogleClientSecret = v
	}
	if v := os.Getenv("GOOGLE_OAUTH_REDIRECT_URL"); v != "" {
		cfg.GoogleOAuthRedirectURL = v
	}
	if v := os.Getenv("GOOGLE_OAUTH_ENABLED"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.GoogleOAuthEnabled = parsed
		}
	}
	if v := os.Getenv("GOOGLE_MEET_ENABLED"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.GoogleMeetEnabled = parsed
		}
	}
	if v := os.Getenv("GOOGLE_MEET_CREDENTIALS_PATH"); v != "" {
		cfg.GoogleMeetCredentialsPath = v
	}
	if v := os.Getenv("GOOGLE_MEET_AUTO_CREATE"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.GoogleMeetAutoCreate = parsed
		}
	}
	if v := os.Getenv("AGENT_SSE_ALLOWED_ORIGINS"); v != "" {
		cfg.AgentSSEAllowedOrigins = v
	}
	if v := os.Getenv("WEBHOOK_ALLOW_QUERY_TOKEN"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.WebhookAllowQueryToken = parsed
		}
	}
	if v := os.Getenv("AGENT_SSE_ALLOW_QUERY_TOKEN"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.AgentSSEAllowQueryToken = parsed
		}
	}
	if cfg.RateLimitGeneralPerMinute == 0 {
		cfg.RateLimitGeneralPerMinute = 20
	}
	if cfg.RateLimitAgentPerMinute == 0 {
		cfg.RateLimitAgentPerMinute = 120
	}
	if v := os.Getenv("RATE_LIMIT_GENERAL_PER_MINUTE"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			cfg.RateLimitGeneralPerMinute = parsed
		}
	}
	if v := os.Getenv("RATE_LIMIT_AGENT_PER_MINUTE"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			cfg.RateLimitAgentPerMinute = parsed
		}
	}
	if v := os.Getenv("STALE_ALERT_THRESHOLD"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.StaleAlertThreshold = d
		}
	}
	if v := os.Getenv("STALE_ALERT_SWEEP_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.StaleAlertSweepInterval = d
		}
	}
	if v := os.Getenv("STUCK_INVESTIGATION_ESCALATION_MULTIPLIER"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.StuckInvestigationEscalationMultiplier = n
		}
	}
	if v := os.Getenv("STUCK_INVESTIGATION_ESCALATION_TICK_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.StuckInvestigationEscalationTickInterval = d
		}
	}
	if v := os.Getenv("OPS_TEAM_NAME"); v != "" {
		cfg.OpsTeamName = v
	}

	if v := os.Getenv("DATA_RETENTION_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.DataRetentionDays = n
		}
	}

	if v := os.Getenv("MEMORY_ENABLED"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.MemoryEnabled = parsed
		}
	}
	if v := os.Getenv("MEMORY_EMBEDDING_URL"); v != "" {
		cfg.MemoryEmbeddingURL = v
	}
	if v := os.Getenv("MEMORY_EMBEDDING_API_KEY"); v != "" {
		cfg.MemoryEmbeddingAPIKey = v
	}
	if v := os.Getenv("MEMORY_EMBEDDING_MODEL"); v != "" {
		cfg.MemoryEmbeddingModel = v
	}
	if v := os.Getenv("MEMORY_LLM_URL"); v != "" {
		cfg.MemoryLLMURL = v
	}
	if v := os.Getenv("MEMORY_LLM_API_KEY"); v != "" {
		cfg.MemoryLLMAPIKey = v
	}
	if v := os.Getenv("MEMORY_LLM_MODEL"); v != "" {
		cfg.MemoryLLMModel = v
	}
	if v := os.Getenv("MEMORY_AUTO_EXTRACT"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.MemoryAutoExtract = parsed
		}
	}
	if v := os.Getenv("MEMORY_MAX_PER_INVESTIGATION"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.MemoryMaxPerInvestigation = n
		}
	}
	if v := os.Getenv("MEMORY_SIMILARITY_THRESHOLD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 && f <= 1 {
			cfg.MemorySimilarityThreshold = f
		}
	}

	if v := os.Getenv("SMTP_HOST"); v != "" {
		cfg.SMTPHost = v
	}
	if v := os.Getenv("SMTP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.SMTPPort = n
		}
	}
	if v := os.Getenv("SMTP_USER"); v != "" {
		cfg.SMTPUser = v
	}
	if v := os.Getenv("SMTP_PASSWORD"); v != "" {
		cfg.SMTPPassword = v
	}
	if v := os.Getenv("SMTP_FROM"); v != "" {
		cfg.SMTPFrom = v
	}
	if v := os.Getenv("SMTP_SKIP_TLS_VERIFY"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.SMTPSkipTLSVerify = parsed
		}
	}

	cfg.TriageEnabled = envBool("TRIAGE_ENABLED", cfg.TriageEnabled)
	if v := os.Getenv("TRIAGE_LLM_URL"); v != "" {
		cfg.TriageLLMURL = v
	}
	if v := os.Getenv("TRIAGE_LLM_API_KEY"); v != "" {
		cfg.TriageLLMAPIKey = v
	}
	if v := os.Getenv("TRIAGE_LLM_MODEL"); v != "" {
		cfg.TriageLLMModel = v
	}
	cfg.TriageMaxConcurrent = envInt("TRIAGE_MAX_CONCURRENT", cfg.TriageMaxConcurrent)
	cfg.TriageConfidenceThreshold = envFloat("TRIAGE_CONFIDENCE_THRESHOLD", cfg.TriageConfidenceThreshold)
	cfg.TriageAutoResolveEnabled = envBool("TRIAGE_AUTO_RESOLVE_ENABLED", cfg.TriageAutoResolveEnabled)
	cfg.TriageSuppressEnabled = envBool("TRIAGE_SUPPRESS_ENABLED", cfg.TriageSuppressEnabled)
	cfg.TriageContextEpisodicLimit = envInt("TRIAGE_CONTEXT_EPISODIC_LIMIT", cfg.TriageContextEpisodicLimit)
	cfg.TriageContextNotesLimit = envInt("TRIAGE_CONTEXT_NOTES_LIMIT", cfg.TriageContextNotesLimit)
	cfg.TriageContextMemoriesLimit = envInt("TRIAGE_CONTEXT_MEMORIES_LIMIT", cfg.TriageContextMemoriesLimit)
	cfg.TriageAutoPromoteConfirmedCount = envInt("TRIAGE_AUTO_PROMOTE_CONFIRMED_COUNT", cfg.TriageAutoPromoteConfirmedCount)

	if v := os.Getenv("ALGA_OTEL_ENABLED"); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.OTELTracingEnabled = parsed
		}
	}
	if v := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); v != "" {
		cfg.OTELExporterOTLPEndpoint = v
	}
	if v := os.Getenv("ALGA_OTEL_SAMPLE_RATIO"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 1 {
			cfg.OTELSampleRatio = f
		}
	}
	if v := os.Getenv("ALGA_OUTBOX_RETENTION"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.OutboxRetention = d
		}
	}

	return cfg, nil
}

// Validate checks critical security settings before starting the server.
//
// Crypto enforcement is unconditional (ASVS V2.1/V6.2, SPEC gap M4): if
// ENCRYPTION_KEYS or SECRET_PEPPER is absent the server refuses to start in
// ANY environment, not just production. Previously this was gated on
// ENVIRONMENT=production, which meant a missing/misspelled ENVIRONMENT var
// silently disabled crypto enforcement.
//
// The production-only extra (warn when SecureCookies is false) remains scoped
// to ENVIRONMENT=production|prod. The admin account is created via the setup
// wizard on first boot instead of a pre-configured email and password.
func (c *Config) Validate() error {
	env := c.Environment
	if env == "" {
		env = os.Getenv("ENVIRONMENT")
	}
	prod := env == "production" || env == "prod"

	if prod && !c.SecureCookies {
		logger.Warn("SECURE_COOKIES is false in production; cookies will be sent over HTTP", "component", "config")
	}

	// SMTP TLS verification is off by default (false). Warn loudly in every
	// environment when an operator explicitly disables it, since it allows
	// MITM of outgoing email (ASVS V9.2, SPEC gap L6).
	if c.SMTPSkipTLSVerify {
		logger.Warn("SMTP_SKIP_TLS_VERIFY is true; SMTP TLS certificate verification is disabled (MITM risk for outgoing email)", "component", "config")
	}

	// Legacy query-token auth escape hatches must be loud: they weaken the
	// default credential-handling posture and are slated for removal.
	if c.RateLimitGeneralPerMinute > 10*20 {
		logger.Warn("RATE_LIMIT_GENERAL_PER_MINUTE is more than 10x the default of 20; verify this is intentional for your webhook volume", "component", "config", "value", c.RateLimitGeneralPerMinute)
	}

	if c.WebhookAllowQueryToken {
		logger.Warn("WEBHOOK_ALLOW_QUERY_TOKEN is true; webhook tokens are accepted via ?token= query parameter (logged in proxy/history trails). Migrate senders to Authorization: Bearer before the flag is removed", "component", "config")
	}
	if c.AgentSSEAllowQueryToken {
		logger.Warn("AGENT_SSE_ALLOW_QUERY_TOKEN is true; agent SSE accepts ?token= query parameter (logged in proxy/access logs). Prefer fetch()-based SSE with Authorization header", "component", "config")
	}

	// Force the lazy keyring init now so we can surface a usable error here
	// rather than at the first encrypt/HMAC call somewhere deep in a handler.
	keyring := algacrypto.Default()
	if err := algacrypto.DefaultErr(); err != nil {
		return fmt.Errorf("crypto keyring init: %w", err)
	}

	// Unconditional fail-closed: crypto config is required in every environment,
	// independent of the ENVIRONMENT value (ASVS V2.1/V6.2).
	if err := keyring.Validate(); err != nil {
		return err
	}

	return nil
}

func loadDotEnv() {
	for _, path := range []string{".env", filepath.Join("..", "..", ".env")} {
		_ = godotenv.Overload(path) // missing file is fine; values already in env
	}
}

// NormalizeVoiceProvider canonicalizes a voice provider value to "twilio" or
// "telnyx", defaulting to "twilio" for empty or unrecognized input.
func NormalizeVoiceProvider(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "telnyx":
		return "telnyx"
	default:
		return "twilio"
	}
}
