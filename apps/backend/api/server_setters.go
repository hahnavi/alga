// Code moved from http.go; see git history.

package api

import (
	"context"
	"time"

	"alga/api/agent"
	"alga/email"
	"alga/health"
	"alga/ics"
	"alga/incidentchannel"
	"alga/memory"
	"alga/oncall"
	"alga/rabbitmq"
	"alga/secretprovider"
	"alga/slack"
	"alga/sse"
	"alga/store"
	"alga/telnyx"
	"alga/twilio"
	"alga/valkey"
	"alga/webhook"
)

// SetAlertBroadcaster wires SSE (or other) publishing for API-driven alert mutations.
func (s *Server) SetAlertBroadcaster(b store.AlertEventPublisher) {
	s.alertBroadcaster = b
}

func (s *Server) SetSlackWebhookHandler(h *webhook.SlackWebhookHandler) {
	s.slackWebhookHandler = h
}

// SetDedupCache wires the dedup cache so resolve/reopen handlers can update tracking.
func (s *Server) SetDedupCache(cache webhook.DedupCache) {
	if cache != nil {
		s.dedupCache = &dedupCacheAdapter{cache: cache}
	}
}

func (s *Server) SetCooldownRemover(r cooldownRemover) {
	s.cooldownRemover = r
}

// SetHealthHandler wires the shared liveness/readiness probe handler used by
// the root-level /live, /ready, and /health endpoints.
func (s *Server) SetHealthHandler(h *health.Handler) {
	s.healthHandler = h
}

// SetAlertIngestor wires the webhook receiver so API handlers can persist
// user-authored alerts through the same pipeline as Grafana webhooks.
func (s *Server) SetAlertIngestor(r *webhook.Receiver) {
	s.alertIngestor = r
}

func (s *Server) SetPasswordResetDeps(prStore store.PasswordResetStore, sender *email.Sender) {
	s.passwordResetStore = prStore
	s.emailSender = sender
}

func (s *Server) SetRabbitMQPublisher(pub *rabbitmq.Publisher) {
	s.rabbitmqPublisher = pub
}

func (s *Server) SetTwilioClient(c *twilio.Client) { s.twilioClient = c }

func (s *Server) SetTelnyxClient(c *telnyx.Client) { s.telnyxClient = c }

func (s *Server) SetPendingNotifier(n pendingNotifier) {
	s.pendingNotifier = n
}

func (s *Server) SetInvestigator(inv alertInvestigator) {
	s.investigator = inv
}

func (s *Server) SetSSEBroker(broker *sse.Broker, vkClient *valkey.Client) {
	if broker != nil {
		s.ssePublisher = &sse.DualPublisher{Broker: broker, VKClient: vkClient}
	}
	s.sseBroker = broker
	s.vkClient = vkClient
}

func (s *Server) SetCancelSet(cs *valkey.CancelSet) { s.cancelSet = cs }

func (s *Server) InitSlackOAuthHandler(slackClient *slack.Client) {
	s.slackOAuthHandler = newSlackOAuthHandler(s.cfg, s.integrationStore, slackClient, s.vkClient, s.rebuildChatRouter)
}

func (s *Server) newOAuthStateStore() oauthStateStore {
	if s.vkClient != nil {
		return &valkeyOAuthStateStore{client: s.vkClient}
	}
	return newMemoryOAuthStateStore()
}

func (s *Server) newOIDCLoginStore() oidcLoginStore {
	if s.vkClient != nil {
		return &valkeyOIDCLoginStore{client: s.vkClient}
	}
	return newMemoryOIDCLoginStore()
}

func (s *Server) InitGoogleOAuthHandler() {
	s.googleOAuthHandler = newGoogleOAuthHandler(s.cfg, s.userStore, s.sessionStore, s.auditStore, s.newOAuthStateStore(), s.sessionExpiry)
}

func (s *Server) InitSlackSignInHandler() {
	s.slackSignInHandler = newSlackSignInHandler(s.cfg, s.userStore, s.sessionStore, s.auditStore, s.integrationStore, s.newOAuthStateStore(), s.sessionExpiry)
}

func (s *Server) InitUserSlackHandler() {
	s.userSlackHandler = newUserSlackHandler(s.cfg, s.userStore, s.integrationStore, s.auditStore, s.newOAuthStateStore())
}

func (s *Server) InitUserGoogleHandler() {
	s.userGoogleHandler = newUserGoogleHandler(s.cfg, s.userStore, s.auditStore, s.newOAuthStateStore())
}

func (s *Server) InitOIDCHandler() {
	if s.oidcProviderStore == nil {
		return
	}
	s.oidcHandler = newOIDCHandler(
		s.cfg, s.oidcProviderStore, s.oidcIdentityStore,
		s.userStore, s.sessionStore, s.auditStore,
		s.newOAuthStateStore(), s.newOIDCLoginStore(), s.sessionExpiry,
	)
}

func (s *Server) SetAgentSSE(h *agent.AgentSSEHandler) {
	s.agentSSE = h
}

func (s *Server) SetAgentService(svc *agent.Service) {
	s.agentService = svc
}

func (s *Server) SetAgentRateLimiter(rl RateLimiting) {
	s.agentRateLimiter = rl
}

// SetInvestigationForwarder wires default-agent forwarding for API-sourced investigation comments.
func (s *Server) SetInvestigationForwarder(f webhook.InvestigationAgentForwarder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.investigationForwarder = f
}

// SetAgentDMStore wires operator–agent private chat persistence.
func (s *Server) SetAgentDMStore(st store.AgentDMStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentDMStore = st
}

// SetKnowledgeStore wires the shared-knowledge notes store used by both
// admin REST (/api/v1/knowledge) and agent REST (/api/v1/agent/knowledge).
// Passing nil disables those endpoints.
func (s *Server) SetKnowledgeStore(st store.KnowledgeStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.knowledgeStore = st
}

// SetAgentAskStore wires the peer-ask RPC store used by the
// /api/v1/agent/peer-ask endpoints. Passing nil disables those endpoints.
func (s *Server) SetAgentAskStore(st store.AgentAskStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentAskStore = st
}

func (s *Server) SetSystemConfigStore(st store.SystemConfigStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.systemConfigStore = st
}

// SetSystemConfigUpdatedAt seeds the last-modified timestamp for the system
// config, typically from the persisted row at startup. PUT updates refresh it.
func (s *Server) SetSystemConfigUpdatedAt(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !t.IsZero() {
		s.systemConfigUpdatedAt = t
	}
}

func (s *Server) SetSummaryConfigApplier(fn func(enabled bool, defaultInterval time.Duration, severityIntervals map[string]time.Duration)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.summaryConfigApplier = fn
}

func (s *Server) SetMemoryService(svc memory.Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memorySvc = svc
}

func (s *Server) SetSummaryLLM(llm memory.LLM) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.summaryLLM = llm
}

func (s *Server) StartSummaryScheduler() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.summaryLLM != nil && s.dashboardStore != nil {
		s.summaryScheduler = NewDailySummaryScheduler(s)
		s.summaryScheduler.Start()
	}
}

func (s *Server) StopSummaryScheduler() {
	s.mu.Lock()
	sched := s.summaryScheduler
	s.mu.Unlock()
	if sched != nil {
		sched.Stop()
	}
}

func (s *Server) SetMaintenanceWindowStore(st store.MaintenanceWindowStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maintenanceWindowStore = st
}

func (s *Server) SetHeartbeatStore(st store.HeartbeatStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.heartbeatStore = st
}

func (s *Server) SetStatusPageStore(st store.StatusPageStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statusPageStore = st
}

func (s *Server) SetOIDCStores(provider store.OIDCProviderStore, identity store.OIDCIdentityStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.oidcProviderStore = provider
	s.oidcIdentityStore = identity
	if s.oidcHandler != nil {
		s.oidcHandler.providerStore = provider
		s.oidcHandler.identityStore = identity
	}
}

// SetCredentialStores wires the shared-credentials stores and the provider
// resolver registry. The admin REST endpoints (/api/v1/credential-providers,
// /api/v1/shared-secrets) and the agent fetch endpoint
// (/api/v1/agent/secrets/{id}) are disabled when these are nil.
func (s *Server) SetCredentialStores(providers store.CredentialProviderStore, secrets store.SharedSecretStore, reg *secretprovider.Registry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.credentialProviderStore = providers
	s.sharedSecretStore = secrets
	s.secretProviderRegistry = reg
}

func (s *Server) SetTriageResultStore(st store.TriageResultStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.triageResultStore = st
}

func (s *Server) SetTriageRuleStore(st store.TriageRuleStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.triageRuleStore = st
}

func (s *Server) SetIncidentStore(st store.IncidentStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incidentStore = st
}

func (s *Server) SetIncidentCoordinationStore(st store.IncidentCoordinationStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incidentCoordinationStore = st
}

func (s *Server) SetIncidentChannelManager(m *incidentchannel.Manager) {
	s.incidentChannelManager = m
}

func (s *Server) SetGoogleMeetClient(c ics.MeetSpaceCreator) {
	s.googleMeetClient = c
}

func (s *Server) SetServiceStore(st store.ServiceStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serviceStore = st
}

func (s *Server) SetTeamStore(st store.TeamStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.teamStore = st
}

func (s *Server) SetEscalationStore(st store.EscalationStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.escalationStore = st
}

func (s *Server) SetOnCallStore(st store.OnCallStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onCallStore = st
}

func (s *Server) SetOnCallResolver(r *oncall.Resolver) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onCallResolver = r
}

func (s *Server) SetHandoffStore(st store.HandoffStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handoffStore = st
}

func (s *Server) SetPlaybookStore(st store.PlaybookStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.playbookStore = st
}

func (s *Server) SetPostMortemStore(st store.PostMortemStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.postmortemStore = st
}

func (s *Server) SetActionItemStore(st store.ActionItemStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.actionItemStore = st
}

func (s *Server) SetStatusTracker(t interface {
	PropagateAndCascade(ctx context.Context, serviceID string) error
}) {
	s.statusTracker = t
}

func (s *Server) SetCache(c *valkey.Cache) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = c
}

// SetIdempotency wires the Idempotency-Key replay cache and TTL for the opted-in
// retry-safe write endpoints. Passing a nil cache disables replay (routes pass
// through unchanged), which is the case when Valkey is not configured or the
// idempotency feature flag is off.
func (s *Server) SetIdempotency(cache *valkey.IdempotencyCache, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idempotency = cache
	s.idempotencyTTL = ttl
}

func (s *Server) SetAlertInvestigationLifecycleService(svc *AlertInvestigationLifecycleService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alertInvestigationLifecycle = svc
}

func (s *Server) SetICSRoleStore(st store.ICSRoleStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.icsRoleStore = st
	if s.agentService != nil {
		s.agentService.SetICSRoleStore(st)
	}
}

func (s *Server) SetIncidentDocumentStore(st store.IncidentDocumentStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incidentDocumentStore = st
}
