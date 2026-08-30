package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"alga/api/agent"
	"alga/api/platform"
	"alga/config"
	"alga/email"
	"alga/health"
	"alga/ics"
	"alga/incidentchannel"
	"alga/logger"
	"alga/matching"
	"alga/mattermost"
	"alga/memory"
	"alga/oncall"
	"alga/rabbitmq"
	"alga/rbac"
	"alga/routing"
	"alga/secretprovider"
	"alga/slack"
	"alga/sse"
	"alga/store"
	"alga/telnyx"
	"alga/twilio"
	"alga/valkey"
	"alga/webhook"
)

// oauthHTTPClient is the shared HTTP client for outbound OAuth/token-exchange
// calls. A client-level timeout complements the per-request context deadline.
var oauthHTTPClient = &http.Client{Timeout: 10 * time.Second}

type cooldownRemover interface {
	RemoveCooldown(ctx context.Context, labels map[string]string)
}

type pendingNotifier interface {
	NotifyPending()
}

type alertInvestigator interface {
	ProcessAlert(ctx context.Context, alert rabbitmq.CorrelatedAlert) error
}

// dedupCacheAdapter wraps a webhook.DedupCache to provide context-free methods for API handlers.
type dedupCacheAdapter struct {
	cache webhook.DedupCache
}

func (a *dedupCacheAdapter) RemoveTracking(fingerprint string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	a.cache.RemoveTracking(ctx, fingerprint)
}

func (a *dedupCacheAdapter) MarkTracked(fingerprint string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := a.cache.MarkTracked(ctx, fingerprint); err != nil {
		logger.Error("failed to mark dedup tracking for reopened alert", "fingerprint", fingerprint, "error", err)
	}
}

const maxTokenNameLength = 256

type Server struct {
	mu sync.RWMutex

	cfg                      *config.Config
	alertStore               store.Store
	webhookTokenStore        store.WebhookTokenStore
	agentTokenStore          store.AgentTokenStore
	personalAccessTokenStore store.PersonalAccessTokenStore
	userStore                store.UserStore
	sessionStore             store.SessionStore
	auditStore               store.AuditStore
	integrationStore         store.IntegrationStore
	routeRulesStore          store.RouteRulesStore
	sessionExpiry            time.Duration
	loginLimiter             LoginRateLimiting
	rateLimiter              RateLimiting
	agentRateLimiter         RateLimiting
	mmClient                 *mattermost.Client
	slackClient              *slack.Client
	twilioClient             *twilio.Client
	telnyxClient             *telnyx.Client
	chatRouter               *webhook.ChatRouter

	alertInvestigationStore    store.AlertInvestigationStore
	incidentInvestigationStore store.IncidentInvestigationStore
	investigationThreadStore   store.InvestigationThreadStore
	rabbitmqPublisher          *rabbitmq.Publisher

	onRoutesChanged    func(*routing.Engine)
	ipExtractor        *ipExtractor
	alertBroadcaster   store.AlertEventPublisher
	dedupCache         *dedupCacheAdapter
	cooldownRemover    cooldownRemover
	sseBroker          *sse.Broker
	ssePublisher       *sse.DualPublisher
	vkClient           *valkey.Client
	cancelSet          *valkey.CancelSet
	slackOAuthHandler  *slackOAuthHandler
	googleOAuthHandler *googleOAuthHandler
	oidcHandler        *oidcHandler
	slackSignInHandler *slackSignInHandler
	userSlackHandler   *userSlackHandler
	userGoogleHandler  *userGoogleHandler
	agentSSE           *agent.AgentSSEHandler
	agentService       *agent.Service
	agentDMStore       store.AgentDMStore
	notificationStore  store.NotificationStore
	dashboardStore     store.DashboardStore
	knowledgeStore     store.KnowledgeStore
	agentAskStore      store.AgentAskStore
	memorySvc          memory.Service
	summaryLLM         memory.LLM
	summaryScheduler   *DailySummaryScheduler
	// investigationForwarder is optional; delivers investigation thread text to the default agent (Hermes or OpenClaw).
	investigationForwarder webhook.InvestigationAgentForwarder
	chatSync               *agent.ChatSyncService
	systemConfigStore      store.SystemConfigStore
	systemConfigUpdatedAt  time.Time
	summaryConfigApplier   func(enabled bool, defaultInterval time.Duration, severityIntervals map[string]time.Duration)
	maintenanceWindowStore store.MaintenanceWindowStore
	heartbeatStore         store.HeartbeatStore
	statusPageStore        store.StatusPageStore
	// healthHandler serves the root-level liveness/readiness probes.
	healthHandler             *health.Handler
	oidcProviderStore         store.OIDCProviderStore
	oidcIdentityStore         store.OIDCIdentityStore
	credentialProviderStore   store.CredentialProviderStore
	sharedSecretStore         store.SharedSecretStore
	secretProviderRegistry    *secretprovider.Registry
	pendingNotifier           pendingNotifier
	investigator              alertInvestigator
	triageResultStore         store.TriageResultStore
	triageRuleStore           store.TriageRuleStore
	postmortemStore           store.PostMortemStore
	actionItemStore           store.ActionItemStore
	incidentStore             store.IncidentStore
	incidentCoordinationStore store.IncidentCoordinationStore
	incidentChannelManager    *incidentchannel.Manager
	googleMeetClient          ics.MeetSpaceCreator
	serviceStore              store.ServiceStore
	teamStore                 store.TeamStore
	escalationStore           store.EscalationStore
	onCallStore               store.OnCallStore
	onCallResolver            *oncall.Resolver
	handoffStore              store.HandoffStore
	playbookStore             store.PlaybookStore
	// alertIngestor, if set, lets API handlers persist user-authored alerts
	// through the same routing/delivery/correlator path as Grafana webhooks.
	alertIngestor *webhook.Receiver
	// slackWebhookHandler is wired so the integration PUT handler can update the signing secret at runtime.
	slackWebhookHandler *webhook.SlackWebhookHandler
	passwordResetStore  store.PasswordResetStore
	emailSender         *email.Sender
	statusTracker       interface {
		PropagateAndCascade(ctx context.Context, serviceID string) error
	}
	icsRoleStore                store.ICSRoleStore
	incidentDocumentStore       store.IncidentDocumentStore
	cache                       *valkey.Cache
	alertInvestigationLifecycle *AlertInvestigationLifecycleService
	// idempotency, when set, makes opted-in retry-safe writes (create incident,
	// notification send) replayable via the Idempotency-Key header. nil leaves
	// those routes unwrapped so behavior is unchanged.
	idempotency    *valkey.IdempotencyCache
	idempotencyTTL time.Duration
}

func NewServer(
	cfg *config.Config,
	alertStore store.Store,
	webhookTokenStore store.WebhookTokenStore,
	agentTokenStore store.AgentTokenStore,
	userStore store.UserStore,
	sessionStore store.SessionStore,
	auditStore store.AuditStore,
	integrationStore store.IntegrationStore,
	routeRulesStore store.RouteRulesStore,
	sessionExpiry time.Duration,
	mmClient *mattermost.Client,
	slackClient *slack.Client,
	twilioClient *twilio.Client,
	telnyxClient *telnyx.Client,
	onRoutesChanged func(*routing.Engine),
	loginLimiter LoginRateLimiting,
	rateLimiter RateLimiting,
	alertInvestigationStore store.AlertInvestigationStore,
	incidentInvestigationStore store.IncidentInvestigationStore,
	investigationThreadStore store.InvestigationThreadStore,
	notificationStore store.NotificationStore,
	dashboardStore store.DashboardStore,
	personalAccessTokenStore store.PersonalAccessTokenStore,
) *Server {
	return &Server{
		cfg:                        cfg,
		alertStore:                 alertStore,
		webhookTokenStore:          webhookTokenStore,
		agentTokenStore:            agentTokenStore,
		userStore:                  userStore,
		sessionStore:               sessionStore,
		auditStore:                 auditStore,
		integrationStore:           integrationStore,
		routeRulesStore:            routeRulesStore,
		sessionExpiry:              sessionExpiry,
		loginLimiter:               loginLimiter,
		rateLimiter:                rateLimiter,
		mmClient:                   mmClient,
		slackClient:                slackClient,
		twilioClient:               twilioClient,
		telnyxClient:               telnyxClient,
		chatRouter:                 buildChatRouter(mmClient, slackClient),
		onRoutesChanged:            onRoutesChanged,
		alertInvestigationStore:    alertInvestigationStore,
		incidentInvestigationStore: incidentInvestigationStore,
		investigationThreadStore:   investigationThreadStore,
		chatSync:                   agent.NewChatSyncService(mmClient, slackClient, alertInvestigationStore),
		notificationStore:          notificationStore,
		dashboardStore:             dashboardStore,
		personalAccessTokenStore:   personalAccessTokenStore,
		ipExtractor:                newIPExtractor(cfg),
	}
}

func (s *Server) Register(mux *http.ServeMux) {
	// NOTE: /metrics is intentionally unauthenticated to match the Prometheus
	// scrape contract. It MUST be gated at the ingress/network level (bind to
	// a trusted network or require auth at the load balancer). Do not expose
	// this route on a public listener.
	mux.Handle("GET /metrics", NewMetricsHandler())

	// Root-level health probes (unauthenticated). These MUST NOT leak secrets,
	// connection strings, or internal state — only a status string and the
	// names of dependencies that were probed. /api/v1/readiness remains an alias
	// of /ready for backward compatibility during deprecation.
	mux.HandleFunc("/live", s.handleLive)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/health", s.handleReady)
	mux.HandleFunc("/api/v1/readiness", s.handleReady)

	// Public setup endpoints (no middleware)
	mux.HandleFunc("/api/v1/setup/status", s.rateLimitMiddleware(s.handleSetupStatus))
	mux.HandleFunc("/api/v1/setup", s.rateLimitMiddleware(s.handleSetup))

	// Public auth endpoints (no middleware)
	mux.HandleFunc("/api/v1/auth/login", s.rateLimitMiddleware(s.handleLogin))
	mux.HandleFunc("/api/v1/auth/logout", s.handleLogout)
	mux.HandleFunc("/api/v1/auth/me", s.authMiddleware(s.handleGetCurrentUser))
	mux.HandleFunc("/api/v1/auth/refresh", s.rateLimitMiddleware(s.handleRefreshSession))
	mux.HandleFunc("/api/v1/auth/change-password", s.authMiddleware(s.handleChangePassword))
	mux.HandleFunc("/api/v1/auth/change-email", s.authMiddleware(s.handleChangeEmail))
	mux.HandleFunc("/api/v1/auth/profile", s.authMiddleware(s.handleUpdateProfile))
	mux.HandleFunc("GET /api/v1/auth/sessions", s.authMiddleware(s.handleListSessions))
	mux.HandleFunc("DELETE /api/v1/auth/sessions/{id}", s.authMiddleware(s.handleRevokeSession))
	mux.HandleFunc("DELETE /api/v1/auth/sessions", s.authMiddleware(s.handleRevokeOtherSessions))
	mux.HandleFunc("/api/v1/auth/forgot-password", s.rateLimitMiddleware(s.handleForgotPassword))
	mux.HandleFunc("/api/v1/auth/reset-password", s.rateLimitMiddleware(s.handleResetPassword))
	mux.HandleFunc("/api/v1/auth/google/enabled", s.rateLimitMiddleware(s.handleGoogleOAuthEnabled))
	mux.HandleFunc("/api/v1/auth/google", s.rateLimitMiddleware(s.handleGoogleOAuthAuthorize))
	mux.HandleFunc("/api/v1/auth/google/callback", s.rateLimitMiddleware(s.handleGoogleOAuthCallback))
	mux.HandleFunc("/api/v1/auth/slack/enabled", s.rateLimitMiddleware(s.handleSlackSignInEnabled))
	mux.HandleFunc("/api/v1/auth/slack", s.rateLimitMiddleware(s.handleSlackSignInAuthorize))
	mux.HandleFunc("/api/v1/auth/slack/callback", s.rateLimitMiddleware(s.handleSlackSignInCallback))

	mux.HandleFunc("/api/v1/auth/oidc/providers", s.rateLimitMiddleware(s.handleOIDCPublicProviders))
	mux.HandleFunc("/api/v1/auth/oidc/{id}/authorize", s.rateLimitMiddleware(s.handleOIDCAuthorize))
	mux.HandleFunc("/api/v1/auth/oidc/{id}/callback", s.rateLimitMiddleware(s.handleOIDCCallback))

	// Protected API endpoints
	mux.HandleFunc("/api/v1/alerts", s.authMiddleware(s.handleAlerts, rbac.AlertsRead))
	mux.HandleFunc("/api/v1/alerts/", s.authMiddleware(s.handleAlertByNumber, rbac.AlertsRead))
	mux.HandleFunc("GET /api/v1/alerts/{alert_number}/related", s.authMiddleware(s.handleAlertRelated, rbac.AlertsRead))
	mux.HandleFunc("GET /api/v1/alerts/{alert_number}/thread", s.authMiddleware(s.handleAlertThread, rbac.AlertsRead))
	mux.HandleFunc("POST /api/v1/alerts/{alert_number}/thread/typing", s.authMiddleware(s.handleAlertThreadTyping, rbac.AlertsWrite))
	mux.HandleFunc("POST /api/v1/alerts/{alert_number}/thread/messages", s.authMiddleware(s.handleAlertThreadMessages, rbac.AlertsWrite))

	mux.HandleFunc("/api/v1/webhook-tokens", s.authMiddleware(s.handleWebhookTokens, rbac.TokensManage))
	mux.HandleFunc("/api/v1/webhook-tokens/", s.authMiddleware(s.handleWebhookTokenByID, rbac.TokensManage))
	// /api/v1/agent-tokens*, /api/v1/agent/capabilities, and the agent-bearer
	// /api/v1/agent/* routes are registered by the agent.Service below.
	mux.HandleFunc("/api/v1/user/tokens", s.authMiddleware(s.handleUserTokens))
	mux.HandleFunc("/api/v1/user/tokens/", s.authMiddleware(s.handleUserTokenByID))
	mux.HandleFunc("/api/v1/admin/tokens", s.authMiddleware(s.handleAdminTokens, rbac.TokensManage))
	mux.HandleFunc("/api/v1/admin/tokens/", s.authMiddleware(s.handleAdminTokenByID, rbac.TokensManage))
	mux.HandleFunc("/api/v1/routes", s.authMiddleware(s.handleRoutes, rbac.RoutesRead))
	// Read-only admin audit review surface: the audit:read permission
	// finally gates a route. Rate limiting inherits from authMiddleware.
	mux.HandleFunc("GET /api/v1/audit-events", s.authMiddleware(s.handleListAuditEvents, rbac.AuditRead))
	mux.HandleFunc("/api/v1/knowledge", s.authMiddleware(s.handleKnowledge, rbac.KnowledgeRead))
	mux.HandleFunc("/api/v1/knowledge/", s.authMiddleware(s.handleKnowledgeByID, rbac.KnowledgeRead))
	mux.HandleFunc("/api/v1/memories", s.authMiddleware(s.handleMemories, rbac.MemoriesRead))
	mux.HandleFunc("/api/v1/memories/", s.authMiddleware(s.handleMemoryByID, rbac.MemoriesRead))
	// /api/v1/agent/knowledge*, /api/v1/agent/secrets/ remain here because their
	// handlers (handleAgentKnowledge/ID, handleAgentSecretByID) still live in
	// package api (knowledge.go, shared_secrets.go). The other /api/v1/agent/*
	// routes are registered by agent.Service.Register below.
	mux.HandleFunc("/api/v1/agent/knowledge", s.agentBearerMiddleware(s.agentRateLimitMiddleware(s.handleAgentKnowledge)))
	mux.HandleFunc("/api/v1/agent/knowledge/", s.agentBearerMiddleware(s.agentRateLimitMiddleware(s.handleAgentKnowledgeByID)))
	mux.HandleFunc("/api/v1/agent/secrets/", s.agentBearerMiddleware(s.agentRateLimitMiddleware(s.handleAgentSecretByID)))
	mux.HandleFunc("/api/v1/credential-providers", s.authMiddleware(s.handleCredentialProviders))
	mux.HandleFunc("/api/v1/credential-providers/", s.authMiddleware(s.handleCredentialProviderByID))
	mux.HandleFunc("/api/v1/shared-secrets", s.authMiddleware(s.handleSharedSecrets))
	mux.HandleFunc("/api/v1/shared-secrets/", s.authMiddleware(s.handleSharedSecretByID))
	mux.HandleFunc("/api/v1/channels", s.authMiddleware(s.handleChannels, rbac.ChannelsRead))
	mux.HandleFunc("/api/v1/destinations", s.authMiddleware(s.handleDestinations, rbac.ChannelsRead))
	mux.HandleFunc("/api/v1/integrations", s.authMiddleware(s.handleIntegrations, rbac.IntegrationsRead))
	mux.HandleFunc("/api/v1/integrations/test", s.authMiddleware(s.handleTestIntegration, rbac.IntegrationsTest))
	mux.HandleFunc("/api/v1/integrations/slack/oauth/authorize", s.authMiddleware(s.handleSlackOAuthAuthorize, rbac.IntegrationsWrite))
	mux.HandleFunc("/api/v1/integrations/slack/oauth/callback", s.rateLimitMiddleware(s.handleSlackOAuthCallback))
	mux.HandleFunc("/api/v1/integrations/slack/disconnect", s.authMiddleware(s.handleSlackDisconnect, rbac.IntegrationsWrite))
	mux.HandleFunc("/api/v1/users/me/slack/authorize", s.authMiddleware(s.handleUserSlackAuthorize))
	mux.HandleFunc("/api/v1/users/me/slack/callback", s.rateLimitMiddleware(s.handleUserSlackCallback))
	mux.HandleFunc("/api/v1/users/me/slack/disconnect", s.authMiddleware(s.handleUserSlackDisconnect))
	mux.HandleFunc("/api/v1/users/me/google/authorize", s.authMiddleware(s.handleUserGoogleAuthorize))
	mux.HandleFunc("/api/v1/users/me/google/callback", s.rateLimitMiddleware(s.handleUserGoogleCallback))
	mux.HandleFunc("/api/v1/users/me/google/disconnect", s.authMiddleware(s.handleUserGoogleDisconnect))
	mux.HandleFunc("/api/v1/users", s.authMiddleware(s.handleUsers, rbac.UsersManage))
	mux.HandleFunc("/api/v1/users/", s.authMiddleware(s.handleUserByID, rbac.UsersManage))
	mux.HandleFunc("/api/v1/dashboard/stats", s.authMiddleware(s.handleDashboardStats, rbac.DashboardRead))
	mux.HandleFunc("/api/v1/dashboard/daily-summary", s.authMiddleware(s.handleDailySummary, rbac.DashboardRead))
	mux.HandleFunc("/api/v1/notifications", s.authMiddleware(s.handleNotifications))
	mux.HandleFunc("/api/v1/notifications/unread-count", s.authMiddleware(s.handleUnreadCount))
	mux.HandleFunc("/api/v1/notifications/read-all", s.authMiddleware(s.handleMarkAllRead))
	mux.HandleFunc("/api/v1/notifications/", s.authMiddleware(s.handleNotificationByID))
	mux.HandleFunc("/api/v1/action-items", s.authMiddleware(s.handleGlobalActionItems, rbac.PostMortemsRead))
	mux.HandleFunc("/api/v1/post-mortems", s.authMiddleware(s.handlePostMortemsList, rbac.PostMortemsRead))
	mux.HandleFunc("/api/v1/users/me/notification-preferences/test", s.authMiddleware(s.withIdempotency("notification:send", s.handleTestNotification)))
	mux.HandleFunc("/api/v1/users/me/notification-preferences", s.authMiddleware(s.handleNotificationPreferences))
	mux.HandleFunc("/api/v1/twilio/callback", s.rateLimitMiddleware(s.handleTwilioCallback))
	mux.HandleFunc("/api/v1/telnyx/callback", s.rateLimitMiddleware(s.handleTelnyxCallback))
	mux.HandleFunc("/api/v1/system/config", s.authMiddleware(s.handleSystemConfig, rbac.SystemConfigRead))
	mux.HandleFunc("/api/v1/onboarding/status", s.authMiddleware(s.handleOnboardingStatus, rbac.SystemConfigRead))
	mux.HandleFunc("/api/v1/onboarding/complete", s.authMiddleware(s.handleOnboardingComplete, rbac.SystemConfigWrite))
	mux.HandleFunc("/api/v1/maintenance-windows", s.authMiddleware(s.handleMaintenanceWindows, rbac.RoutesRead))
	mux.HandleFunc("/api/v1/maintenance-windows/", s.authMiddleware(s.handleMaintenanceWindowByID, rbac.RoutesRead))
	mux.HandleFunc("/api/v1/heartbeats/ping/{token}", s.rateLimitMiddleware(s.handleHeartbeatPing))
	mux.HandleFunc("GET /api/v1/heartbeats", s.authMiddleware(s.handleListHeartbeats, rbac.HeartbeatsRead))
	mux.HandleFunc("POST /api/v1/heartbeats", s.authMiddleware(s.handleCreateHeartbeat, rbac.HeartbeatsWrite))
	mux.HandleFunc("/api/v1/heartbeats/", s.authMiddleware(s.handleHeartbeatByID, rbac.HeartbeatsRead))
	mux.HandleFunc("GET /api/v1/status-pages", s.authMiddleware(s.handleListStatusPages, rbac.StatusPagesRead))
	mux.HandleFunc("POST /api/v1/status-pages", s.authMiddleware(s.handleCreateStatusPage, rbac.StatusPagesWrite))
	mux.HandleFunc("GET /api/v1/status-pages/slug/{slug}", s.authMiddleware(s.handleStatusPageViewBySlug, rbac.StatusPagesRead))
	mux.HandleFunc("/api/v1/status-pages/", s.authMiddleware(s.handleStatusPageRoutes, rbac.StatusPagesRead))

	mux.HandleFunc("GET /api/v1/oidc/providers", s.authMiddleware(s.handleOIDCListProviders, rbac.OIDCManage))
	mux.HandleFunc("POST /api/v1/oidc/providers", s.authMiddleware(s.handleOIDCCreateProvider, rbac.OIDCManage))
	mux.HandleFunc("/api/v1/oidc/providers/", s.authMiddleware(s.handleOIDCProviderRoutes, rbac.OIDCManage))
	mux.HandleFunc("/api/v1/incidents/metrics", s.authMiddleware(s.handleIncidentMetrics, rbac.IncidentsRead))
	mux.HandleFunc("/api/v1/incidents", s.authMiddleware(s.withIdempotency("incident:create", s.handleIncidents), rbac.IncidentsRead))
	mux.HandleFunc("/api/v1/incidents/", s.authMiddleware(s.handleIncidentRoutes, rbac.IncidentsRead))
	mux.HandleFunc("GET /api/v1/incidents/{incident_id}/thread", s.authMiddleware(s.handleIncidentThread, rbac.IncidentsRead))
	mux.HandleFunc("POST /api/v1/incidents/{incident_id}/thread/messages", s.authMiddleware(s.handleIncidentThreadMessages, rbac.IncidentsWrite))
	mux.HandleFunc("/api/v1/incidents/{id}/ics/roles", s.authMiddleware(s.handleICSRoles, rbac.IncidentsRead))
	mux.HandleFunc("/api/v1/incidents/{id}/ics/roles/", s.authMiddleware(s.handleICSRoleRoutes, rbac.IncidentsRead))
	mux.HandleFunc("/api/v1/incidents/{id}/ics/document", s.authMiddleware(s.handleICSDocument, rbac.IncidentsRead))
	mux.HandleFunc("/api/v1/incidents/{id}/ics/document/", s.authMiddleware(s.handleICSDocumentRoutes, rbac.IncidentsRead))
	mux.HandleFunc("/api/v1/incidents/{id}/begin-triage", s.authMiddleware(s.handleBeginTriage, rbac.IncidentsRead))
	mux.HandleFunc("/api/v1/incidents/{id}/promote", s.authMiddleware(s.handlePromote, rbac.IncidentsRead))
	mux.HandleFunc("/api/v1/services", s.authMiddleware(s.handleServices))
	mux.HandleFunc("/api/v1/services/", s.authMiddleware(s.handleServiceRoutes))
	mux.HandleFunc("/api/v1/triage/rules", s.authMiddleware(s.handleTriageRules, rbac.TriageRead))
	mux.HandleFunc("/api/v1/triage/rules/reorder", s.authMiddleware(s.handleTriageRulesReorder, rbac.TriageWrite))
	mux.HandleFunc("/api/v1/triage/rules/", s.authMiddleware(s.handleTriageRuleByID, rbac.TriageRead))
	mux.HandleFunc("/api/v1/triage/results", s.authMiddleware(s.handleTriageResults, rbac.TriageRead))
	mux.HandleFunc("/api/v1/triage/results/", s.authMiddleware(s.handleTriageResultByID, rbac.TriageRead))
	mux.HandleFunc("/api/v1/triage/stats", s.authMiddleware(s.handleTriageStats, rbac.TriageRead))

	mux.HandleFunc("/api/v1/teams", s.authMiddleware(s.handleTeams))
	mux.HandleFunc("/api/v1/teams/", s.authMiddleware(s.handleTeamRoutes))
	mux.HandleFunc("/api/v1/escalation-policies", s.authMiddleware(s.handleEscalationPolicies))
	mux.HandleFunc("/api/v1/escalation-policies/", s.authMiddleware(s.handleEscalationPolicyRoutes))
	mux.HandleFunc("/api/v1/on-call/schedules", s.authMiddleware(s.handleOnCallSchedules))
	mux.HandleFunc("/api/v1/on-call/schedules/", s.authMiddleware(s.handleOnCallScheduleRoutes))
	mux.HandleFunc("/api/v1/on-call/overrides/", s.authMiddleware(s.handleOnCallOverrideRoutes))
	mux.HandleFunc("/api/v1/on-call/handoffs", s.authMiddleware(s.handleListHandoffs))
	mux.HandleFunc("/api/v1/on-call/handoffs/", s.authMiddleware(s.handleHandoffRoutes))
	mux.HandleFunc("GET /api/v1/investigations/dead-lettered", s.authMiddleware(s.handleDeadLetteredInvestigations, rbac.AdminAccess))
	mux.HandleFunc("PATCH /api/v1/alert-investigations/{id}/assign", s.authMiddleware(s.handleAlertInvestigationAssign, rbac.AlertsWrite))
	mux.HandleFunc("PATCH /api/v1/incident-investigations/{id}/assign", s.authMiddleware(s.handleIncidentInvestigationAssign, rbac.IncidentsWrite))
	mux.HandleFunc("/api/v1/on-call/who-is-on-call", s.authMiddleware(s.handleWhoIsOnCall))
	mux.HandleFunc("/api/v1/on-call/me", s.authMiddleware(s.handleMyOnCall))
	mux.HandleFunc("/api/v1/on-call/my-on-call", s.authMiddleware(s.handleMyOnCall))
	mux.HandleFunc("/api/v1/on-call/metrics", s.authMiddleware(s.handleOnCallMetrics))
	mux.HandleFunc("/api/v1/playbooks", s.authMiddleware(s.handlePlaybooks))
	mux.HandleFunc("/api/v1/playbooks/", s.authMiddleware(s.handlePlaybookRoutes))

	// Internal: serve Mattermost plugin tarball for the auto-install CronJob.
	mux.HandleFunc("/internal/mm-plugin", s.handleMMPluginDownload)

	// Agent domain routes (agent-bearer + operator agent-token management).
	// Registered last so the agent.Service can compose its own middleware.
	if s.agentService != nil {
		s.agentService.Register(mux)
	}
}

// rateLimitMiddleware wraps a handler with per-IP rate limiting.
func (s *Server) rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return platform.RateLimitMiddleware(platform.RateLimitDeps{
		RateLimiter: s.rateLimiter,
		IPExtractor: s.ipExtractor,
	}, next)
}

// agentRateLimitMiddleware wraps a handler with per-agent-token rate limiting.
// Must be placed AFTER agentBearerMiddleware so the agent context is available.
func (s *Server) agentRateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return platform.AgentRateLimitMiddleware(platform.AgentRateLimitDeps{
		RateLimiter: s.agentRateLimiter,
	}, next)
}

// agentBearerMiddleware validates the agent bearer token and stores the agent
// identity in the request context. Compose agentRateLimitMiddleware inside it
// at registration time so per-agent rate limiting sees the agent context.
func (s *Server) agentBearerMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return platform.AgentBearerMiddleware(platform.AgentAuthDeps{
		AgentTokenStore: s.agentTokenStore,
	}, next)
}

// withIdempotency wraps a handler with Idempotency-Key replay for retry-safe
// writes when the cache is configured; otherwise it returns next unchanged so
// the route behaves exactly as before. scope namespaces keys per endpoint.
func (s *Server) withIdempotency(scope string, next http.HandlerFunc) http.HandlerFunc {
	if s.idempotency == nil {
		return next
	}
	return platform.WithIdempotency(s.idempotency, s.idempotencyTTL, scope, next)
}

// ErrorCode is a stable, machine-readable error identifier emitted in the
// error envelope. It aliases platform.ErrorCode so handlers can reference the
// constants without the platform. prefix.
type ErrorCode = platform.ErrorCode

// Stable error codes. Every error response carries exactly one of these.
const (
	ErrorCodeValidationFailed = platform.ErrorCodeValidationFailed
	ErrorCodeUnauthorized     = platform.ErrorCodeUnauthorized
	ErrorCodeForbidden        = platform.ErrorCodeForbidden
	ErrorCodeNotFound         = platform.ErrorCodeNotFound
	ErrorCodeConflict         = platform.ErrorCodeConflict
	ErrorCodeRateLimited      = platform.ErrorCodeRateLimited
	ErrorCodeInternal         = platform.ErrorCodeInternal
)

// ErrorDetail carries field-level validation context for an error.
type ErrorDetail = platform.ErrorDetail

// writeJSON delegates to platform.WriteJSON (low-level, no envelope). Prefer
// writeData for single-resource success responses.
func writeJSON(w http.ResponseWriter, code int, payload any) {
	platform.WriteJSON(w, code, payload)
}

// writeData wraps a single-resource success body in the {"data": ...} envelope.
func writeData(w http.ResponseWriter, code int, payload any) {
	platform.WriteData(w, code, payload)
}

// writeError delegates to platform.WriteError (canonical status for the code).
func writeError(w http.ResponseWriter, code ErrorCode, message string, details ...ErrorDetail) {
	platform.WriteError(w, code, message, details...)
}

// writeErrorStatus delegates to platform.WriteErrorStatus (explicit status).
func writeErrorStatus(w http.ResponseWriter, status int, code ErrorCode, message string, details ...ErrorDetail) {
	platform.WriteErrorStatus(w, status, code, message, details...)
}

// writeInternalError delegates to platform.WriteInternalError.
func writeInternalError(w http.ResponseWriter, err error, message string) {
	platform.WriteInternalError(w, err, message)
}

// writeStatus writes a 200 with a {"status": status} JSON body. Use for simple
// ok/status acknowledgements instead of repeating the map literal at every call site.
func writeStatus(w http.ResponseWriter, status string) {
	platform.WriteStatus(w, status)
}

// writeConflict writes a 409 Conflict response. Use for concurrent-modification /
// invariant-conflict failures instead of repeating the status literal.
func writeConflict(w http.ResponseWriter, message string) {
	platform.WriteConflict(w, message)
}

func validateRouteConfigs(routes []config.RouteConfig) error {
	for i, route := range routes {
		if route.MatchMode != "" && route.MatchMode != "all" && route.MatchMode != "any" {
			return fmt.Errorf("routes[%d].match_mode must be one of: all, any", i)
		}
		if len(route.Conditions) == 0 {
			return fmt.Errorf("routes[%d] must contain at least one condition", i)
		}
		for j, condition := range route.Conditions {
			source := strings.TrimSpace(condition.Source)
			field := strings.TrimSpace(condition.Field)
			operator := strings.TrimSpace(condition.Operator)
			if source != "labels" && source != "annotations" && source != "alert" {
				return fmt.Errorf("routes[%d].conditions[%d].source must be one of: labels, annotations, alert", i, j)
			}
			if field == "" {
				return fmt.Errorf("routes[%d].conditions[%d].field is required", i, j)
			}
			switch operator {
			case "exact", "contains", "prefix", "suffix", "wildcard", "regex", "exists", "not_exists":
			default:
				return fmt.Errorf("routes[%d].conditions[%d].operator is invalid", i, j)
			}
			if operator != "exists" && operator != "not_exists" && strings.TrimSpace(condition.Value) == "" {
				return fmt.Errorf("routes[%d].conditions[%d].value is required", i, j)
			}

			if operator == "regex" {
				if err := validateRegexPattern(condition.Value); err != nil {
					return fmt.Errorf("routes[%d].conditions[%d].value: %w", i, j, err)
				}
			}
		}
		if route.Silenced {
			continue
		}
		if len(route.Targets) == 0 {
			return fmt.Errorf("routes[%d] must have at least one target", i)
		}
		for j, t := range route.Targets {
			if strings.TrimSpace(t.Channel) == "" {
				return fmt.Errorf("routes[%d].targets[%d].channel is required", i, j)
			}
			p := strings.TrimSpace(t.Provider)
			if p != "" && p != "mattermost" && p != "slack" {
				return fmt.Errorf("routes[%d].targets[%d].provider must be one of: mattermost, slack", i, j)
			}
		}
	}
	return nil
}

// validateRegexPattern checks for potentially-dangerous regex patterns
func validateRegexPattern(pattern string) error {
	// Limit pattern length
	if len(pattern) > 128 {
		return errors.New("regex pattern too long (max 128 chars)")
	}

	// Test compile the pattern to catch invalid regex
	re, err := matching.GetCompiledRegex(pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}

	// Warn about potentially dangerous patterns (nested quantifiers)
	// These patterns can cause catastrophic backtracking
	dangerous := []string{
		".*.*", ".+.+", "\\.*\\.*", "\\.+\\.+",
		"(.*).*", "(.+).+", "(.*).+.", "(.+).*.",
		".*(.+)", ".+(.*)", "(.+)(.+)", "(.*)(.*)",
	}
	strPattern := re.String()
	for _, danger := range dangerous {
		if strings.Contains(strPattern, danger) {
			return errors.New("potentially slow regex pattern detected (nested quantifiers)")
		}
	}

	return nil
}

func mustParseCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

var privateCIDRs = []*net.IPNet{
	mustParseCIDR("10.0.0.0/8"),
	mustParseCIDR("172.16.0.0/12"),
	mustParseCIDR("192.168.0.0/16"),
	mustParseCIDR("fc00::/7"),
}

var privateHostSuffixes = []string{
	".local",
	".internal",
	".cluster.local",
	".svc",
	".svc.local",
}

var privateMetadataHosts = map[string]bool{
	"metadata.google.internal": true,
	"169.254.169.254":          true,
}

func isPrivateURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	host := u.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	if privateMetadataHosts[host] {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		for _, suffix := range privateHostSuffixes {
			if strings.HasSuffix(host, suffix) {
				return true
			}
		}
		return false
	}
	for _, cidr := range privateCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

func (s *Server) requireIncidentInvestigationStore(w http.ResponseWriter) bool {
	return s.requireStore(w, s.incidentInvestigationStore, "incident investigation store")
}

func (s *Server) requireIncidentStore(w http.ResponseWriter) bool {
	s.mu.RLock()
	st := s.incidentStore
	s.mu.RUnlock()
	return s.requireStore(w, st, "incident store")
}

func (s *Server) requireServiceStore(w http.ResponseWriter) bool {
	s.mu.RLock()
	st := s.serviceStore
	s.mu.RUnlock()
	return s.requireStore(w, st, "service store")
}

func (s *Server) requireTeamStore(w http.ResponseWriter) bool {
	s.mu.RLock()
	st := s.teamStore
	s.mu.RUnlock()
	return s.requireStore(w, st, "team store")
}

func (s *Server) requireEscalationStore(w http.ResponseWriter) bool {
	s.mu.RLock()
	st := s.escalationStore
	s.mu.RUnlock()
	return s.requireStore(w, st, "escalation store")
}

func (s *Server) requireOnCallStore(w http.ResponseWriter) bool {
	s.mu.RLock()
	st := s.onCallStore
	s.mu.RUnlock()
	return s.requireStore(w, st, "on-call store")
}
