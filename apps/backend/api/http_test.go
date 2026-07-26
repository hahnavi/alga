package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"alga/api/agent"
	"alga/api/platform"
	"alga/capability"
	"alga/config"
	entschema "alga/ent/schema"
	"alga/ics"
	"alga/incmetrics"
	"alga/rabbitmq"
	"alga/routing"
	"alga/sse"
	"alga/store"
	"alga/telnyx"

	"github.com/google/uuid"
)

type mockStore struct {
	alerts           []store.AlertRecord
	byFP             map[string]store.AlertRecord
	byNumber         map[int64]store.AlertRecord
	incidentsByAlert map[int64][]store.IncidentRecord
	lastFilter       map[string]any
	updateCalls      int
	resolveResult    store.AlertCascadeResult
	resolveCalls     int
	lastCascadeID    string
}

func (m *mockStore) Create(record store.AlertRecord) (int64, error) { return 1, nil }
func (m *mockStore) GetByFingerprint(fingerprint string) (*store.AlertRecord, error) {
	if r, ok := m.byFP[fingerprint]; ok {
		cp := r
		return &cp, nil
	}
	return nil, nil
}
func (m *mockStore) GetOpenByFingerprint(fingerprint string) (*store.AlertRecord, error) {
	if r, ok := m.byFP[fingerprint]; ok {
		if r.Status != "resolved" {
			cp := r
			return &cp, nil
		}
	}
	return nil, nil
}
func (m *mockStore) UpdateStatus(fingerprint, status string, resolvedEvent *store.AlertEvent) error {
	m.updateCalls++
	return nil
}
func (m *mockStore) UpdateStatusSilenced(fingerprint string) error {
	return nil
}
func (m *mockStore) UpdateDeliveryTargets(fingerprint string, targets []store.DeliveryTarget) error {
	return nil
}
func (m *mockStore) QueryAlerts(filter map[string]any) ([]store.AlertRecord, error) {
	m.lastFilter = filter
	return m.alerts, nil
}
func (m *mockStore) ListUninvestigatedAlerts(_ context.Context, _ time.Duration) ([]store.AlertRecord, error) {
	return nil, nil
}
func (m *mockStore) DeleteOlderThan(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
func (m *mockStore) CountOlderThan(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
func (m *mockStore) Close() {}

func (m *mockStore) AcknowledgeAlert(fingerprint string, actor *store.EventActor) error {
	return nil
}

func (m *mockStore) ReopenAlert(fingerprint string, ev store.AlertEvent) error {
	return nil
}

func (m *mockStore) ResolveAlertByUser(fingerprint string, actor *store.EventActor) error {
	return nil
}

func (m *mockStore) DeleteAlert(fingerprint string) error {
	return nil
}

func (m *mockStore) GetByAlertNumber(n int64) (*store.AlertRecord, error) {
	if m.byNumber != nil {
		if r, ok := m.byNumber[n]; ok {
			cp := r
			return &cp, nil
		}
	}
	return nil, nil
}
func (m *mockStore) AcknowledgeAlertByNumber(_ int64, _ *store.EventActor) error { return nil }
func (m *mockStore) ReopenAlertByNumber(_ int64, _ store.AlertEvent) error       { return nil }
func (m *mockStore) ResolveAlertByNumber(_ int64, _ *store.EventActor) error     { return nil }
func (m *mockStore) DeleteAlertByNumber(_ int64) error                           { return nil }

func (m *mockStore) TriageResultStore() store.TriageResultStore { return nil }

func (m *mockStore) TriageRuleStore() store.TriageRuleStore { return nil }

func (m *mockStore) LinkAlertToIncident(_ context.Context, _ string, _ int64) error {
	return nil
}

func (m *mockStore) UnlinkAlertFromIncident(_ context.Context, _ string, _ int64) error {
	return nil
}

func (m *mockStore) GetAlertsByIncident(_ context.Context, _ int64) ([]string, error) {
	return nil, nil
}

func (m *mockStore) ResolveAlertsByIncident(_ context.Context, incidentNumber int64, _ *store.EventActor) (store.AlertCascadeResult, error) {
	m.resolveCalls++
	m.lastCascadeID = strconv.FormatInt(incidentNumber, 10)
	return m.resolveResult, nil
}

func (m *mockStore) GetIncidentsByAlertNumber(_ context.Context, alertNumber int64) ([]store.IncidentRecord, error) {
	return m.incidentsByAlert[alertNumber], nil
}

type trackingAlertStore struct {
	mockStore
	reopened     []string
	acknowledged []string
	resolved     []string
}

func (m *trackingAlertStore) AcknowledgeAlert(fingerprint string, actor *store.EventActor) error {
	m.acknowledged = append(m.acknowledged, fingerprint)
	if m.byFP != nil {
		if rec, ok := m.byFP[fingerprint]; ok {
			rec.Acknowledged = true
			m.byFP[fingerprint] = rec
		}
	}
	return nil
}

func (m *trackingAlertStore) ResolveAlertByUser(fingerprint string, _ *store.EventActor) error {
	m.resolved = append(m.resolved, fingerprint)
	return nil
}

func (m *trackingAlertStore) ReopenAlert(fingerprint string, ev store.AlertEvent) error {
	m.reopened = append(m.reopened, fingerprint)
	if m.byFP != nil {
		if rec, ok := m.byFP[fingerprint]; ok {
			rec.Status = "firing"
			m.byFP[fingerprint] = rec
		}
	}
	return nil
}

func (m *trackingAlertStore) ReopenAlertByNumber(n int64, ev store.AlertEvent) error {
	if m.byNumber != nil {
		if rec, ok := m.byNumber[n]; ok {
			m.reopened = append(m.reopened, rec.Fingerprint)
			rec.Status = "firing"
			m.byNumber[n] = rec
			return nil
		}
	}
	for _, rec := range m.byFP {
		if rec.AlertNumber == n {
			m.reopened = append(m.reopened, rec.Fingerprint)
			return nil
		}
	}
	return fmt.Errorf("alert not found")
}

type trackingInvestigationForwarder struct {
	mu             sync.Mutex
	messages       map[string][]string
	overrideOnline *bool
}

func (t *trackingInvestigationForwarder) AgentOnline(agentIDHex string) bool {
	if t.overrideOnline != nil {
		return *t.overrideOnline
	}
	return true
}

func (t *trackingInvestigationForwarder) ForwardToAgent(agentIDHex, investigationID, senderID, senderName, message string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.messages == nil {
		t.messages = map[string][]string{}
	}
	t.messages[investigationID] = append(t.messages[investigationID], message)
	return nil
}

func (t *trackingInvestigationForwarder) ForwardEventToAgent(agentIDHex string, event sse.Event) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.messages == nil {
		t.messages = map[string][]string{}
	}
	key := "event:" + agentIDHex
	t.messages[key] = append(t.messages[key], event.Type)
	return nil
}

type mockWebhookTokenStore struct {
	tokens map[string]store.WebhookTokenRecord
}

func (m *mockWebhookTokenStore) CreateToken(name string, expiresAt *time.Time) (*store.WebhookTokenRecord, error) {
	id := uuid.New()
	record := store.WebhookTokenRecord{
		ID:        id,
		Name:      name,
		Token:     "tok-" + id.String(),
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}
	m.tokens[id.String()] = record
	return &record, nil
}
func (m *mockWebhookTokenStore) ListTokens() ([]store.WebhookTokenRecord, error) {
	out := make([]store.WebhookTokenRecord, 0, len(m.tokens))
	for _, token := range m.tokens {
		out = append(out, token)
	}
	return out, nil
}
func (m *mockWebhookTokenStore) RevokeToken(id uuid.UUID) error {
	delete(m.tokens, id.String())
	return nil
}
func (m *mockWebhookTokenStore) ValidateToken(token string) (bool, error) { return true, nil }
func (m *mockWebhookTokenStore) Close()                                   {}

type mockAgentTokenStore struct{}

func (m *mockAgentTokenStore) CreateToken(name string, expiresAt *time.Time, agentType string, capabilities []string) (*store.AgentTokenRecord, error) {
	return nil, nil
}
func (m *mockAgentTokenStore) ListTokens() ([]store.AgentTokenRecord, error) { return nil, nil }
func (m *mockAgentTokenStore) RevokeToken(id uuid.UUID) error                { return nil }
func (m *mockAgentTokenStore) ValidateToken(token string) (*store.AgentTokenRecord, error) {
	return nil, nil
}
func (m *mockAgentTokenStore) RegenerateToken(id uuid.UUID) (*store.AgentTokenRecord, error) {
	return nil, nil
}
func (m *mockAgentTokenStore) GetActiveAgentTokenByID(id uuid.UUID) (*store.AgentTokenRecord, error) {
	return nil, nil
}
func (m *mockAgentTokenStore) UpdateAgentConfig(id uuid.UUID, scope string, selectors []config.RouteCondition, capabilities []string) error {
	return nil
}
func (m *mockAgentTokenStore) SetAgentEnabled(id uuid.UUID, enabled bool) error {
	return nil
}
func (m *mockAgentTokenStore) ListActiveAgents() ([]store.AgentTokenRecord, error) {
	return nil, nil
}
func (m *mockAgentTokenStore) Close() {}

// testAgentTokenStore validates a fixed Bearer token for agent API tests.
type testAgentTokenStore struct {
	mockAgentTokenStore
	validToken   string
	agentID      uuid.UUID
	name         string
	capabilities []string
}

func (t *testAgentTokenStore) ValidateToken(token string) (*store.AgentTokenRecord, error) {
	if token == t.validToken {
		caps := t.capabilities
		if caps == nil {
			caps = capability.All
		}
		return &store.AgentTokenRecord{
			ID:           t.agentID,
			Name:         t.name,
			AgentType:    store.AgentTypeHermes,
			Capabilities: caps,
		}, nil
	}
	return nil, nil
}

type mockUserStore struct {
	users []store.UserRecord
}

func (m *mockUserStore) CreateUser(email, password, role string) (*store.UserRecord, error) {
	id := uuid.New()
	record := store.UserRecord{ID: id, Email: email, Role: role, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	m.users = append(m.users, record)
	return &record, nil
}
func (m *mockUserStore) GetByEmail(email string) (*store.UserRecord, error) {
	for _, u := range m.users {
		if u.Email == email {
			return &u, nil
		}
	}
	return nil, nil
}
func (m *mockUserStore) GetByID(id uuid.UUID) (*store.UserRecord, error) {
	for _, u := range m.users {
		if u.ID == id {
			return &u, nil
		}
	}
	return nil, nil
}
func (m *mockUserStore) ListUsers() ([]store.UserRecord, error)                { return m.users, nil }
func (m *mockUserStore) UpdateUser(id uuid.UUID, updates map[string]any) error { return nil }
func (m *mockUserStore) DeleteUser(id uuid.UUID) error                         { return nil }
func (m *mockUserStore) CountAdmins() (int64, error)                           { return 2, nil }
func (m *mockUserStore) Authenticate(email, password string) (*store.UserRecord, error) {
	for _, u := range m.users {
		if u.Email == email {
			return &u, nil
		}
	}
	return nil, nil
}
func (m *mockUserStore) CountUsers() (int64, error)                              { return int64(len(m.users)), nil }
func (m *mockUserStore) RecordFailedLogin(email string) error                    { return nil }
func (m *mockUserStore) RecordSuccessfulLogin(userID uuid.UUID, ip string) error { return nil }
func (m *mockUserStore) UnlockAccount(userID uuid.UUID) error                    { return nil }
func (m *mockUserStore) GetNotificationPreferences(ctx context.Context, userID string) (map[string]any, error) {
	return map[string]any{}, nil
}
func (m *mockUserStore) UpdateNotificationPreferences(ctx context.Context, userID string, prefs map[string]any) error {
	return nil
}
func (m *mockUserStore) GetByGoogleID(googleID string) (*store.UserRecord, error) {
	return nil, nil
}
func (m *mockUserStore) GetBySlackUserID(slackUserID string) (*store.UserRecord, error) {
	return nil, nil
}
func (m *mockUserStore) UpdateGoogleID(userID uuid.UUID, googleID string) error {
	return nil
}
func (m *mockUserStore) ClearGoogleID(userID uuid.UUID) error {
	return nil
}
func (m *mockUserStore) SetSlackIdentity(ctx context.Context, userID uuid.UUID, slackUserID, slackDisplayName string) error {
	return nil
}
func (m *mockUserStore) ClearSlackIdentity(ctx context.Context, userID uuid.UUID) error {
	return nil
}

type mockSessionStore struct {
	sessions map[string]*store.SessionRecord
}

func (m *mockSessionStore) CreateSession(userID uuid.UUID, ip, userAgent string) (*store.SessionRecord, error) {
	id := "test-session-id"
	record := &store.SessionRecord{ID: id, UserID: userID, CreatedAt: time.Now(), ExpiresAt: time.Now().Add(24 * time.Hour)}
	m.sessions[id] = record
	return record, nil
}
func (m *mockSessionStore) GetSession(id string) (*store.SessionRecord, error) {
	if s, ok := m.sessions[id]; ok {
		return s, nil
	}
	return nil, nil
}
func (m *mockSessionStore) GetSessionByRefreshToken(token string) (*store.SessionRecord, error) {
	return nil, nil
}
func (m *mockSessionStore) RefreshSession(sessionID string, ip, userAgent string) (*store.SessionRecord, error) {
	return m.GetSession(sessionID)
}
func (m *mockSessionStore) DeleteSession(id string) error {
	delete(m.sessions, id)
	return nil
}
func (m *mockSessionStore) DeleteAllUserSessions(userID uuid.UUID) error { return nil }
func (m *mockSessionStore) DeleteExpired(_ context.Context) (int, error) { return 0, nil }

type mockAuditStore struct{}

func (m *mockAuditStore) Log(event store.AuditEvent, userID *uuid.UUID, username, ip, userAgent string, success bool, details map[string]any) {
}
func (m *mockAuditStore) LogEntity(event store.AuditEvent, userID *uuid.UUID, username, ip, userAgent string, success bool, details map[string]any, entityType string, entityID *uuid.UUID) {
}
func (m *mockAuditStore) Query(filter map[string]any) ([]store.AuditRecord, error) {
	return nil, nil
}
func (m *mockAuditStore) GetRecentEvents(limit int) ([]store.AuditRecord, error) { return nil, nil }

type mockIntegrationStore struct {
	cfg *store.IntegrationConfig
}

func (m *mockIntegrationStore) Get() (*store.IntegrationConfig, error) { return m.cfg, nil }
func (m *mockIntegrationStore) Save(cfg store.IntegrationConfig) error { m.cfg = &cfg; return nil }

type mockRouteRulesStore struct {
	routes []config.RouteConfig
}

func (m *mockRouteRulesStore) Get() ([]config.RouteConfig, error) {
	if m.routes == nil {
		return []config.RouteConfig{}, nil
	}
	return m.routes, nil
}

func (m *mockRouteRulesStore) Save(routes []config.RouteConfig) error {
	m.routes = routes
	return nil
}

var testAdminUser = store.UserRecord{
	ID:        uuid.New(),
	Email:     "admin@alga.local",
	Role:      "admin",
	CreatedAt: time.Now(),
	UpdatedAt: time.Now(),
}

type mockAlertInvestigationStore struct{}

func (m *mockAlertInvestigationStore) CreateAlertInvestigation(ctx context.Context, record store.AlertInvestigationRecord) (*store.AlertInvestigationRecord, error) {
	return &record, nil
}

func (m *mockAlertInvestigationStore) GetAlertInvestigation(ctx context.Context, id string) (*store.AlertInvestigationRecord, error) {
	return nil, nil
}

func (m *mockAlertInvestigationStore) ListAlertInvestigationsByAlertNumber(ctx context.Context, alertNumber int64) ([]store.AlertInvestigationRecord, error) {
	return nil, nil
}

func (m *mockAlertInvestigationStore) GetActiveAlertInvestigationByCorrelationKey(ctx context.Context, correlationKey string) (*store.AlertInvestigationRecord, error) {
	return nil, nil
}

func (m *mockAlertInvestigationStore) AppendAlertsToAlertInvestigation(ctx context.Context, id string, alerts []rabbitmq.CorrelatedAlert) error {
	return nil
}

func (m *mockAlertInvestigationStore) AddAlertInvestigationUpdate(ctx context.Context, id string, update store.InvestigationUpdate) error {
	return nil
}

func (m *mockAlertInvestigationStore) MarkAlertInvestigationPromoted(ctx context.Context, id string, incidentID string, incidentNumber int64, incidentInvestigationID string) (*store.AlertInvestigationRecord, error) {
	return nil, nil
}

func (m *mockAlertInvestigationStore) UpdateAlertInvestigationStatus(ctx context.Context, id string, status string) error {
	return nil
}

func (m *mockAlertInvestigationStore) GetAlertInvestigationByAlertNumber(ctx context.Context, alertNumber int64) (*store.AlertInvestigationRecord, error) {
	return nil, nil
}

func (m *mockAlertInvestigationStore) ListPendingAlertInvestigations(ctx context.Context, limit int64) ([]store.AlertInvestigationRecord, error) {
	return nil, nil
}

func (m *mockAlertInvestigationStore) ClaimPendingAlertInvestigation(ctx context.Context, id string, agentID string, agentName string, agentType string) (*store.AlertInvestigationRecord, error) {
	return nil, nil
}

func (m *mockAlertInvestigationStore) TransitionAlertInvestigationStatus(ctx context.Context, id string, fromStatuses []string, toStatus string) error {
	return nil
}

func (m *mockAlertInvestigationStore) PatchAlertInvestigationOutcome(ctx context.Context, id string, rootCause *string, resolution *string) error {
	return nil
}

func (m *mockAlertInvestigationStore) UpdateAlertInvestigationAgent(ctx context.Context, id string, agentID string, agentName string, agentType string) error {
	return nil
}

func (m *mockAlertInvestigationStore) SetAlertInvestigationAssignee(ctx context.Context, id string, assigneeType string, assigneeID *uuid.UUID) error {
	return nil
}

func (m *mockAlertInvestigationStore) ResetInvestigatingByAgent(ctx context.Context, agentID string) error {
	return nil
}

func (m *mockAlertInvestigationStore) ResetAssignedByAgent(ctx context.Context, agentID string) error {
	return nil
}

func (m *mockAlertInvestigationStore) CountActiveByAgent(ctx context.Context, agentID string) (int, error) {
	return 0, nil
}

func (m *mockAlertInvestigationStore) CountActiveByAgents(ctx context.Context, agentIDs []string) (map[string]int, error) {
	result := make(map[string]int)
	for _, id := range agentIDs {
		result[id] = 0
	}
	return result, nil
}

func (m *mockAlertInvestigationStore) DeleteAlertInvestigation(ctx context.Context, id string) error {
	return nil
}

func (m *mockAlertInvestigationStore) ListAlertInvestigations(ctx context.Context, filter map[string]any) ([]store.AlertInvestigationRecord, error) {
	return nil, nil
}

func (m *mockAlertInvestigationStore) UpdateAlertInvestigationMessage(ctx context.Context, investigationID string, updateID string, message string) error {
	return nil
}

func (m *mockAlertInvestigationStore) DeleteAlertInvestigationMessage(ctx context.Context, investigationID string, updateID string) error {
	return nil
}

func (m *mockAlertInvestigationStore) SetAlertInvestigationUpdateMMPostID(ctx context.Context, investigationID string, updateID string, mmPostID string) error {
	return nil
}

func (m *mockAlertInvestigationStore) SetAlertInvestigationUpdateSlackMessageTS(ctx context.Context, investigationID string, updateID string, slackMessageTS string) error {
	return nil
}

func (m *mockAlertInvestigationStore) GetAlertInvestigationByMMThread(ctx context.Context, mmThreadID string) (*store.AlertInvestigationRecord, error) {
	return nil, nil
}

func (m *mockAlertInvestigationStore) GetAlertInvestigationBySlackThread(ctx context.Context, channelID string, threadTS string) (*store.AlertInvestigationRecord, error) {
	return nil, nil
}

func (m *mockAlertInvestigationStore) FindSimilarAlertInvestigations(ctx context.Context, q store.SimilarAlertInvestigationsQuery) ([]store.AlertInvestigationRecord, error) {
	return nil, nil
}

func (m *mockAlertInvestigationStore) GetCurrentAlertInvestigationByAlertNumber(ctx context.Context, alertNumber int64) (*store.AlertInvestigationRecord, error) {
	return nil, nil
}

func (m *mockAlertInvestigationStore) GetCurrentAlertInvestigationSummariesByAlertNumbers(ctx context.Context, alertNumbers []int64) (map[int64]store.AlertInvestigationSummary, error) {
	return map[int64]store.AlertInvestigationSummary{}, nil
}

func (m *mockAlertInvestigationStore) MarkAlertInvestigationAlertsCurrent(ctx context.Context, investigationID string, current bool) error {
	return nil
}

func (m *mockAlertInvestigationStore) AppendAlertInvestigationEvent(ctx context.Context, investigationUUID uuid.UUID, event store.AlertInvestigationEvent) error {
	return nil
}

func (m *mockAlertInvestigationStore) CompleteAlertInvestigation(ctx context.Context, id string, completion store.AlertInvestigationCompletion) error {
	return nil
}

func (m *mockAlertInvestigationStore) RequeueAlertInvestigation(ctx context.Context, id string, requeue store.AlertInvestigationRequeue) error {
	return nil
}

func (m *mockAlertInvestigationStore) ListStalledAssignedAlertInvestigations(ctx context.Context, threshold time.Duration) ([]store.AlertInvestigationRecord, error) {
	return nil, nil
}

func (m *mockAlertInvestigationStore) ListStalledInvestigatingAlertInvestigations(ctx context.Context, threshold time.Duration) ([]store.AlertInvestigationRecord, error) {
	return nil, nil
}

func (m *mockAlertInvestigationStore) ResetStalledAssignedAlertInvestigations(timeout time.Duration) ([]string, error) {
	return nil, nil
}

func (m *mockAlertInvestigationStore) ResetStalledInvestigatingAlertInvestigations(timeout time.Duration) ([]string, error) {
	return nil, nil
}

type mockIncidentInvestigationStore struct{}

func (m *mockIncidentInvestigationStore) CreateIncidentInvestigation(ctx context.Context, record store.IncidentInvestigationRecord) (*store.IncidentInvestigationRecord, error) {
	return &record, nil
}
func (m *mockIncidentInvestigationStore) GetIncidentInvestigation(ctx context.Context, id string) (*store.IncidentInvestigationRecord, error) {
	return nil, nil
}
func (m *mockIncidentInvestigationStore) GetActiveIncidentInvestigationByIncident(ctx context.Context, incidentNumber int64) (*store.IncidentInvestigationRecord, error) {
	return nil, nil
}
func (m *mockIncidentInvestigationStore) ListIncidentInvestigationsByIncident(ctx context.Context, incidentNumber int64) ([]store.IncidentInvestigationRecord, error) {
	return nil, nil
}
func (m *mockIncidentInvestigationStore) AddIncidentInvestigationUpdate(ctx context.Context, id string, update store.InvestigationUpdate) error {
	return nil
}
func (m *mockIncidentInvestigationStore) UpdateIncidentInvestigationStatus(ctx context.Context, id string, status string) error {
	return nil
}
func (m *mockIncidentInvestigationStore) ClaimPendingIncidentInvestigation(ctx context.Context, id string, agentID string, agentName string, agentType string) (*store.IncidentInvestigationRecord, error) {
	return nil, nil
}
func (m *mockIncidentInvestigationStore) ListPendingIncidentInvestigations(ctx context.Context, limit int64) ([]store.IncidentInvestigationRecord, error) {
	return nil, nil
}
func (m *mockIncidentInvestigationStore) SetIncidentInvestigationSummary(ctx context.Context, incidentInvestigationID string, summary *entschema.InvestigationSummary) error {
	return nil
}
func (m *mockIncidentInvestigationStore) SetIncidentInvestigationAssignee(ctx context.Context, id string, assigneeType string, assigneeID *uuid.UUID) error {
	return nil
}

type trackingIncidentStore struct {
	nextID     string
	nextNumber int64
	created    []store.IncidentRecord
	byIncident map[int64]*store.IncidentRecord
	byUUID     map[string]*store.IncidentRecord
	timeline   []store.IncidentTimelineEntryRecord
}

func (s *trackingIncidentStore) ReserveIncidentNumber(ctx context.Context) (int64, error) {
	if s.nextNumber == 0 {
		s.nextNumber = 12
	}
	return s.nextNumber, nil
}

func (s *trackingIncidentStore) CreateIncident(ctx context.Context, record *store.IncidentRecord) (*store.IncidentRecord, error) {
	if record.ID == uuid.Nil {
		record.ID = uuid.New()
	}
	s.created = append(s.created, *record)
	stored := *record
	if s.byIncident == nil {
		s.byIncident = map[int64]*store.IncidentRecord{}
	}
	if s.byUUID == nil {
		s.byUUID = map[string]*store.IncidentRecord{}
	}
	s.byIncident[record.IncidentNumber] = &stored
	s.byUUID[record.ID.String()] = &stored
	return &stored, nil
}

func (s *trackingIncidentStore) GetIncident(ctx context.Context, incidentNumber int64) (*store.IncidentRecord, error) {
	if s.byIncident != nil {
		if rec, ok := s.byIncident[incidentNumber]; ok {
			cp := *rec
			return &cp, nil
		}
	}
	return nil, store.ErrIncidentNotFound
}

func (s *trackingIncidentStore) GetIncidentByID(ctx context.Context, id uuid.UUID) (*store.IncidentRecord, error) {
	if s.byUUID != nil {
		if rec, ok := s.byUUID[id.String()]; ok {
			cp := *rec
			return &cp, nil
		}
	}
	return nil, store.ErrIncidentNotFound
}

func (s *trackingIncidentStore) UpdateIncident(ctx context.Context, incidentNumber int64, record *store.IncidentRecord) (*store.IncidentRecord, error) {
	return record, nil
}
func (s *trackingIncidentStore) DeleteIncident(ctx context.Context, incidentNumber int64) error {
	return nil
}
func (s *trackingIncidentStore) ListIncidents(ctx context.Context, filter store.IncidentListFilter) ([]store.IncidentRecord, int64, error) {
	return nil, 0, nil
}
func (s *trackingIncidentStore) ListSLAEligibleIncidents(ctx context.Context) ([]store.IncidentRecord, error) {
	return nil, nil
}
func (s *trackingIncidentStore) UpdateIncidentStatus(ctx context.Context, incidentNumber int64, status string) error {
	return nil
}
func (s *trackingIncidentStore) TransitionIncidentStatus(ctx context.Context, incidentNumber int64, fromStatuses []string, toStatus string) error {
	return nil
}
func (s *trackingIncidentStore) AddTimelineEntry(ctx context.Context, record *store.IncidentTimelineEntryRecord) error {
	s.timeline = append(s.timeline, *record)
	return nil
}
func (s *trackingIncidentStore) GetTimeline(ctx context.Context, incidentNumber int64) ([]store.IncidentTimelineEntryRecord, error) {
	return nil, nil
}
func (s *trackingIncidentStore) GetIncidentMetrics(ctx context.Context, startDate, endDate time.Time) (*incmetrics.Metrics, error) {
	return nil, nil
}
func (s *trackingIncidentStore) CountActiveByService(ctx context.Context) (map[string]int64, error) {
	return nil, nil
}
func (s *trackingIncidentStore) CountActiveByServiceID(ctx context.Context, serviceID string) (int, error) {
	return 0, nil
}
func (s *trackingIncidentStore) CountActiveByPriority(ctx context.Context, serviceID string) (map[string]int, error) {
	return nil, nil
}
func (s *trackingIncidentStore) ListActiveSummarizableIncidents(ctx context.Context) ([]store.IncidentRecord, error) {
	return nil, nil
}
func (s *trackingIncidentStore) ListActiveIncidents(ctx context.Context) ([]store.IncidentRecord, error) {
	return nil, nil
}
func (s *trackingIncidentStore) GetIncidentBySlackChannel(ctx context.Context, channelID string) (*store.IncidentRecord, error) {
	return nil, nil
}
func (s *trackingIncidentStore) SetIncidentWarRoomMeet(ctx context.Context, incidentNumber int64, spaceName, conferenceURL string) error {
	if s.byIncident != nil {
		if rec, ok := s.byIncident[incidentNumber]; ok {
			rec.GoogleMeetSpaceName = spaceName
			rec.ConferenceURL = conferenceURL
		}
	}
	return nil
}

type trackingPostMortemStore struct {
	byIncident           map[uuid.UUID]*store.PostMortemRecord
	getByIncidentIDCalls []uuid.UUID
	updateStatusCalls    []trackingPostMortemStatusCall
	created              []store.PostMortemRecord
}

type trackingPostMortemStatusCall struct {
	id         uuid.UUID
	status     string
	approvedBy *uuid.UUID
}

func (s *trackingPostMortemStore) Create(ctx context.Context, record *store.PostMortemRecord) (*store.PostMortemRecord, error) {
	if record.ID == uuid.Nil {
		record.ID = uuid.New()
	}
	s.created = append(s.created, *record)
	return record, nil
}

func (s *trackingPostMortemStore) GetByIncidentID(ctx context.Context, incidentID uuid.UUID) (*store.PostMortemRecord, error) {
	s.getByIncidentIDCalls = append(s.getByIncidentIDCalls, incidentID)
	if s.byIncident != nil {
		if rec, ok := s.byIncident[incidentID]; ok {
			cp := *rec
			return &cp, nil
		}
	}
	return nil, nil
}

func (s *trackingPostMortemStore) GetByID(ctx context.Context, id uuid.UUID) (*store.PostMortemRecord, error) {
	return nil, nil
}

func (s *trackingPostMortemStore) Update(ctx context.Context, id uuid.UUID, record *store.PostMortemRecord) (*store.PostMortemRecord, error) {
	return record, nil
}

func (s *trackingPostMortemStore) UpdateStatus(ctx context.Context, id uuid.UUID, status string, approvedBy *uuid.UUID) (*store.PostMortemRecord, error) {
	s.updateStatusCalls = append(s.updateStatusCalls, trackingPostMortemStatusCall{id: id, status: status, approvedBy: approvedBy})
	return &store.PostMortemRecord{ID: id, Status: status, ApprovedByID: approvedBy}, nil
}

func (s *trackingPostMortemStore) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (s *trackingPostMortemStore) List(ctx context.Context, filter store.PostMortemListFilter) ([]store.PostMortemRecord, int, error) {
	return nil, 0, nil
}

type trackingActionItemStore struct {
	byPostMortem            map[uuid.UUID][]store.ActionItemRecord
	listByPostMortemCalls   []uuid.UUID
	created                 []store.ActionItemRecord
	deleteByPostMortemCalls []uuid.UUID
}

func (s *trackingActionItemStore) Create(ctx context.Context, record *store.ActionItemRecord) (*store.ActionItemRecord, error) {
	if record.ID == uuid.Nil {
		record.ID = uuid.New()
	}
	s.created = append(s.created, *record)
	return record, nil
}

func (s *trackingActionItemStore) GetByID(ctx context.Context, id uuid.UUID) (*store.ActionItemRecord, error) {
	return nil, nil
}

func (s *trackingActionItemStore) ListByPostMortem(ctx context.Context, postMortemID uuid.UUID) ([]store.ActionItemRecord, error) {
	s.listByPostMortemCalls = append(s.listByPostMortemCalls, postMortemID)
	if s.byPostMortem != nil {
		items := s.byPostMortem[postMortemID]
		return append([]store.ActionItemRecord(nil), items...), nil
	}
	return nil, nil
}

func (s *trackingActionItemStore) ListOpen(ctx context.Context) ([]store.ActionItemRecord, error) {
	return nil, nil
}

func (s *trackingActionItemStore) ListOverdue(ctx context.Context) ([]store.ActionItemRecord, error) {
	return nil, nil
}

func (s *trackingActionItemStore) Update(ctx context.Context, id uuid.UUID, record *store.ActionItemRecord) (*store.ActionItemRecord, error) {
	return record, nil
}

func (s *trackingActionItemStore) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (s *trackingActionItemStore) DeleteByPostMortemID(ctx context.Context, postMortemID uuid.UUID) error {
	s.deleteByPostMortemCalls = append(s.deleteByPostMortemCalls, postMortemID)
	return nil
}

func TestEnsureIncidentInvestigationDoesNotAddTimelineEntry(t *testing.T) {
	incidentStore := &trackingIncidentStore{}
	srv, _ := newTestServer(nil)
	srv.SetIncidentStore(incidentStore)
	srv.incidentInvestigationStore = &mockIncidentInvestigationStore{}

	srv.ensureIncidentInvestigation(context.Background(), &store.IncidentRecord{IncidentNumber: 1})

	for _, entry := range incidentStore.timeline {
		if entry.EventType == "investigation_created" {
			t.Fatalf("unexpected investigation_created timeline entry: %#v", entry)
		}
	}
}

func TestCreateIncidentInvestigationDoesNotAddTimelineEntry(t *testing.T) {
	incidentStore := &trackingIncidentStore{byIncident: map[int64]*store.IncidentRecord{
		1: {ID: uuid.New(), IncidentNumber: 1, Status: "active"},
	}}
	srv, mux := newTestServer(nil)
	srv.SetIncidentStore(incidentStore)
	srv.incidentInvestigationStore = &mockIncidentInvestigationStore{}

	req := authRequest(http.MethodPost, "/api/v1/incidents/1/investigations", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, entry := range incidentStore.timeline {
		if entry.EventType == "investigation_created" {
			t.Fatalf("unexpected investigation_created timeline entry: %#v", entry)
		}
	}
}

func TestEnsurePostMortemDraftTimelineMessageOmitsPostMortemID(t *testing.T) {
	incidentStore := &trackingIncidentStore{}
	postMortemStore := &trackingPostMortemStore{}
	srv, _ := newTestServer(nil)
	srv.SetIncidentStore(incidentStore)
	srv.SetPostMortemStore(postMortemStore)

	srv.ensurePostMortemDraft(context.Background(), &store.IncidentRecord{ID: uuid.New(), IncidentNumber: 1, Summary: "summary"}, "")

	if len(postMortemStore.created) != 1 {
		t.Fatalf("created post-mortems = %d, want 1", len(postMortemStore.created))
	}
	for _, entry := range incidentStore.timeline {
		if entry.EventType == "postmortem_created" && entry.Message != "Post-mortem created" {
			t.Fatalf("postmortem timeline message = %q, want %q", entry.Message, "Post-mortem created")
		}
	}
}

func newTestServer(mockAlerts *mockStore) (*Server, *http.ServeMux) {
	if mockAlerts == nil {
		mockAlerts = &mockStore{}
	}
	userStore := &mockUserStore{users: []store.UserRecord{testAdminUser}}
	sessionStore := &mockSessionStore{
		sessions: map[string]*store.SessionRecord{
			"test-session-id": {ID: "test-session-id", UserID: testAdminUser.ID, ExpiresAt: time.Now().Add(24 * time.Hour)},
		},
	}
	auditStore := &mockAuditStore{}
	srv := NewServer(
		&config.Config{},
		mockAlerts,
		&mockWebhookTokenStore{tokens: map[string]store.WebhookTokenRecord{}},
		&mockAgentTokenStore{},
		userStore,
		sessionStore,
		auditStore,
		&mockIntegrationStore{},
		&mockRouteRulesStore{},
		24*time.Hour,
		nil,
		nil,
		nil,
		nil,
		func(*routing.Engine) {},
		NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute),
		NewRateLimiter(10, 20),
		&mockAlertInvestigationStore{},
		&mockIncidentInvestigationStore{},
		nil,
		nil,
		nil,
		nil,
	)
	mux := http.NewServeMux()
	srv.Register(mux)
	return srv, mux
}

func authRequest(method, path string, body *bytes.Buffer) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, body)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.AddCookie(&http.Cookie{Name: "alga_session", Value: "test-session-id"})
	req.AddCookie(&http.Cookie{Name: "alga_csrf", Value: "test-csrf-token"})
	req.Header.Set("X-CSRF-Token", "test-csrf-token")
	return req
}

func TestIntegrationsEndpoint(t *testing.T) {
	_, mux := newTestServer(nil)

	req := authRequest(http.MethodGet, "/api/v1/integrations", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := decodeResponse(t, rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if _, ok := payload["mattermost"]; !ok {
		t.Fatalf("expected mattermost key in response")
	}
	if _, ok := payload["slack"]; !ok {
		t.Fatalf("expected slack key in response")
	}
}

func TestTokensCreateAndList(t *testing.T) {
	_, mux := newTestServer(nil)

	createBody := bytes.NewBufferString(`{"name":"grafana-prod"}`)
	createReq := authRequest(http.MethodPost, "/api/v1/webhook-tokens", createBody)
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}

	listReq := authRequest(http.MethodGet, "/api/v1/webhook-tokens", nil)
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	var listed []map[string]any
	if err := decodeResponse(t, listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 token, got %d", len(listed))
	}
}

func TestRoutesPutAndGet(t *testing.T) {
	userStore := &mockUserStore{users: []store.UserRecord{testAdminUser}}
	sessionStore := &mockSessionStore{
		sessions: map[string]*store.SessionRecord{
			"test-session-id": {ID: "test-session-id", UserID: testAdminUser.ID, ExpiresAt: time.Now().Add(24 * time.Hour)},
		},
	}
	changed := false
	auditStore := &mockAuditStore{}
	routeStore := &mockRouteRulesStore{}
	srv := NewServer(
		&config.Config{},
		&mockStore{},
		&mockWebhookTokenStore{tokens: map[string]store.WebhookTokenRecord{}},
		&mockAgentTokenStore{},
		userStore,
		sessionStore,
		auditStore,
		&mockIntegrationStore{},
		routeStore,
		24*time.Hour,
		nil,
		nil,
		nil,
		nil,
		func(*routing.Engine) { changed = true },
		NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute),
		NewRateLimiter(10, 20),
		&mockAlertInvestigationStore{},
		&mockIncidentInvestigationStore{},
		nil,
		nil,
		nil,
		nil,
	)
	mux := http.NewServeMux()
	srv.Register(mux)

	putPayload := `{"routes":[{"match_mode":"all","conditions":[{"source":"labels","field":"severity","operator":"exact","value":"critical"},{"source":"labels","field":"team","operator":"exact","value":"backend"}],"targets":[{"provider":"slack","channel":"C123"}]}]}`
	putReq := authRequest(http.MethodPut, "/api/v1/routes", bytes.NewBufferString(putPayload))
	putRec := httptest.NewRecorder()
	mux.ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", putRec.Code, putRec.Body.String())
	}
	if !changed {
		t.Fatalf("expected onRoutesChanged to be called")
	}
	if len(routeStore.routes) != 1 ||
		len(routeStore.routes[0].Targets) != 1 ||
		routeStore.routes[0].Targets[0].Provider != "slack" ||
		routeStore.routes[0].Targets[0].Channel != "C123" {
		t.Fatalf("expected route store to hold slack rule, got %#v", routeStore.routes)
	}

	getReq := authRequest(http.MethodGet, "/api/v1/routes", nil)
	getRec := httptest.NewRecorder()
	mux.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}
	var got struct {
		Routes []config.RouteConfig `json:"routes"`
	}
	if err := decodeResponse(t, getRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if len(got.Routes) != 1 ||
		len(got.Routes[0].Targets) != 1 ||
		got.Routes[0].Targets[0].Provider != "slack" ||
		got.Routes[0].Targets[0].Channel != "C123" {
		t.Fatalf("unexpected GET body: %#v", got.Routes)
	}
}

func TestAlertsQueryIncludesFilters(t *testing.T) {
	storeMock := &mockStore{
		alerts: []store.AlertRecord{{Fingerprint: "abc", Status: "firing"}},
	}
	_, mux := newTestServer(storeMock)

	req := authRequest(http.MethodGet, "/api/v1/alerts?status=firing&provider=slack&severity=critical", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if storeMock.lastFilter["provider"] != "slack" {
		t.Fatalf("expected provider filter to be forwarded, got %#v", storeMock.lastFilter["provider"])
	}
	if storeMock.lastFilter["severity"] != "critical" {
		t.Fatalf("expected severity filter to be forwarded, got %#v", storeMock.lastFilter["severity"])
	}
}

func TestMethodNotAllowed(t *testing.T) {
	_, mux := newTestServer(nil)

	req := authRequest(http.MethodPut, "/api/v1/alerts", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestAlertByFingerprintNotFound(t *testing.T) {
	_, mux := newTestServer(&mockStore{byFP: map[string]store.AlertRecord{}})

	req := authRequest(http.MethodGet, "/api/v1/alerts/99999", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestTokenDeleteInvalidID(t *testing.T) {
	_, mux := newTestServer(nil)

	req := authRequest(http.MethodDelete, "/api/v1/webhook-tokens/not-an-object-id", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestRoutesPutValidationErrors(t *testing.T) {
	userStore := &mockUserStore{users: []store.UserRecord{testAdminUser}}
	sessionStore := &mockSessionStore{
		sessions: map[string]*store.SessionRecord{
			"test-session-id": {ID: "test-session-id", UserID: testAdminUser.ID, ExpiresAt: time.Now().Add(24 * time.Hour)},
		},
	}
	auditStore := &mockAuditStore{}
	srv := NewServer(
		&config.Config{},
		&mockStore{},
		&mockWebhookTokenStore{tokens: map[string]store.WebhookTokenRecord{}},
		&mockAgentTokenStore{},
		userStore,
		sessionStore,
		auditStore,
		&mockIntegrationStore{},
		&mockRouteRulesStore{},
		24*time.Hour,
		nil,
		nil,
		nil,
		nil,
		nil,
		NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute),
		NewRateLimiter(10, 20),
		&mockAlertInvestigationStore{},
		&mockIncidentInvestigationStore{},
		nil,
		nil,
		nil,
		nil,
	)
	mux := http.NewServeMux()
	srv.Register(mux)

	payloads := []string{
		`{"routes":[{"conditions":[{"source":"labels","field":"severity","operator":"exact","value":"critical"}],"targets":[{"provider":"pagerduty","channel":"team-a"}]}]}`,
		`{"routes":[{"conditions":[],"targets":[{"provider":"slack","channel":"C123"}]}]}`,
		`{"routes":[{"conditions":[{"source":"labels","field":"severity","operator":"exact","value":"critical"}],"targets":[{"provider":"slack","channel":"  "}]}]}`,
	}

	for _, body := range payloads {
		req := authRequest(http.MethodPut, "/api/v1/routes", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for body %s, got %d", body, rec.Code)
		}
	}
}

func TestRoutesPutEmptyAndSilenced(t *testing.T) {
	userStore := &mockUserStore{users: []store.UserRecord{testAdminUser}}
	sessionStore := &mockSessionStore{
		sessions: map[string]*store.SessionRecord{
			"test-session-id": {ID: "test-session-id", UserID: testAdminUser.ID, ExpiresAt: time.Now().Add(24 * time.Hour)},
		},
	}
	auditStore := &mockAuditStore{}
	srv := NewServer(
		&config.Config{},
		&mockStore{},
		&mockWebhookTokenStore{tokens: map[string]store.WebhookTokenRecord{}},
		&mockAgentTokenStore{},
		userStore,
		sessionStore,
		auditStore,
		&mockIntegrationStore{},
		&mockRouteRulesStore{},
		24*time.Hour,
		nil,
		nil,
		nil,
		nil,
		nil,
		NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute),
		NewRateLimiter(10, 20),
		&mockAlertInvestigationStore{},
		&mockIncidentInvestigationStore{},
		nil,
		nil,
		nil,
		nil,
	)
	mux := http.NewServeMux()
	srv.Register(mux)

	reqEmpty := authRequest(http.MethodPut, "/api/v1/routes", bytes.NewBufferString(`{"routes":[]}`))
	recEmpty := httptest.NewRecorder()
	mux.ServeHTTP(recEmpty, reqEmpty)
	if recEmpty.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty routes, got %d: %s", recEmpty.Code, recEmpty.Body.String())
	}

	reqSilenced := authRequest(http.MethodPut, "/api/v1/routes", bytes.NewBufferString(
		`{"routes":[{"silenced":true,"conditions":[{"source":"labels","field":"severity","operator":"exact","value":"critical"}]}]}`,
	))
	recSilenced := httptest.NewRecorder()
	mux.ServeHTTP(recSilenced, reqSilenced)
	if recSilenced.Code != http.StatusOK {
		t.Fatalf("expected 200 for silenced route without channel, got %d: %s", recSilenced.Code, recSilenced.Body.String())
	}

	reqTargets := authRequest(http.MethodPut, "/api/v1/routes", bytes.NewBufferString(
		`{"routes":[{"conditions":[{"source":"labels","field":"severity","operator":"exact","value":"critical"}],"targets":[{"provider":"mattermost","channel":"a"},{"provider":"slack","channel":"C1"}]}]}`,
	))
	recTargets := httptest.NewRecorder()
	mux.ServeHTTP(recTargets, reqTargets)
	if recTargets.Code != http.StatusOK {
		t.Fatalf("expected 200 for multi-target route, got %d: %s", recTargets.Code, recTargets.Body.String())
	}
}

func TestUnauthenticatedRequest(t *testing.T) {
	_, mux := newTestServer(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestLoginEndpoint(t *testing.T) {
	userStore := &mockUserStore{users: []store.UserRecord{testAdminUser}}
	sessionStore := &mockSessionStore{sessions: map[string]*store.SessionRecord{}}
	auditStore := &mockAuditStore{}
	srv := NewServer(
		&config.Config{},
		&mockStore{},
		&mockWebhookTokenStore{tokens: map[string]store.WebhookTokenRecord{}},
		&mockAgentTokenStore{},
		userStore,
		sessionStore,
		auditStore,
		&mockIntegrationStore{},
		&mockRouteRulesStore{},
		24*time.Hour,
		nil,
		nil,
		nil,
		nil,
		nil,
		NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute),
		NewRateLimiter(10, 20),
		&mockAlertInvestigationStore{},
		&mockIncidentInvestigationStore{},
		nil,
		nil,
		nil,
		nil,
	)
	mux := http.NewServeMux()
	srv.Register(mux)

	body := bytes.NewBufferString(`{"email":"admin@alga.local","password":"P@ssw0rd"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func agentAuthRequest(method, path string, body *bytes.Buffer, bearer string) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, body)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return req
}

// trackingInvestigationStore overrides the manual-create / link / unlink
// methods so HTTP-handler tests can assert what the underlying store saw.
type trackingAlertInvestigationStore struct {
	mockAlertInvestigationStore
	byID          map[string]*store.AlertInvestigationRecord
	byAlertNumber map[int64][]store.AlertInvestigationRecord
	created       []store.AlertInvestigationRecord
	statusUpdates map[string][]string
	updatesAdded  map[string][]store.InvestigationUpdate
	listCalls     []string
}

func (t *trackingAlertInvestigationStore) CreateAlertInvestigation(ctx context.Context, record store.AlertInvestigationRecord) (*store.AlertInvestigationRecord, error) {
	if record.ID == uuid.Nil {
		record.ID = uuid.New()
	}
	if record.AlertInvestigationID == "" {
		record.AlertInvestigationID = record.ID.String()
	}
	t.created = append(t.created, record)
	if t.byID == nil {
		t.byID = map[string]*store.AlertInvestigationRecord{}
	}
	stored := record
	t.byID[record.AlertInvestigationID] = &stored
	return &record, nil
}

func (t *trackingAlertInvestigationStore) GetAlertInvestigation(ctx context.Context, id string) (*store.AlertInvestigationRecord, error) {
	if t.byID == nil {
		return nil, nil
	}
	if rec, ok := t.byID[id]; ok {
		c := *rec
		return &c, nil
	}
	return nil, nil
}

func (t *trackingAlertInvestigationStore) ListAlertInvestigationsByAlertNumber(ctx context.Context, n int64) ([]store.AlertInvestigationRecord, error) {
	t.listCalls = append(t.listCalls, fmt.Sprintf("num:%d", n))
	if t.byAlertNumber != nil {
		if recs, ok := t.byAlertNumber[n]; ok {
			return recs, nil
		}
	}
	var out []store.AlertInvestigationRecord
	for _, rec := range t.byID {
		for _, a := range rec.Alerts {
			if a.AlertNumber == n {
				out = append(out, *rec)
				break
			}
		}
	}
	return out, nil
}

func (t *trackingAlertInvestigationStore) TransitionAlertInvestigationStatus(ctx context.Context, id string, fromStatus []string, toStatus string) error {
	if t.statusUpdates == nil {
		t.statusUpdates = map[string][]string{}
	}
	t.statusUpdates[id] = append(t.statusUpdates[id], toStatus)
	for key, rec := range t.byID {
		if rec.ID.String() == id || key == id {
			t.statusUpdates[key] = append(t.statusUpdates[key], toStatus)
			rec.Status = toStatus
		}
	}
	if t.byID != nil {
		if rec, ok := t.byID[id]; ok {
			rec.Status = toStatus
		}
	}
	return nil
}

func (t *trackingAlertInvestigationStore) UpdateAlertInvestigationAgent(ctx context.Context, id string, agentID string, agentName string, agentType string) error {
	if t.statusUpdates == nil {
		t.statusUpdates = map[string][]string{}
	}
	t.statusUpdates[id] = append(t.statusUpdates[id], "agent_cleared")
	if t.byID != nil {
		if rec, ok := t.byID[id]; ok {
			rec.AgentID = ""
			rec.AgentName = ""
			rec.AgentType = ""
		}
	}
	return nil
}

func (t *trackingAlertInvestigationStore) SetAlertInvestigationAssignee(ctx context.Context, id string, assigneeType string, assigneeID *uuid.UUID) error {
	if t.byID != nil {
		if rec, ok := t.byID[id]; ok {
			rec.AssigneeType = assigneeType
			rec.AssigneeID = assigneeID
		}
	}
	return nil
}

func (t *trackingAlertInvestigationStore) AddAlertInvestigationUpdate(ctx context.Context, id string, update store.InvestigationUpdate) error {
	if t.updatesAdded == nil {
		t.updatesAdded = map[string][]store.InvestigationUpdate{}
	}
	t.updatesAdded[id] = append(t.updatesAdded[id], update)
	if t.byID != nil {
		if rec, ok := t.byID[id]; ok {
			rec.Updates = append(rec.Updates, update)
		}
	}
	return nil
}

func (t *trackingAlertInvestigationStore) GetAlertInvestigationByAlertNumber(ctx context.Context, alertNumber int64) (*store.AlertInvestigationRecord, error) {
	if t.byAlertNumber != nil {
		if recs, ok := t.byAlertNumber[alertNumber]; ok && len(recs) > 0 {
			return &recs[0], nil
		}
	}
	for _, rec := range t.byID {
		for _, a := range rec.Alerts {
			if a.AlertNumber == alertNumber {
				return rec, nil
			}
		}
	}
	return nil, nil
}

func (t *trackingAlertInvestigationStore) GetCurrentAlertInvestigationByAlertNumber(ctx context.Context, alertNumber int64) (*store.AlertInvestigationRecord, error) {
	if t.byAlertNumber != nil {
		if recs, ok := t.byAlertNumber[alertNumber]; ok {
			for i := range recs {
				if !store.IsTerminalInvestigationStatus(recs[i].Status) {
					return &recs[i], nil
				}
			}
			return &recs[0], nil
		}
	}
	for _, rec := range t.byID {
		if store.IsTerminalInvestigationStatus(rec.Status) {
			continue
		}
		for _, a := range rec.Alerts {
			if a.AlertNumber == alertNumber {
				return rec, nil
			}
		}
	}
	return t.GetAlertInvestigationByAlertNumber(ctx, alertNumber)
}

func (t *trackingAlertInvestigationStore) GetCurrentAlertInvestigationSummariesByAlertNumbers(ctx context.Context, alertNumbers []int64) (map[int64]store.AlertInvestigationSummary, error) {
	out := make(map[int64]store.AlertInvestigationSummary, len(alertNumbers))
	for _, n := range alertNumbers {
		rec, err := t.GetCurrentAlertInvestigationByAlertNumber(ctx, n)
		if err != nil || rec == nil {
			continue
		}
		out[n] = store.AlertInvestigationSummary{
			AlertInvestigationID: rec.AlertInvestigationID,
			Status:               rec.Status,
			AgentID:              rec.AgentID,
			AgentName:            rec.AgentName,
			AgentType:            rec.AgentType,
		}
	}
	return out, nil
}

func (t *trackingAlertInvestigationStore) MarkAlertInvestigationAlertsCurrent(ctx context.Context, investigationID string, current bool) error {
	return nil
}

func (t *trackingAlertInvestigationStore) AppendAlertInvestigationEvent(ctx context.Context, investigationUUID uuid.UUID, event store.AlertInvestigationEvent) error {
	return nil
}

func (t *trackingAlertInvestigationStore) CompleteAlertInvestigation(ctx context.Context, id string, completion store.AlertInvestigationCompletion) error {
	if t.statusUpdates == nil {
		t.statusUpdates = map[string][]string{}
	}
	t.statusUpdates[id] = append(t.statusUpdates[id], store.AlertInvestigationStatusComplete)
	for key, rec := range t.byID {
		if rec.ID.String() == id || key == id {
			t.statusUpdates[key] = append(t.statusUpdates[key], store.AlertInvestigationStatusComplete)
			rec.Status = store.AlertInvestigationStatusComplete
		}
	}
	return nil
}

func (t *trackingAlertInvestigationStore) MarkAlertInvestigationPromoted(ctx context.Context, id string, incidentID string, incidentNumber int64, incidentInvestigationID string) (*store.AlertInvestigationRecord, error) {
	incidentUUID, _ := uuid.Parse(incidentID)
	incidentInvUUID, _ := uuid.Parse(incidentInvestigationID)
	for key, rec := range t.byID {
		if rec.ID.String() == id || key == id || rec.AlertInvestigationID == id {
			rec.Status = store.AlertInvestigationStatusPromoted
			rec.PromotedIncidentID = &incidentUUID
			rec.PromotedIncidentInvestigationID = &incidentInvUUID
			t.statusUpdates[key] = append(t.statusUpdates[key], store.AlertInvestigationStatusPromoted)
			return rec, nil
		}
	}
	return nil, nil
}

func (t *trackingAlertInvestigationStore) RequeueAlertInvestigation(ctx context.Context, id string, requeue store.AlertInvestigationRequeue) error {
	return nil
}

func (t *trackingAlertInvestigationStore) ListAlertInvestigations(ctx context.Context, filter map[string]any) ([]store.AlertInvestigationRecord, error) {
	if t.byID == nil {
		return nil, nil
	}
	incidentID, _ := filter["promoted_incident_id"].(string)
	out := make([]store.AlertInvestigationRecord, 0, len(t.byID))
	for _, rec := range t.byID {
		if incidentID != "" {
			if rec.PromotedIncidentID == nil || rec.PromotedIncidentID.String() != incidentID {
				continue
			}
		}
		out = append(out, *rec)
	}
	return out, nil
}

func newInvestigationTestServer(mockAlerts *mockStore, invStore store.AlertInvestigationStore) (*Server, *http.ServeMux) {
	if mockAlerts == nil {
		mockAlerts = &mockStore{}
	}
	userStore := &mockUserStore{users: []store.UserRecord{testAdminUser}}
	sessionStore := &mockSessionStore{
		sessions: map[string]*store.SessionRecord{
			"test-session-id": {ID: "test-session-id", UserID: testAdminUser.ID, ExpiresAt: time.Now().Add(24 * time.Hour)},
		},
	}
	srv := NewServer(
		&config.Config{},
		mockAlerts,
		&mockWebhookTokenStore{tokens: map[string]store.WebhookTokenRecord{}},
		&mockAgentTokenStore{},
		userStore,
		sessionStore,
		&mockAuditStore{},
		&mockIntegrationStore{},
		&mockRouteRulesStore{},
		24*time.Hour,
		nil,
		nil,
		nil,
		nil,
		func(*routing.Engine) {},
		NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute),
		NewRateLimiter(10, 20),
		invStore,
		&mockIncidentInvestigationStore{},
		nil,
		nil,
		nil,
		nil,
	)
	mux := http.NewServeMux()
	srv.Register(mux)
	return srv, mux
}

func TestAlertByFingerprintIncludesInvestigationsArray(t *testing.T) {
	alerts := &mockStore{
		byFP: map[string]store.AlertRecord{
			"fp1": {Fingerprint: "fp1", AlertNumber: 1},
		},
		byNumber: map[int64]store.AlertRecord{
			1: {Fingerprint: "fp1", AlertNumber: 1},
		},
	}
	inv := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-A": {
				AlertInvestigationID: "AINV-A",
				Status:               "investigating",
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp1", AlertNumber: 1}},
			},
		},
		byAlertNumber: map[int64][]store.AlertInvestigationRecord{
			1: {
				{AlertInvestigationID: "AINV-A", Status: "investigating"},
			},
		},
	}
	_, mux := newInvestigationTestServer(alerts, inv)

	req := authRequest(http.MethodGet, "/api/v1/alerts/1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload map[string]json.RawMessage
	if err := decodeResponse(t, rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := payload["alert_investigation"]; !ok {
		t.Fatalf("expected alert_investigation key, got: %s", rec.Body.String())
	}
}

func TestAlertRelatedReturnsPromotedIncidentByUUID(t *testing.T) {
	incidentUUID := uuid.New()
	alerts := &mockStore{
		byNumber: map[int64]store.AlertRecord{
			1: {Fingerprint: "fp1", AlertNumber: 1},
		},
	}
	inv := &trackingAlertInvestigationStore{
		byAlertNumber: map[int64][]store.AlertInvestigationRecord{
			1: {
				{
					AlertInvestigationID: "AINV-1",
					Status:               store.AlertInvestigationStatusInvestigating,
					PromotedIncidentID:   &incidentUUID,
					Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp1", AlertNumber: 1}},
				},
			},
		},
	}
	incidentStore := &trackingIncidentStore{
		byUUID: map[string]*store.IncidentRecord{
			incidentUUID.String(): {
				ID:             incidentUUID,
				IncidentNumber: 12,
				Title:          "Database outage",
				Status:         "active",
				Severity:       "critical",
				Priority:       "P1",
			},
		},
	}
	srv, mux := newInvestigationTestServer(alerts, inv)
	srv.incidentStore = incidentStore

	req := authRequest(http.MethodGet, "/api/v1/alerts/1/related", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Incident *struct {
			IncidentNumber int64  `json:"incident_number"`
			Title          string `json:"title"`
			Status         string `json:"status"`
		} `json:"incident"`
	}
	if err := decodeResponse(t, rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Incident == nil {
		t.Fatalf("expected related incident, got: %s", rec.Body.String())
	}
	if payload.Incident.IncidentNumber != 12 {
		t.Fatalf("unexpected incident payload: %#v", payload.Incident)
	}
}

func TestAlertRelatedReturnsDirectlyLinkedIncident(t *testing.T) {
	alerts := &mockStore{
		byNumber: map[int64]store.AlertRecord{
			24: {Fingerprint: "fp24", AlertNumber: 24},
		},
		incidentsByAlert: map[int64][]store.IncidentRecord{
			24: {
				{
					IncidentNumber: 12,
					Title:          "TestAlert",
					Status:         "detected",
					Severity:       "warning",
					Priority:       "P3",
				},
			},
		},
	}
	inv := &trackingAlertInvestigationStore{
		byAlertNumber: map[int64][]store.AlertInvestigationRecord{
			24: {},
		},
	}
	_, mux := newInvestigationTestServer(alerts, inv)

	req := authRequest(http.MethodGet, "/api/v1/alerts/24/related", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Incident *struct {
			IncidentNumber int64  `json:"incident_number"`
			Title          string `json:"title"`
			Status         string `json:"status"`
		} `json:"incident"`
	}
	if err := decodeResponse(t, rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Incident == nil {
		t.Fatalf("expected directly linked incident, got: %s", rec.Body.String())
	}
	if payload.Incident.IncidentNumber != 12 {
		t.Fatalf("unexpected incident payload: %#v", payload.Incident)
	}
}

func TestPostMortemRoutesResolvePublicIncidentID(t *testing.T) {
	incidentUUID := uuid.New()
	postMortemID := uuid.New()
	incidentStore := &trackingIncidentStore{
		byIncident: map[int64]*store.IncidentRecord{
			42: {
				ID:             incidentUUID,
				IncidentNumber: 42,
				Title:          "Public ID outage",
				Status:         "resolved",
			},
		},
	}
	postMortemStore := &trackingPostMortemStore{
		byIncident: map[uuid.UUID]*store.PostMortemRecord{
			incidentUUID: {
				ID:         postMortemID,
				IncidentID: incidentUUID,
				Status:     "draft",
				Summary:    "Investigated",
			},
		},
	}
	srv, mux := newTestServer(nil)
	srv.SetIncidentStore(incidentStore)
	srv.SetPostMortemStore(postMortemStore)

	req := authRequest(http.MethodGet, "/api/v1/incidents/42/post-mortem", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(postMortemStore.getByIncidentIDCalls) != 1 {
		t.Fatalf("expected one post-mortem lookup, got %d", len(postMortemStore.getByIncidentIDCalls))
	}
	if got := postMortemStore.getByIncidentIDCalls[0]; got != incidentUUID {
		t.Fatalf("expected post-mortem lookup by internal uuid %s, got %s", incidentUUID, got)
	}

	var payload store.PostMortemRecord
	if err := decodeResponse(t, rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.ID != postMortemID || payload.IncidentID != incidentUUID {
		t.Fatalf("unexpected post-mortem payload: %#v", payload)
	}
}

func TestPostMortemSubmitReviewResolvesPublicIncidentID(t *testing.T) {
	incidentUUID := uuid.New()
	postMortemID := uuid.New()
	incidentStore := &trackingIncidentStore{
		byIncident: map[int64]*store.IncidentRecord{
			42: {
				ID:             incidentUUID,
				IncidentNumber: 42,
				Title:          "Public ID outage",
				Status:         "resolved",
			},
		},
	}
	postMortemStore := &trackingPostMortemStore{
		byIncident: map[uuid.UUID]*store.PostMortemRecord{
			incidentUUID: {
				ID:         postMortemID,
				IncidentID: incidentUUID,
				Status:     "draft",
			},
		},
	}
	srv, mux := newTestServer(nil)
	srv.SetIncidentStore(incidentStore)
	srv.SetPostMortemStore(postMortemStore)

	req := authRequest(http.MethodPost, "/api/v1/incidents/42/post-mortem/submit-review", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(postMortemStore.getByIncidentIDCalls) != 1 {
		t.Fatalf("expected one post-mortem lookup, got %d", len(postMortemStore.getByIncidentIDCalls))
	}
	if got := postMortemStore.getByIncidentIDCalls[0]; got != incidentUUID {
		t.Fatalf("expected post-mortem lookup by internal uuid %s, got %s", incidentUUID, got)
	}
	if len(postMortemStore.updateStatusCalls) != 1 {
		t.Fatalf("expected one status update, got %d", len(postMortemStore.updateStatusCalls))
	}
	call := postMortemStore.updateStatusCalls[0]
	if call.id != postMortemID || call.status != "in_review" || call.approvedBy != nil {
		t.Fatalf("unexpected status update call: %#v", call)
	}

	var payload store.PostMortemRecord
	if err := decodeResponse(t, rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.ID != postMortemID || payload.Status != "in_review" {
		t.Fatalf("unexpected post-mortem payload: %#v", payload)
	}
}

func TestPostMortemActionItemListResolvesPublicIncidentID(t *testing.T) {
	incidentUUID := uuid.New()
	postMortemID := uuid.New()
	actionItemID := uuid.New()
	incidentStore := &trackingIncidentStore{
		byIncident: map[int64]*store.IncidentRecord{
			42: {ID: incidentUUID, IncidentNumber: 42},
		},
	}
	postMortemStore := &trackingPostMortemStore{
		byIncident: map[uuid.UUID]*store.PostMortemRecord{
			incidentUUID: {ID: postMortemID, IncidentID: incidentUUID, Status: "draft"},
		},
	}
	actionItemStore := &trackingActionItemStore{
		byPostMortem: map[uuid.UUID][]store.ActionItemRecord{
			postMortemID: {{ID: actionItemID, PostMortemID: postMortemID, Description: "Follow up", Status: "detected"}},
		},
	}
	srv, mux := newTestServer(nil)
	srv.SetIncidentStore(incidentStore)
	srv.SetPostMortemStore(postMortemStore)
	srv.SetActionItemStore(actionItemStore)

	req := authRequest(http.MethodGet, "/api/v1/incidents/42/post-mortem/action-items", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(actionItemStore.listByPostMortemCalls) != 1 || actionItemStore.listByPostMortemCalls[0] != postMortemID {
		t.Fatalf("expected action-item list by post-mortem %s, got %#v", postMortemID, actionItemStore.listByPostMortemCalls)
	}

	var payload []store.ActionItemRecord
	if err := decodeResponse(t, rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload) != 1 || payload[0].ID != actionItemID || payload[0].PostMortemID != postMortemID {
		t.Fatalf("unexpected action items payload: %#v", payload)
	}
}

func TestPostMortemActionItemCreateResolvesPublicIncidentID(t *testing.T) {
	incidentUUID := uuid.New()
	postMortemID := uuid.New()
	incidentStore := &trackingIncidentStore{
		byIncident: map[int64]*store.IncidentRecord{
			42: {ID: incidentUUID, IncidentNumber: 42},
		},
	}
	postMortemStore := &trackingPostMortemStore{
		byIncident: map[uuid.UUID]*store.PostMortemRecord{
			incidentUUID: {ID: postMortemID, IncidentID: incidentUUID, Status: "draft"},
		},
	}
	actionItemStore := &trackingActionItemStore{}
	srv, mux := newTestServer(nil)
	srv.SetIncidentStore(incidentStore)
	srv.SetPostMortemStore(postMortemStore)
	srv.SetActionItemStore(actionItemStore)

	body := bytes.NewBufferString(`{"description":"Follow up","priority":"P2","type":"task"}`)
	req := authRequest(http.MethodPost, "/api/v1/incidents/42/post-mortem/action-items", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(actionItemStore.created) != 1 {
		t.Fatalf("expected one created action item, got %d", len(actionItemStore.created))
	}
	if actionItemStore.created[0].PostMortemID != postMortemID {
		t.Fatalf("expected created action item for post-mortem %s, got %s", postMortemID, actionItemStore.created[0].PostMortemID)
	}
}

func TestPostMortemActionItemPublicIDMissingIncidentStoreReturnsUnavailable(t *testing.T) {
	srv, mux := newTestServer(nil)
	srv.SetPostMortemStore(&trackingPostMortemStore{})
	srv.SetActionItemStore(&trackingActionItemStore{})

	req := authRequest(http.MethodGet, "/api/v1/incidents/42/post-mortem/action-items", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateIncidentFromAlertQueuesPromotedAlertInvestigation(t *testing.T) {
	alerts := &mockStore{
		byNumber: map[int64]store.AlertRecord{
			42: {
				Fingerprint: "fp42",
				AlertNumber: 42,
				Status:      "firing",
				Labels:      map[string]string{"alertname": "HighCPU"},
				Annotations: map[string]string{"summary": "CPU is high"},
				StartsAt:    time.Now().UTC(),
			},
		},
	}
	inv := &trackingAlertInvestigationStore{}
	incidentStore := &trackingIncidentStore{nextID: "12", nextNumber: 12}
	srv, mux := newInvestigationTestServer(alerts, inv)
	srv.incidentStore = incidentStore

	body := bytes.NewBufferString(`{"title":"HighCPU","severity":"critical","alert_numbers":[42]}`)
	req := authRequest(http.MethodPost, "/api/v1/incidents", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(incidentStore.created) != 1 {
		t.Fatalf("created incidents = %d, want 1", len(incidentStore.created))
	}
	created := incidentStore.created[0]
	if created.Status != "active" {
		t.Fatalf("incident status = %q, want active", created.Status)
	}
	if created.Title != "HighCPU" {
		t.Fatalf("incident title = %q, want alert title", created.Title)
	}
	if created.Description != "CPU is high" {
		t.Fatalf("incident description = %q, want alert summary", created.Description)
	}
	if len(inv.created) != 1 {
		t.Fatalf("created alert investigations = %d, want 1", len(inv.created))
	}
	queued := inv.created[0]
	if queued.Status != store.AlertInvestigationStatusPending {
		t.Fatalf("investigation status = %q, want pending", queued.Status)
	}
	if queued.PromotedIncidentID == nil || *queued.PromotedIncidentID != created.ID {
		t.Fatalf("promoted incident id = %#v, want %s", queued.PromotedIncidentID, created.ID)
	}
	if queued.PrimaryAlertNumber != 42 || len(queued.Alerts) != 1 || queued.Alerts[0].Fingerprint != "fp42" {
		t.Fatalf("queued alert payload = %#v", queued)
	}
}

func TestCreateIncidentFromAlertPostsAlertThreadHandoff(t *testing.T) {
	alerts := &mockStore{
		byNumber: map[int64]store.AlertRecord{
			42: {
				Fingerprint: "fp42",
				AlertNumber: 42,
				Status:      "firing",
				Labels:      map[string]string{"alertname": "HighCPU"},
				StartsAt:    time.Now().UTC(),
			},
		},
	}
	inv := &trackingAlertInvestigationStore{}
	incidentStore := &trackingIncidentStore{nextID: "12", nextNumber: 12}
	threadStore := newMemoryInvestigationThreadStore()
	srv, mux := newInvestigationTestServer(alerts, inv)
	srv.incidentStore = incidentStore
	srv.investigationThreadStore = threadStore

	body := bytes.NewBufferString(`{"title":"HighCPU","severity":"critical","alert_numbers":[42]}`)
	req := authRequest(http.MethodPost, "/api/v1/incidents", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	thread, _, err := threadStore.GetThreadByOwner(context.Background(), store.ThreadOwnerAlert, "42", 20, 0)
	if err != nil {
		t.Fatalf("alert thread missing: %v", err)
	}
	if len(thread.Messages) != 1 {
		t.Fatalf("thread messages = %d, want 1", len(thread.Messages))
	}
	msg := thread.Messages[0]
	if msg.Type != "action" || msg.Source != "system" {
		t.Fatalf("thread message type/source = %q/%q, want action/system", msg.Type, msg.Source)
	}
	if !strings.Contains(msg.Message, "](/incidents/12)") {
		t.Fatalf("thread message should include incident route link, got %q", msg.Message)
	}
}

func TestAlertReopenReopensLatestLinkedTerminalOnly(t *testing.T) {
	alerts := &trackingAlertStore{
		mockStore: mockStore{
			byFP: map[string]store.AlertRecord{
				"fp1": {Fingerprint: "fp1", Status: "resolved", AlertNumber: 1},
			},
			byNumber: map[int64]store.AlertRecord{
				1: {Fingerprint: "fp1", Status: "resolved", AlertNumber: 1},
			},
		},
	}
	inv := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-NEW": {
				ID:                   uuid.MustParse("00000000-0000-0000-0000-000000000001"),
				AlertInvestigationID: "AINV-NEW",
				Status:               "complete",
				AgentID:              "agent-new",
				AgentName:            "agent-new",
				AgentType:            "hermes",
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp1", AlertNumber: 1}},
			},
			"AINV-OLD": {
				ID:                   uuid.MustParse("00000000-0000-0000-0000-000000000002"),
				AlertInvestigationID: "AINV-OLD",
				Status:               "complete",
				AgentID:              "agent-old",
				AgentName:            "agent-old",
				AgentType:            "hermes",
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp1", AlertNumber: 1}},
			},
		},
		byAlertNumber: map[int64][]store.AlertInvestigationRecord{
			1: {
				{ID: uuid.MustParse("00000000-0000-0000-0000-000000000001"), AlertInvestigationID: "AINV-NEW", Status: "complete", AgentID: "agent-new", AgentName: "agent-new", AgentType: "hermes"},
				{ID: uuid.MustParse("00000000-0000-0000-0000-000000000002"), AlertInvestigationID: "AINV-OLD", Status: "complete", AgentID: "agent-old", AgentName: "agent-old", AgentType: "hermes"},
			},
		},
	}
	srv, mux := newInvestigationTestServer(&alerts.mockStore, inv)
	forwarder := &trackingInvestigationForwarder{}
	srv.SetInvestigationForwarder(forwarder)
	srv.alertStore = alerts

	req := authRequest(http.MethodPost, "/api/v1/alerts/1/reopen", bytes.NewBufferString(`{}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if len(alerts.reopened) != 1 || alerts.reopened[0] != "fp1" {
		t.Fatalf("expected alert fp1 reopened once, got %#v", alerts.reopened)
	}
	if len(alerts.acknowledged) != 1 || alerts.acknowledged[0] != "fp1" {
		t.Fatalf("expected reopened alert fp1 auto-acknowledged once, got %#v", alerts.acknowledged)
	}
	if len(inv.listCalls) == 0 {
		t.Fatalf("expected linked investigations query on reopen")
	}
	if got := inv.statusUpdates["AINV-NEW"]; len(got) == 0 || got[len(got)-1] != "investigating" {
		t.Fatalf("expected latest investigation reopened, got %#v (all status updates: %#v)", got, inv.statusUpdates)
	}
	if gotOld := inv.statusUpdates["AINV-OLD"]; len(gotOld) != 0 {
		t.Fatalf("expected old investigation untouched, got %#v", gotOld)
	}
	if msgs := forwarder.messages["event:agent-new"]; len(msgs) == 0 {
		t.Fatalf("expected investigation_resume signal forwarded for latest investigation")
	}
	if msgs := forwarder.messages["event:agent-old"]; len(msgs) != 0 {
		t.Fatalf("expected no forwarded signal for old investigation, got %#v", msgs)
	}
}

func TestAgentAlertReopenReopensLatestLinkedTerminalOnly(t *testing.T) {
	alerts := &trackingAlertStore{
		mockStore: mockStore{
			byFP: map[string]store.AlertRecord{
				"fp1": {Fingerprint: "fp1", Status: "resolved", AlertNumber: 1, Labels: map[string]string{"alertname": "HighErrorRate"}},
			},
			byNumber: map[int64]store.AlertRecord{
				1: {Fingerprint: "fp1", Status: "resolved", AlertNumber: 1, Labels: map[string]string{"alertname": "HighErrorRate"}},
			},
		},
	}
	inv := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-NEW": {
				ID:                   uuid.MustParse("00000000-0000-0000-0000-000000000003"),
				AlertInvestigationID: "AINV-NEW",
				Status:               "complete",
				AgentID:              "agent-new",
				AgentName:            "agent-new",
				AgentType:            "hermes",
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp1", AlertNumber: 1}},
			},
			"AINV-OLD": {
				ID:                   uuid.MustParse("00000000-0000-0000-0000-000000000004"),
				AlertInvestigationID: "AINV-OLD",
				Status:               "complete",
				AgentID:              "agent-old",
				AgentName:            "agent-old",
				AgentType:            "hermes",
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp1", AlertNumber: 1}},
			},
		},
		byAlertNumber: map[int64][]store.AlertInvestigationRecord{
			1: {
				{ID: uuid.MustParse("00000000-0000-0000-0000-000000000003"), AlertInvestigationID: "AINV-NEW", Status: "complete", AgentID: "agent-new", AgentName: "agent-new", AgentType: "hermes"},
				{ID: uuid.MustParse("00000000-0000-0000-0000-000000000004"), AlertInvestigationID: "AINV-OLD", Status: "complete", AgentID: "agent-old", AgentName: "agent-old", AgentType: "hermes"},
			},
		},
	}
	agentTok := &testAgentTokenStore{
		validToken: "secret-agent-token",
		agentID:    uuid.New(),
		name:       "test-agent",
	}
	userStore := &mockUserStore{users: []store.UserRecord{testAdminUser}}
	sessionStore := &mockSessionStore{
		sessions: map[string]*store.SessionRecord{
			"test-session-id": {ID: "test-session-id", UserID: testAdminUser.ID, ExpiresAt: time.Now().Add(24 * time.Hour)},
		},
	}
	srv := NewServer(
		&config.Config{},
		&alerts.mockStore,
		&mockWebhookTokenStore{tokens: map[string]store.WebhookTokenRecord{}},
		agentTok,
		userStore,
		sessionStore,
		&mockAuditStore{},
		&mockIntegrationStore{},
		&mockRouteRulesStore{},
		24*time.Hour,
		nil,
		nil,
		nil,
		nil,
		func(*routing.Engine) {},
		NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute),
		NewRateLimiter(10, 20),
		inv,
		&mockIncidentInvestigationStore{},
		nil,
		nil,
		nil,
		nil,
	)
	srv.alertStore = alerts
	forwarder := &trackingInvestigationForwarder{}
	srv.SetInvestigationForwarder(forwarder)
	executor := agent.NewAgentToolExecutor(inv, nil, nil, nil, nil)
	srv.SetAgentService(agent.NewService(
		nil, executor, nil, srv.agentTokenStore, nil, nil, nil, nil,
		platform.AuthDeps{}, platform.AgentRateLimitDeps{}, nil,
		agent.WithAlertStores(alerts, nil, nil, nil, nil),
		agent.WithReopenAlert(srv.AgentReopenAlertFn()),
	))

	mux := http.NewServeMux()
	srv.Register(mux)

	req := agentAuthRequest(http.MethodPost, "/api/v1/agent/alerts/fp1/reopen", bytes.NewBufferString(`{}`), agentTok.validToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if len(alerts.reopened) != 1 || alerts.reopened[0] != "fp1" {
		t.Fatalf("expected alert fp1 reopened once, got %#v", alerts.reopened)
	}
	if len(inv.listCalls) == 0 {
		t.Fatalf("expected linked investigations query on reopen")
	}
	if got := inv.statusUpdates["AINV-NEW"]; len(got) == 0 || got[len(got)-1] != "investigating" {
		t.Fatalf("expected latest investigation reopened, got %#v (all status updates: %#v)", got, inv.statusUpdates)
	}
	if gotOld := inv.statusUpdates["AINV-OLD"]; len(gotOld) != 0 {
		t.Fatalf("expected old investigation untouched, got %#v", gotOld)
	}
	if msgs := forwarder.messages["event:agent-new"]; len(msgs) == 0 || msgs[len(msgs)-1] != "investigation_resume" {
		t.Fatalf("expected investigation_resume signal forwarded for latest investigation, got %#v", msgs)
	}
	if msgs := forwarder.messages["event:agent-old"]; len(msgs) != 0 {
		t.Fatalf("expected no forwarded signal for old investigation, got %#v", msgs)
	}
}

func TestAgentAlertReopenReopensPausedInvestigation(t *testing.T) {
	alerts := &trackingAlertStore{
		mockStore: mockStore{
			byFP: map[string]store.AlertRecord{
				"fp1": {Fingerprint: "fp1", Status: "resolved", AlertNumber: 1, Labels: map[string]string{"alertname": "HighErrorRate"}},
			},
			byNumber: map[int64]store.AlertRecord{
				1: {Fingerprint: "fp1", Status: "resolved", AlertNumber: 1, Labels: map[string]string{"alertname": "HighErrorRate"}},
			},
		},
	}
	inv := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-PAUSED": {
				ID:                   uuid.MustParse("00000000-0000-0000-0000-000000000005"),
				AlertInvestigationID: "AINV-PAUSED",
				Status:               "paused",
				AgentID:              "agent-paused",
				AgentName:            "agent-paused",
				AgentType:            "hermes",
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp1", AlertNumber: 1}},
			},
		},
		byAlertNumber: map[int64][]store.AlertInvestigationRecord{
			1: {
				{ID: uuid.MustParse("00000000-0000-0000-0000-000000000005"), AlertInvestigationID: "AINV-PAUSED", Status: "paused", AgentID: "agent-paused", AgentName: "agent-paused", AgentType: "hermes"},
			},
		},
	}
	agentTok := &testAgentTokenStore{
		validToken: "secret-agent-token",
		agentID:    uuid.New(),
		name:       "test-agent",
	}
	userStore := &mockUserStore{users: []store.UserRecord{testAdminUser}}
	sessionStore := &mockSessionStore{
		sessions: map[string]*store.SessionRecord{
			"test-session-id": {ID: "test-session-id", UserID: testAdminUser.ID, ExpiresAt: time.Now().Add(24 * time.Hour)},
		},
	}
	srv := NewServer(
		&config.Config{},
		&alerts.mockStore,
		&mockWebhookTokenStore{tokens: map[string]store.WebhookTokenRecord{}},
		agentTok,
		userStore,
		sessionStore,
		&mockAuditStore{},
		&mockIntegrationStore{},
		&mockRouteRulesStore{},
		24*time.Hour,
		nil,
		nil,
		nil,
		nil,
		func(*routing.Engine) {},
		NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute),
		NewRateLimiter(10, 20),
		inv,
		&mockIncidentInvestigationStore{},
		nil,
		nil,
		nil,
		nil,
	)
	srv.alertStore = alerts
	forwarder := &trackingInvestigationForwarder{}
	srv.SetInvestigationForwarder(forwarder)
	executor := agent.NewAgentToolExecutor(inv, nil, nil, nil, nil)
	srv.SetAgentService(agent.NewService(
		nil, executor, nil, srv.agentTokenStore, nil, nil, nil, nil,
		platform.AuthDeps{}, platform.AgentRateLimitDeps{}, nil,
		agent.WithAlertStores(alerts, nil, nil, nil, nil),
		agent.WithReopenAlert(srv.AgentReopenAlertFn()),
	))

	mux := http.NewServeMux()
	srv.Register(mux)

	req := agentAuthRequest(http.MethodPost, "/api/v1/agent/alerts/fp1/reopen", bytes.NewBufferString(`{}`), agentTok.validToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if len(alerts.reopened) != 1 || alerts.reopened[0] != "fp1" {
		t.Fatalf("expected alert fp1 reopened once, got %#v", alerts.reopened)
	}
	if len(alerts.acknowledged) != 1 || alerts.acknowledged[0] != "fp1" {
		t.Fatalf("expected reopened alert fp1 auto-acknowledged once, got %#v", alerts.acknowledged)
	}
	if got := inv.statusUpdates["AINV-PAUSED"]; len(got) == 0 || got[len(got)-1] != "investigating" {
		t.Fatalf("expected paused investigation reopened, got %#v", got)
	}
	if msgs := forwarder.messages["event:agent-paused"]; len(msgs) == 0 || msgs[len(msgs)-1] != "investigation_resume" {
		t.Fatalf("expected investigation_resume signal for paused investigation, got %#v", msgs)
	}
}

func TestAgentAlertRoutesRequireInvestigateOrCommandCapability(t *testing.T) {
	agentTok := &testAgentTokenStore{
		validToken:   "secret-agent-token",
		agentID:      uuid.New(),
		name:         "communicator-agent",
		capabilities: []string{capability.Communicate},
	}
	userStore := &mockUserStore{users: []store.UserRecord{testAdminUser}}
	sessionStore := &mockSessionStore{
		sessions: map[string]*store.SessionRecord{
			"test-session-id": {ID: "test-session-id", UserID: testAdminUser.ID, ExpiresAt: time.Now().Add(24 * time.Hour)},
		},
	}
	srv := NewServer(
		&config.Config{},
		&mockStore{},
		&mockWebhookTokenStore{tokens: map[string]store.WebhookTokenRecord{}},
		agentTok,
		userStore,
		sessionStore,
		&mockAuditStore{},
		&mockIntegrationStore{},
		&mockRouteRulesStore{},
		24*time.Hour,
		nil,
		nil,
		nil,
		nil,
		func(*routing.Engine) {},
		NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute),
		NewRateLimiter(10, 20),
		&mockAlertInvestigationStore{},
		&mockIncidentInvestigationStore{},
		nil,
		nil,
		nil,
		nil,
	)
	srv.SetAgentService(agent.NewService(
		nil, nil, nil, srv.agentTokenStore, nil, nil, nil, nil,
		platform.AuthDeps{}, platform.AgentRateLimitDeps{}, nil,
		agent.WithAlertStores(srv.alertStore, nil, nil, nil, nil),
	))
	mux := http.NewServeMux()
	srv.Register(mux)

	req := agentAuthRequest(http.MethodGet, "/api/v1/agent/alerts", nil, agentTok.validToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for agent without investigate capability, got %d: %s", rec.Code, rec.Body.String())
	}

	req = agentAuthRequest(http.MethodGet, "/api/v1/agent/alerts/fp1", nil, agentTok.validToken)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for agent alert detail without investigate capability, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCommandCapabilityCanAccessAgentAlertRoutes(t *testing.T) {
	agentTok := &testAgentTokenStore{
		validToken:   "secret-agent-token",
		agentID:      uuid.New(),
		name:         "commander-agent",
		capabilities: []string{capability.Command},
	}
	userStore := &mockUserStore{users: []store.UserRecord{testAdminUser}}
	sessionStore := &mockSessionStore{
		sessions: map[string]*store.SessionRecord{
			"test-session-id": {ID: "test-session-id", UserID: testAdminUser.ID, ExpiresAt: time.Now().Add(24 * time.Hour)},
		},
	}
	srv := NewServer(
		&config.Config{},
		&mockStore{},
		&mockWebhookTokenStore{tokens: map[string]store.WebhookTokenRecord{}},
		agentTok,
		userStore,
		sessionStore,
		&mockAuditStore{},
		&mockIntegrationStore{},
		&mockRouteRulesStore{},
		24*time.Hour,
		nil,
		nil,
		nil,
		nil,
		func(*routing.Engine) {},
		NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute),
		NewRateLimiter(10, 20),
		&mockAlertInvestigationStore{},
		&mockIncidentInvestigationStore{},
		nil,
		nil,
		nil,
		nil,
	)
	srv.SetAgentService(agent.NewService(
		nil, nil, nil, srv.agentTokenStore, nil, nil, nil, nil,
		platform.AuthDeps{}, platform.AgentRateLimitDeps{}, nil,
		agent.WithAlertStores(srv.alertStore, nil, nil, nil, nil),
	))
	mux := http.NewServeMux()
	srv.Register(mux)

	req := agentAuthRequest(http.MethodGet, "/api/v1/agent/alerts", nil, agentTok.validToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("command-capability agent must be allowed to list alerts, got 403: %s", rec.Body.String())
	}

	req = agentAuthRequest(http.MethodGet, "/api/v1/agent/alerts/fp1", nil, agentTok.validToken)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusForbidden {
		t.Fatalf("command-capability agent must be allowed to view alert detail, got 403: %s", rec.Body.String())
	}
}

func TestCommandOnlyCommanderCanResolveIncidentThroughAgentMessages(t *testing.T) {
	agentID := uuid.New()
	agentTok := &testAgentTokenStore{
		validToken:   "secret-agent-token",
		agentID:      agentID,
		name:         "commander-agent",
		capabilities: []string{capability.Command},
	}
	userStore := &mockUserStore{users: []store.UserRecord{testAdminUser}}
	sessionStore := &mockSessionStore{
		sessions: map[string]*store.SessionRecord{
			"test-session-id": {ID: "test-session-id", UserID: testAdminUser.ID, ExpiresAt: time.Now().Add(24 * time.Hour)},
		},
	}
	srv := NewServer(
		&config.Config{},
		&mockStore{},
		&mockWebhookTokenStore{tokens: map[string]store.WebhookTokenRecord{}},
		agentTok,
		userStore,
		sessionStore,
		&mockAuditStore{},
		&mockIntegrationStore{},
		&mockRouteRulesStore{},
		24*time.Hour,
		nil,
		nil,
		nil,
		nil,
		func(*routing.Engine) {},
		NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute),
		NewRateLimiter(10, 20),
		nil,
		&mockIncidentInvestigationStore{},
		nil,
		nil,
		nil,
		nil,
	)
	incidentStore := &incidentStoreSpy{transitionTo: "active"}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{
			RoleType:     string(ics.RoleIncidentCommander),
			AssigneeType: "agent",
			AgentTokenID: &agentID,
			Status:       string(ics.RoleStatusActive),
		},
	}}
	docStore := &mockIncidentDocumentStore{sections: []store.IncidentDocumentRecord{
		{Section: "impact_assessment", Content: "Test impact", Version: 1},
		{Section: "actions_taken", Content: "Test action", Version: 1},
		{Section: "root_cause", Content: "Test root cause", Version: 1},
		{Section: "resolution", Content: "Test resolution", Version: 1},
	}}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetICSRoleStore(roleStore)
	executor.SetIncidentDocumentStore(docStore)
	srv.SetAgentService(agent.NewService(nil, executor, nil, srv.agentTokenStore, nil, nil, nil, nil, platform.AuthDeps{}, platform.AgentRateLimitDeps{}, nil))

	mux := http.NewServeMux()
	srv.Register(mux)

	body := bytes.NewBufferString(`{"chat_id":"incident_coord_23","kind":"inv_tool","command":{"op":"resolve_incident","reason":"fixed"}}`)
	req := agentAuthRequest(http.MethodPost, "/api/v1/agent/messages", body, agentTok.validToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if incidentStore.transitionTo != "resolved" {
		t.Fatalf("incident transition = %q, want resolved", incidentStore.transitionTo)
	}
}

func TestCommanderWithoutInvestigateCanResolveLinkedAlertThroughAgentMessages(t *testing.T) {
	agentID := uuid.New()
	agentTok := &testAgentTokenStore{
		validToken:   "secret-agent-token",
		agentID:      agentID,
		name:         "commander-agent",
		capabilities: []string{capability.Command}, // No investigate capability!
	}
	userStore := &mockUserStore{users: []store.UserRecord{testAdminUser}}
	sessionStore := &mockSessionStore{
		sessions: map[string]*store.SessionRecord{
			"test-session-id": {ID: "test-session-id", UserID: testAdminUser.ID, ExpiresAt: time.Now().Add(24 * time.Hour)},
		},
	}
	alerts := &resolvingAlertStore{mockStore: mockStore{
		byFP:     map[string]store.AlertRecord{"fp-1": {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1}},
		byNumber: map[int64]store.AlertRecord{1: {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1}},
	}}

	incidentID := uuid.New()
	invUUID := uuid.New()
	invStore := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-1": {
				ID:                   invUUID,
				AlertInvestigationID: "AINV-1",
				Status:               "investigating",
				AgentID:              agentID.String(),
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
				PromotedIncidentID:   &incidentID,
			},
		},
		byAlertNumber: map[int64][]store.AlertInvestigationRecord{
			1: {
				{
					ID:                   invUUID,
					AlertInvestigationID: "AINV-1",
					Status:               "investigating",
					AgentID:              agentID.String(),
					Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp-1", AlertNumber: 1}},
					PromotedIncidentID:   &incidentID,
				},
			},
		},
	}

	srv := NewServer(
		&config.Config{},
		alerts,
		&mockWebhookTokenStore{tokens: map[string]store.WebhookTokenRecord{}},
		agentTok,
		userStore,
		sessionStore,
		&mockAuditStore{},
		&mockIntegrationStore{},
		&mockRouteRulesStore{},
		24*time.Hour,
		nil,
		nil,
		nil,
		nil,
		func(*routing.Engine) {},
		NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute),
		NewRateLimiter(10, 20),
		invStore,
		&mockIncidentInvestigationStore{},
		nil,
		nil,
		nil,
		nil,
	)
	srv.alertStore = alerts

	incidentStore := &incidentStoreSpy{transitionTo: "active"}
	roleStore := &mockICSRoleStore{roles: []store.ICSRoleRecord{
		{
			RoleType:     string(ics.RoleIncidentCommander),
			AssigneeType: "agent",
			AgentTokenID: &agentID,
			Status:       string(ics.RoleStatusActive),
		},
	}}

	executor := agent.NewAgentToolExecutor(invStore, nil, nil, nil, nil)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: alerts})
	executor.SetIncidentStore(incidentStore)
	executor.SetIncidentInvestigationStore(&spyIncidentInvestigationStore{})
	executor.SetICSRoleStore(roleStore)
	executor.SetAlertInvestigationLifecycleService(NewAlertInvestigationLifecycleService(alerts, invStore, nil, nil, nil))
	srv.SetAgentService(agent.NewService(nil, executor, nil, srv.agentTokenStore, nil, nil, nil, nil, platform.AuthDeps{}, platform.AgentRateLimitDeps{}, nil))

	mux := http.NewServeMux()
	srv.Register(mux)

	body := bytes.NewBufferString(`{"chat_id":"alert_1","kind":"inv_tool","command":{"op":"resolve_alert"}}`)
	req := agentAuthRequest(http.MethodPost, "/api/v1/agent/messages", body, agentTok.validToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if alerts.byNumber[1].Status != "resolved" {
		t.Fatalf("alert should have been resolved")
	}
}

func TestAgentMessagesInvToolStructuredCapabilityError(t *testing.T) {
	agentID := uuid.New()
	agentTok := &testAgentTokenStore{
		validToken:   "secret-agent-token",
		agentID:      agentID,
		name:         "limited-agent",
		capabilities: []string{}, // No capabilities at all!
	}
	userStore := &mockUserStore{users: []store.UserRecord{testAdminUser}}
	sessionStore := &mockSessionStore{
		sessions: map[string]*store.SessionRecord{
			"test-session-id": {ID: "test-session-id", UserID: testAdminUser.ID, ExpiresAt: time.Now().Add(24 * time.Hour)},
		},
	}
	alerts := &resolvingAlertStore{mockStore: mockStore{
		byFP:     map[string]store.AlertRecord{"fp-1": {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1}},
		byNumber: map[int64]store.AlertRecord{1: {Fingerprint: "fp-1", Status: "firing", AlertNumber: 1}},
	}}

	srv := NewServer(
		&config.Config{},
		alerts,
		&mockWebhookTokenStore{tokens: map[string]store.WebhookTokenRecord{}},
		agentTok,
		userStore,
		sessionStore,
		&mockAuditStore{},
		&mockIntegrationStore{},
		&mockRouteRulesStore{},
		24*time.Hour,
		nil,
		nil,
		nil,
		nil,
		func(*routing.Engine) {},
		NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute),
		NewRateLimiter(10, 20),
		nil,
		&mockIncidentInvestigationStore{},
		nil,
		nil,
		nil,
		nil,
	)
	srv.alertStore = alerts

	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	executor.SetAlertSideEffects(&agent.AgentAlertSideEffects{Store: alerts})
	srv.SetAgentService(agent.NewService(nil, executor, nil, srv.agentTokenStore, nil, nil, nil, nil, platform.AuthDeps{}, platform.AgentRateLimitDeps{}, nil))

	mux := http.NewServeMux()
	srv.Register(mux)

	body := bytes.NewBufferString(`{"chat_id":"alert_1","kind":"inv_tool","command":{"op":"resolve_alert"}}`)
	req := agentAuthRequest(http.MethodPost, "/api/v1/agent/messages", body, agentTok.validToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Error              string `json:"error"`
		RequiredCapability string `json:"required_capability"`
	}
	if err := decodeResponse(t, rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.RequiredCapability != "investigate" {
		t.Fatalf("expected required_capability='investigate', got %q", resp.RequiredCapability)
	}
}

func TestGetAlertReturnsCurrentInvestigation(t *testing.T) {
	alerts := &mockStore{
		byFP: map[string]store.AlertRecord{
			"fp1": {Fingerprint: "fp1", AlertNumber: 1},
		},
		byNumber: map[int64]store.AlertRecord{
			1: {Fingerprint: "fp1", AlertNumber: 1},
		},
	}
	oldInvUUID := uuid.New()
	currentInvUUID := uuid.New()
	inv := &trackingAlertInvestigationStore{
		byID: map[string]*store.AlertInvestigationRecord{
			"AINV-old": {
				ID:                   oldInvUUID,
				AlertInvestigationID: "AINV-old",
				Status:               "complete",
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp1", AlertNumber: 1}},
			},
			"AINV-current": {
				ID:                   currentInvUUID,
				AlertInvestigationID: "AINV-current",
				Status:               "investigating",
				Alerts:               []rabbitmq.CorrelatedAlert{{Fingerprint: "fp1", AlertNumber: 1}},
			},
		},
	}
	_, mux := newInvestigationTestServer(alerts, inv)
	req := authRequest(http.MethodGet, "/api/v1/alerts/1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload map[string]json.RawMessage
	if err := decodeResponse(t, rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var invPayload map[string]any
	if err := json.Unmarshal(payload["alert_investigation"], &invPayload); err != nil {
		t.Fatalf("decode investigation: %v", err)
	}
	if invPayload["alert_investigation_id"] != "AINV-current" {
		t.Fatalf("expected AINV-current, got %v", invPayload["alert_investigation_id"])
	}
}

type mockIncidentDocumentStore struct {
	sections []store.IncidentDocumentRecord
}

func (m *mockIncidentDocumentStore) GetAllSections(ctx context.Context, incidentNumber int64) ([]store.IncidentDocumentRecord, error) {
	return m.sections, nil
}

func (m *mockIncidentDocumentStore) GetSection(ctx context.Context, incidentNumber int64, section ics.DocumentSection) (*store.IncidentDocumentRecord, error) {
	for _, sec := range m.sections {
		if sec.Section == string(section) {
			return &sec, nil
		}
	}
	return nil, nil
}

func (m *mockIncidentDocumentStore) UpsertSection(ctx context.Context, incidentNumber int64, section ics.DocumentSection, content string, version int, userID uuid.UUID) (*store.IncidentDocumentRecord, error) {
	return nil, nil
}

func (m *mockIncidentDocumentStore) InitializeDocument(ctx context.Context, incidentNumber int64, sections map[ics.DocumentSection]string) error {
	return nil
}

func TestResolveIncidentRequiresSummaryImpactAndActions(t *testing.T) {
	incidentID := "77"
	srv, mux := newTestServer(nil)
	srv.SetIncidentStore(&trackingIncidentStore{byIncident: map[int64]*store.IncidentRecord{
		mustParseIncidentNumber(incidentID): {ID: uuid.New(), IncidentNumber: mustParseIncidentNumber(incidentID), Status: "mitigated", Summary: ""},
	}})
	srv.SetIncidentDocumentStore(&mockIncidentDocumentStore{sections: []store.IncidentDocumentRecord{
		{Section: "impact_assessment", Content: "Customers saw elevated latency", Version: 1},
		{Section: "actions_taken", Content: "Rolled back deployment", Version: 1},
		{Section: "root_cause", Content: "Bad deploy bypassed canary", Version: 1},
		{Section: "resolution", Content: "Rolled back the deploy", Version: 1},
	}})

	req := authRequest(http.MethodPost, "/api/v1/incidents/"+incidentID+"/resolve", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "summary") {
		t.Fatalf("response should mention missing summary: %s", rr.Body.String())
	}

	// Success case
	srv.SetIncidentStore(&trackingIncidentStore{byIncident: map[int64]*store.IncidentRecord{
		mustParseIncidentNumber(incidentID): {ID: uuid.New(), IncidentNumber: mustParseIncidentNumber(incidentID), Status: "mitigated", Summary: "Incident summary is here"},
	}})
	req = authRequest(http.MethodPost, "/api/v1/incidents/"+incidentID+"/resolve", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rr.Code, rr.Body.String())
	}
}

func TestResolveIncidentCascadesLinkedAlerts(t *testing.T) {
	incidentID := "78"
	srv, mux := newTestServer(&mockStore{
		resolveResult: store.AlertCascadeResult{
			Resolved: []store.AlertRecord{{AlertNumber: 1, Fingerprint: "fp-1", Status: "resolved"}},
		},
	})
	srv.SetIncidentStore(&trackingIncidentStore{byIncident: map[int64]*store.IncidentRecord{
		mustParseIncidentNumber(incidentID): {ID: uuid.New(), IncidentNumber: mustParseIncidentNumber(incidentID), Status: "mitigated", Summary: "ok"},
	}})
	srv.SetIncidentDocumentStore(&mockIncidentDocumentStore{sections: []store.IncidentDocumentRecord{
		{Section: "impact_assessment", Content: "impact", Version: 1},
		{Section: "actions_taken", Content: "actions", Version: 1},
		{Section: "root_cause", Content: "root cause", Version: 1},
		{Section: "resolution", Content: "resolution", Version: 1},
	}})

	req := authRequest(http.MethodPost, "/api/v1/incidents/"+incidentID+"/resolve", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rr.Code, rr.Body.String())
	}
	var resp incidentResolveResponse
	if err := decodeResponse(t, rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rr.Body.String())
	}
	if resp.Cascade.Resolved != 1 {
		t.Fatalf("cascade.resolved = %d, want 1 (body=%s)", resp.Cascade.Resolved, rr.Body.String())
	}
	if resp.Incident == nil || resp.Incident.IncidentNumber != mustParseIncidentNumber(incidentID) {
		t.Fatalf("incident not wrapped correctly: %#v", resp.Incident)
	}
}

func TestCloseIncidentCascadesLinkedAlerts(t *testing.T) {
	incidentID := "79"
	srv, mux := newTestServer(&mockStore{
		resolveResult: store.AlertCascadeResult{
			Resolved: []store.AlertRecord{{AlertNumber: 5, Fingerprint: "fp-5", Status: "resolved"}},
		},
	})
	srv.SetIncidentStore(&trackingIncidentStore{byIncident: map[int64]*store.IncidentRecord{
		mustParseIncidentNumber(incidentID): {ID: uuid.New(), IncidentNumber: mustParseIncidentNumber(incidentID), Status: "resolved", Summary: "ok"},
	}})

	req := authRequest(http.MethodPost, "/api/v1/incidents/"+incidentID+"/close", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rr.Code, rr.Body.String())
	}
	var resp incidentResolveResponse
	if err := decodeResponse(t, rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rr.Body.String())
	}
	if resp.Cascade.Resolved != 1 {
		t.Fatalf("cascade.resolved = %d, want 1 (body=%s)", resp.Cascade.Resolved, rr.Body.String())
	}
}

func TestTelnyxCallback_NotConfigured(t *testing.T) {
	t.Parallel()
	_, mux := newTestServer(nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/telnyx/callback?incident=1&level=1", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "telnyx not configured") {
		t.Fatalf("body = %q, expected \"telnyx not configured\"", rr.Body.String())
	}
}

func TestTelnyxCallback_MethodNotAllowed(t *testing.T) {
	t.Parallel()
	srv, mux := newTestServer(nil)
	srv.telnyxClient = telnyx.NewClient("k", "c", "+1", "", "", "")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/telnyx/callback?incident=1&level=1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rr.Code)
	}
}

func TestTelnyxCallback_InvalidSignature(t *testing.T) {
	t.Parallel()
	srv, mux := newTestServer(nil)
	srv.telnyxClient = telnyx.NewClient("k", "c", "+1", "", "", "")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/telnyx/callback?incident=1&level=1", strings.NewReader(`{"data":{"event_type":"call.initiated"}}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "invalid signature") {
		t.Fatalf("body = %q, expected \"invalid signature\"", rr.Body.String())
	}
}
