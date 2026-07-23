// Package agent holds the agent-facing HTTP domain: the agent tool executor,
// the agent SSE handler, the agent-token / agent-message / peer-ask REST
// handlers, and the agent-scoped memory handlers.
//
// The package is extracted from the legacy god package `api` as part of the
// Prompt D decomposition (see docs/refactor/api-agent-extraction-plan.md). It
// MUST NOT import `alga/api`; cross-domain operations that the agent handlers
// used to perform via *Server method calls are instead injected as function
// fields on Service (see NewService).
package agent

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"alga/api/platform"
	"alga/config"
	"alga/incidentchannel"
	"alga/memory"
	"alga/oncall"
	"alga/rbac"
	"alga/sse"
	"alga/store"
	"alga/valkey"
)

// MemoryStore is the subset of memory.Service the agent memory handlers need.
// Defined here so this package does not depend on the memory package's
// concrete type; memory.Service satisfies it structurally.
type MemoryStore interface {
	Get(ctx context.Context, id uuid.UUID) (*store.AgentMemoryRecord, error)
	List(ctx context.Context, f store.MemoryFilters) ([]store.AgentMemoryRecord, int, error)
	CreateMemory(ctx context.Context, input memory.CreateMemoryInput) (*store.AgentMemoryRecord, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// AlertActionActor mirrors the unexported api.alertActionActor. It carries the
// event actor plus whether the actor is an agent (used by the alert
// resolve/reopen callbacks so they can preserve audit attribution).
type AlertActionActor struct {
	Actor     *store.EventActor
	IsAgent   bool
	AgentName string
}

// AlertInvestigationLifecycle is the narrow interface over
// api.AlertInvestigationLifecycleService used by AgentToolExecutor. The
// concrete type (wherever it lives at runtime) satisfies it structurally.
type AlertInvestigationLifecycle interface {
	RequireAlertActionAllowed(ctx context.Context, alertNumber int64, agent *store.AgentTokenRecord) error
	CompleteIfAllAlertsResolved(ctx context.Context, req store.AlertInvestigationLifecycleCompletionRequest) error
}

// pendingNotifier is the unexported api.pendingNotifier interface replicated
// here so this package does not import `alga/api`. worker.InvestigationScheduler
// satisfies it structurally.
type pendingNotifier interface {
	NotifyPending()
}

// memoryExtractor mirrors the unexported api.memoryExtractor interface; the
// concrete memory.Service satisfies it structurally.
type memoryExtractor interface {
	ExtractFromInvestigation(ctx context.Context, inv *store.AlertInvestigationRecord) error
}

// Service bundles every dependency the agent HTTP handlers need and registers
// the /api/v1/agent/* (agent-bearer) routes on a mux. The operator-facing
// /api/v1/agent-tokens* routes also live here because they manage agent
// tokens; they use operator (session/PAT) auth.
//
// Cross-domain operations that the legacy handlers performed via *Server
// method calls (resolve/reopen alert, post incident summary, publish incident
// SSE, derive schedule display name, revoke token by id) are injected as
// function fields so this package never has to import `alga/api`. wire.go
// populates them with closures over the parent Server.
type Service struct {
	cfg  *config.Config
	exec *AgentToolExecutor
	sse  *AgentSSEHandler

	// Stores owned by the agent domain.
	agentTokenStore store.AgentTokenStore
	agentDMStore    store.AgentDMStore
	agentAskStore   store.AgentAskStore
	memorySvc       MemoryStore
	auditStore      store.AuditStore

	// Cross-domain stores the agent handlers read/write directly. Owned by
	// package api; passed in via NewService options.
	alertStore                 store.Store
	incidentStore              store.IncidentStore
	incidentCoordinationStore  store.IncidentCoordinationStore
	incidentInvestigationStore store.IncidentInvestigationStore
	serviceStore               store.ServiceStore
	playbookStore              store.PlaybookStore
	onCallStore                store.OnCallStore
	onCallResolver             *oncall.Resolver
	userStore                  store.UserStore
	icsRoleStore               store.ICSRoleStore
	incidentChannelManager     *incidentchannel.Manager
	vkClient                   *valkey.Client

	// idempotency, when set, makes agent state-changing actions safely
	// retryable via the Idempotency-Key header. nil leaves routes unwrapped.
	idempotency    *valkey.IdempotencyCache
	idempotencyTTL time.Duration

	// SSE publisher for incident_updated / agent_dm_message events. Same type
	// the rest of the API uses; injected so agent does not reach into Server.
	ssePublisher *sse.DualPublisher

	// Operator-auth middleware deps for /api/v1/agent-tokens* and
	// /api/v1/agent/capabilities. Owned by the parent Server; the Service
	// only holds them so Register can compose platform.AuthMiddleware.
	authDeps platform.AuthDeps

	// Agent-bearer + per-agent rate-limit middleware deps.
	agentAuthDeps      platform.AgentAuthDeps
	agentRateLimitDeps platform.AgentRateLimitDeps

	// Cross-domain function fields. Each is a thin closure over a Server
	// method; nil-guards preserve the legacy 503/ignore behavior when the
	// collaborator is not configured.

	resolveAlert                 func(w http.ResponseWriter, r *http.Request, fingerprint string, a *AlertActionActor)
	reopenAlert                  func(w http.ResponseWriter, r *http.Request, fingerprint string, a *AlertActionActor)
	writeAlertsQueryResponse     func(w http.ResponseWriter, r *http.Request)
	postIncidentSummaryFromAgent func(ctx context.Context, agentRec *store.AgentTokenRecord, incidentID, text string) error
	scheduleDisplayName          func(ctx context.Context, sched *store.OnCallScheduleRecord) string
	revokeTokenByID              func(w http.ResponseWriter, r *http.Request, idHex string, revokeFn func(uuid.UUID) error, kind string)
}

// NewService constructs the agent Service. executor and agentSSE must already
// be fully configured (all their setters called). The store/function-field
// options wire cross-domain collaborators.
func NewService(
	cfg *config.Config,
	executor *AgentToolExecutor,
	agentSSE *AgentSSEHandler,
	agentTokenStore store.AgentTokenStore,
	agentDMStore store.AgentDMStore,
	agentAskStore store.AgentAskStore,
	memorySvc MemoryStore,
	auditStore store.AuditStore,
	authDeps platform.AuthDeps,
	agentRateLimitDeps platform.AgentRateLimitDeps,
	ssePublisher *sse.DualPublisher,
	opts ...ServiceOption,
) *Service {
	s := &Service{
		cfg:                cfg,
		exec:               executor,
		sse:                agentSSE,
		agentTokenStore:    agentTokenStore,
		agentDMStore:       agentDMStore,
		agentAskStore:      agentAskStore,
		memorySvc:          memorySvc,
		auditStore:         auditStore,
		authDeps:           authDeps,
		agentRateLimitDeps: agentRateLimitDeps,
		agentAuthDeps:      platform.AgentAuthDeps{AgentTokenStore: agentTokenStore},
		ssePublisher:       ssePublisher,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// ServiceOption configures a Service's cross-domain stores / function fields.
type ServiceOption func(*Service)

// WithAlertStores injects the alert/incident stores the agent handlers read.
func WithAlertStores(
	alertStore store.Store,
	incidentStore store.IncidentStore,
	incidentCoordinationStore store.IncidentCoordinationStore,
	incidentInvestigationStore store.IncidentInvestigationStore,
	playbookStore store.PlaybookStore,
) ServiceOption {
	return func(s *Service) {
		s.alertStore = alertStore
		s.incidentStore = incidentStore
		s.incidentCoordinationStore = incidentCoordinationStore
		s.incidentInvestigationStore = incidentInvestigationStore
		s.playbookStore = playbookStore
	}
}

// WithOnCall injects the on-call store/resolver and user store the agent
// on-call handler reads.
func WithOnCall(onCallStore store.OnCallStore, resolver *oncall.Resolver, userStore store.UserStore) ServiceOption {
	return func(s *Service) {
		s.onCallStore = onCallStore
		s.onCallResolver = resolver
		s.userStore = userStore
	}
}

// WithICSRoles injects the ICS role store used by the agent incident-context
// handler.
func WithICSRoles(icsRoleStore store.ICSRoleStore) ServiceOption {
	return func(s *Service) { s.icsRoleStore = icsRoleStore }
}

// WithIncidentChannelManager injects the Slack incident channel manager used
// by the agent status-update handler.
func WithIncidentChannelManager(m *incidentchannel.Manager) ServiceOption {
	return func(s *Service) { s.incidentChannelManager = m }
}

// WithVKClient injects the Valkey client used by the agent status-update
// handler (summary throttling keys).
func WithVKClient(c *valkey.Client) ServiceOption {
	return func(s *Service) { s.vkClient = c }
}

// WithIdempotencyCache enables Idempotency-Key replay for agent state-changing
// actions (e.g. POST /api/v1/agent/messages). A nil cache leaves the routes
// unwrapped so behavior is unchanged when Valkey/idempotency is disabled.
func WithIdempotencyCache(cache *valkey.IdempotencyCache, ttl time.Duration) ServiceOption {
	return func(s *Service) {
		s.idempotency = cache
		s.idempotencyTTL = ttl
	}
}

// WithResolveAlert wires the cross-domain alert-resolve callback.
func WithResolveAlert(fn func(w http.ResponseWriter, r *http.Request, fingerprint string, a *AlertActionActor)) ServiceOption {
	return func(s *Service) { s.resolveAlert = fn }
}

// WithReopenAlert wires the cross-domain alert-reopen callback.
func WithReopenAlert(fn func(w http.ResponseWriter, r *http.Request, fingerprint string, a *AlertActionActor)) ServiceOption {
	return func(s *Service) { s.reopenAlert = fn }
}

// WithWriteAlertsQueryResponse wires the alert list serializer.
func WithWriteAlertsQueryResponse(fn func(w http.ResponseWriter, r *http.Request)) ServiceOption {
	return func(s *Service) { s.writeAlertsQueryResponse = fn }
}

// WithPostIncidentSummaryFromAgent wires the incident summary poster.
func WithPostIncidentSummaryFromAgent(fn func(ctx context.Context, agentRec *store.AgentTokenRecord, incidentID, text string) error) ServiceOption {
	return func(s *Service) { s.postIncidentSummaryFromAgent = fn }
}

// WithScheduleDisplayName wires the schedule-display-name resolver used by the
// on-call handler.
func WithScheduleDisplayName(fn func(ctx context.Context, sched *store.OnCallScheduleRecord) string) ServiceOption {
	return func(s *Service) { s.scheduleDisplayName = fn }
}

// WithRevokeTokenByID wires the operator token revocation helper.
func WithRevokeTokenByID(fn func(w http.ResponseWriter, r *http.Request, idHex string, revokeFn func(uuid.UUID) error, kind string)) ServiceOption {
	return func(s *Service) { s.revokeTokenByID = fn }
}

// SetICSRoleStore wires the ICS role store used by agent incident-context
// handlers. Propagates to the executor for tool execution. Idempotent with
// the WithICSRoles construction option.
func (s *Service) SetICSRoleStore(st store.ICSRoleStore) {
	s.icsRoleStore = st
	if s.exec != nil {
		s.exec.SetICSRoleStore(st)
	}
}

// agentBearer wraps a handler with agent-bearer auth + per-agent rate limiting.
func (s *Service) agentBearer(next http.HandlerFunc) http.HandlerFunc {
	wrapped := platform.AgentRateLimitMiddleware(s.agentRateLimitDeps, next)
	return platform.AgentBearerMiddleware(s.agentAuthDeps, wrapped)
}

// applyIdempotency wraps next with Idempotency-Key replay when the cache is
// configured; otherwise it returns next unchanged.
func (s *Service) applyIdempotency(scope string, next http.HandlerFunc) http.HandlerFunc {
	if s.idempotency == nil {
		return next
	}
	return platform.WithIdempotency(s.idempotency, s.idempotencyTTL, scope, next)
}

// Register mounts every agent-domain route on mux:
//
//   - /api/v1/agent-tokens*, /api/v1/agent/capabilities — operator (session/
//     PAT) auth, requires rbac.TokensManage.
//   - /api/v1/agent/* — agent-bearer auth + per-agent rate limit.
//
// Routes whose handlers live outside this package (knowledge, secrets) stay
// registered in api/http.go.
func (s *Service) Register(mux *http.ServeMux) {
	// Operator-facing agent token management.
	mux.HandleFunc("/api/v1/agent-tokens", platform.AuthMiddleware(s.authDeps, s.handleAgentTokens, rbac.TokensManage))
	mux.HandleFunc("/api/v1/agent-tokens/", platform.AuthMiddleware(s.authDeps, s.handleAgentTokenByID, rbac.TokensManage))
	mux.HandleFunc("/api/v1/agent/capabilities", platform.AuthMiddleware(s.authDeps, s.handleAgentCapabilities, rbac.TokensManage))

	// Agent-bearer routes.
	mux.HandleFunc("/api/v1/agent/alerts", s.agentBearer(s.handleAgentAlerts))
	mux.HandleFunc("/api/v1/agent/alerts/", s.agentBearer(s.handleAgentAlertByFingerprint))
	mux.HandleFunc("/api/v1/agent/memories", s.agentBearer(s.handleAgentMemories))
	mux.HandleFunc("/api/v1/agent/memories/", s.agentBearer(s.handleAgentMemoryByID))
	mux.HandleFunc("/api/v1/agent/peer-ask", s.agentBearer(s.handleAgentPeerAsk))
	mux.HandleFunc("/api/v1/agent/peer-ask/", s.agentBearer(s.handleAgentPeerAskByID))
	mux.HandleFunc("/api/v1/agent/events", s.agentBearer(s.handleAgentSSE))
	mux.HandleFunc("/api/v1/agent/messages", s.agentBearer(s.applyIdempotency("agent:message", s.handleAgentSendMessage)))
	mux.HandleFunc("/api/v1/agent/messages/", s.agentBearer(s.handleAgentMessageRoute))
	mux.HandleFunc("/api/v1/agent/drafts", s.agentBearer(s.handleAgentDraft))
	mux.HandleFunc("/api/v1/agent/typing", s.agentBearer(s.handleAgentTyping))
	mux.HandleFunc("/api/v1/agent/heartbeat", s.agentBearer(s.handleAgentHeartbeat))
	mux.HandleFunc("/api/v1/agent/playbooks", s.agentBearer(s.handleAgentPlaybooks))
	mux.HandleFunc("/api/v1/agent/incidents/", s.agentBearer(s.handleAgentIncidentRoutes))
	mux.HandleFunc("/api/v1/agent/services", s.agentBearer(s.handleAgentServices))
	mux.HandleFunc("/api/v1/agent/on-call/current", s.agentBearer(s.handleAgentOnCallCurrent))
}

// timeNow is a package-level indirection so tests that need a fixed clock can
// override it (mirrors the alga timeNow convention).
var timeNow = time.Now
