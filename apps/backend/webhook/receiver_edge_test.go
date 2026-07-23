package webhook

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	entschema "alga/ent/schema"
	"alga/logger"
	"alga/rabbitmq"
	"alga/routing"
	"alga/store"
	"alga/types"
)

type mockAlertStore struct {
	records     map[string]*store.AlertRecord
	createCalls map[string]int
	createErr   error
}

func newMockAlertStore() *mockAlertStore {
	return &mockAlertStore{
		records:     make(map[string]*store.AlertRecord),
		createCalls: make(map[string]int),
	}
}

func (m *mockAlertStore) Create(record store.AlertRecord) (int64, error) {
	if m.createErr != nil {
		return 0, m.createErr
	}
	m.createCalls[record.Fingerprint]++
	if _, exists := m.records[record.Fingerprint]; !exists {
		record.AlertNumber = int64(len(m.records) + 1)
		m.records[record.Fingerprint] = &record
	}
	return record.AlertNumber, nil
}

func (m *mockAlertStore) GetByFingerprint(fingerprint string) (*store.AlertRecord, error) {
	r, ok := m.records[fingerprint]
	if !ok {
		return nil, nil
	}
	return r, nil
}

func (m *mockAlertStore) GetOpenByFingerprint(fingerprint string) (*store.AlertRecord, error) {
	r, ok := m.records[fingerprint]
	if !ok || r.Status == "resolved" {
		return nil, nil
	}
	return r, nil
}

func (m *mockAlertStore) UpdateStatus(fingerprint, status string, resolvedEvent *store.AlertEvent) error {
	r, ok := m.records[fingerprint]
	if !ok {
		return errors.New("not found")
	}
	r.Status = status
	return nil
}

func (m *mockAlertStore) UpdateStatusSilenced(fingerprint string) error {
	r, ok := m.records[fingerprint]
	if !ok {
		return errors.New("not found")
	}
	r.Status = "resolved"
	r.Silenced = true
	return nil
}

func (m *mockAlertStore) UpdateDeliveryTargets(fingerprint string, targets []store.DeliveryTarget) error {
	r, ok := m.records[fingerprint]
	if !ok {
		return errors.New("not found")
	}
	r.DeliveryTargets = targets
	return nil
}

func (m *mockAlertStore) AcknowledgeAlert(fingerprint string, actor *store.EventActor) error {
	return nil
}

func (m *mockAlertStore) ReopenAlert(fingerprint string, ev store.AlertEvent) error {
	return nil
}

func (m *mockAlertStore) ResolveAlertByUser(fingerprint string, actor *store.EventActor) error {
	return nil
}

func (m *mockAlertStore) DeleteAlert(fingerprint string) error {
	return nil
}

func (m *mockAlertStore) GetByAlertNumber(_ int64) (*store.AlertRecord, error)        { return nil, nil }
func (m *mockAlertStore) AcknowledgeAlertByNumber(_ int64, _ *store.EventActor) error { return nil }
func (m *mockAlertStore) ReopenAlertByNumber(_ int64, _ store.AlertEvent) error       { return nil }
func (m *mockAlertStore) ResolveAlertByNumber(_ int64, _ *store.EventActor) error     { return nil }
func (m *mockAlertStore) DeleteAlertByNumber(_ int64) error                           { return nil }

func (m *mockAlertStore) QueryAlerts(filter map[string]any) ([]store.AlertRecord, error) {
	return nil, nil
}

func (m *mockAlertStore) ListUninvestigatedAlerts(ctx context.Context, threshold time.Duration) ([]store.AlertRecord, error) {
	return nil, nil
}

func (m *mockAlertStore) DeleteOlderThan(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (m *mockAlertStore) CountOlderThan(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (m *mockAlertStore) Close() {}

func (m *mockAlertStore) TriageResultStore() store.TriageResultStore { return nil }

func (m *mockAlertStore) TriageRuleStore() store.TriageRuleStore { return nil }

func (m *mockAlertStore) LinkAlertToIncident(_ context.Context, _ string, _ int64) error {
	return nil
}

func (m *mockAlertStore) UnlinkAlertFromIncident(_ context.Context, _ string, _ int64) error {
	return nil
}

func (m *mockAlertStore) GetAlertsByIncident(_ context.Context, _ int64) ([]string, error) {
	return nil, nil
}

func (m *mockAlertStore) ResolveAlertsByIncident(_ context.Context, _ int64, _ *store.EventActor) (store.AlertCascadeResult, error) {
	return store.AlertCascadeResult{}, nil
}

type mockAlertStoreWithError struct {
	getErr error
}

func (m *mockAlertStoreWithError) Create(record store.AlertRecord) (int64, error) { return 0, nil }
func (m *mockAlertStoreWithError) GetByFingerprint(fingerprint string) (*store.AlertRecord, error) {
	return nil, m.getErr
}
func (m *mockAlertStoreWithError) GetOpenByFingerprint(fingerprint string) (*store.AlertRecord, error) {
	return nil, m.getErr
}
func (m *mockAlertStoreWithError) UpdateStatus(fingerprint, status string, resolvedEvent *store.AlertEvent) error {
	return nil
}
func (m *mockAlertStoreWithError) UpdateStatusSilenced(fingerprint string) error { return nil }
func (m *mockAlertStoreWithError) UpdateDeliveryTargets(fingerprint string, targets []store.DeliveryTarget) error {
	return nil
}
func (m *mockAlertStoreWithError) AcknowledgeAlert(fingerprint string, actor *store.EventActor) error {
	return nil
}
func (m *mockAlertStoreWithError) ReopenAlert(fingerprint string, ev store.AlertEvent) error {
	return nil
}
func (m *mockAlertStoreWithError) ResolveAlertByUser(fingerprint string, actor *store.EventActor) error {
	return nil
}
func (m *mockAlertStoreWithError) DeleteAlert(fingerprint string) error { return nil }

func (m *mockAlertStoreWithError) GetByAlertNumber(_ int64) (*store.AlertRecord, error) {
	return nil, nil
}
func (m *mockAlertStoreWithError) AcknowledgeAlertByNumber(_ int64, _ *store.EventActor) error {
	return nil
}
func (m *mockAlertStoreWithError) ReopenAlertByNumber(_ int64, _ store.AlertEvent) error { return nil }
func (m *mockAlertStoreWithError) ResolveAlertByNumber(_ int64, _ *store.EventActor) error {
	return nil
}
func (m *mockAlertStoreWithError) DeleteAlertByNumber(_ int64) error { return nil }

func (m *mockAlertStoreWithError) QueryAlerts(filter map[string]any) ([]store.AlertRecord, error) {
	return nil, nil
}
func (m *mockAlertStoreWithError) ListUninvestigatedAlerts(ctx context.Context, threshold time.Duration) ([]store.AlertRecord, error) {
	return nil, nil
}
func (m *mockAlertStoreWithError) DeleteOlderThan(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
func (m *mockAlertStoreWithError) CountOlderThan(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}
func (m *mockAlertStoreWithError) Close() {}

func (m *mockAlertStoreWithError) TriageResultStore() store.TriageResultStore { return nil }

func (m *mockAlertStoreWithError) TriageRuleStore() store.TriageRuleStore { return nil }

func (m *mockAlertStoreWithError) LinkAlertToIncident(_ context.Context, _ string, _ int64) error {
	return nil
}

func (m *mockAlertStoreWithError) UnlinkAlertFromIncident(_ context.Context, _ string, _ int64) error {
	return nil
}

func (m *mockAlertStoreWithError) GetAlertsByIncident(_ context.Context, _ int64) ([]string, error) {
	return nil, nil
}

func (m *mockAlertStoreWithError) ResolveAlertsByIncident(_ context.Context, _ int64, _ *store.EventActor) (store.AlertCascadeResult, error) {
	return store.AlertCascadeResult{}, nil
}

type mockReceiverAlertInvestigationStore struct {
	byAlertNumber map[int64][]store.AlertInvestigationRecord
}

func (m *mockReceiverAlertInvestigationStore) CreateAlertInvestigation(ctx context.Context, record store.AlertInvestigationRecord) (*store.AlertInvestigationRecord, error) {
	return &record, nil
}
func (m *mockReceiverAlertInvestigationStore) GetAlertInvestigation(ctx context.Context, id string) (*store.AlertInvestigationRecord, error) {
	return nil, store.ErrInvestigationNotFound
}
func (m *mockReceiverAlertInvestigationStore) ListAlertInvestigationsByAlertNumber(ctx context.Context, alertNumber int64) ([]store.AlertInvestigationRecord, error) {
	return m.byAlertNumber[alertNumber], nil
}
func (m *mockReceiverAlertInvestigationStore) GetCurrentAlertInvestigationByAlertNumber(ctx context.Context, alertNumber int64) (*store.AlertInvestigationRecord, error) {
	return nil, store.ErrInvestigationNotFound
}
func (m *mockReceiverAlertInvestigationStore) GetCurrentAlertInvestigationSummariesByAlertNumbers(ctx context.Context, alertNumbers []int64) (map[int64]store.AlertInvestigationSummary, error) {
	return map[int64]store.AlertInvestigationSummary{}, nil
}
func (m *mockReceiverAlertInvestigationStore) GetActiveAlertInvestigationByCorrelationKey(ctx context.Context, correlationKey string) (*store.AlertInvestigationRecord, error) {
	return nil, store.ErrInvestigationNotFound
}
func (m *mockReceiverAlertInvestigationStore) AppendAlertsToAlertInvestigation(ctx context.Context, id string, alerts []rabbitmq.CorrelatedAlert) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) MarkAlertInvestigationAlertsCurrent(ctx context.Context, investigationID string, current bool) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) AddAlertInvestigationUpdate(ctx context.Context, id string, update store.InvestigationUpdate) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) AppendAlertInvestigationEvent(ctx context.Context, investigationUUID uuid.UUID, event store.AlertInvestigationEvent) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) CompleteAlertInvestigation(ctx context.Context, id string, completion store.AlertInvestigationCompletion) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) RequeueAlertInvestigation(ctx context.Context, id string, requeue store.AlertInvestigationRequeue) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) MarkAlertInvestigationPromoted(ctx context.Context, id string, incidentID string, incidentNumber int64, incidentInvestigationID string) (*store.AlertInvestigationRecord, error) {
	return nil, nil
}
func (m *mockReceiverAlertInvestigationStore) UpdateAlertInvestigationStatus(ctx context.Context, id string, status string) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) GetAlertInvestigationByAlertNumber(ctx context.Context, alertNumber int64) (*store.AlertInvestigationRecord, error) {
	return nil, store.ErrInvestigationNotFound
}
func (m *mockReceiverAlertInvestigationStore) ListPendingAlertInvestigations(ctx context.Context, limit int64) ([]store.AlertInvestigationRecord, error) {
	return nil, nil
}
func (m *mockReceiverAlertInvestigationStore) ClaimPendingAlertInvestigation(ctx context.Context, id string, agentID string, agentName string, agentType string) (*store.AlertInvestigationRecord, error) {
	return nil, nil
}
func (m *mockReceiverAlertInvestigationStore) TransitionAlertInvestigationStatus(ctx context.Context, id string, fromStatuses []string, toStatus string) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) PatchAlertInvestigationOutcome(ctx context.Context, id string, rootCause *string, resolution *string) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) UpdateAlertInvestigationAgent(ctx context.Context, id string, agentID string, agentName string, agentType string) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) SetAlertInvestigationAssignee(ctx context.Context, id string, assigneeType string, assigneeID *uuid.UUID) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) ResetInvestigatingByAgent(ctx context.Context, agentID string) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) ResetAssignedByAgent(ctx context.Context, agentID string) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) CountActiveByAgent(ctx context.Context, agentID string) (int, error) {
	return 0, nil
}
func (m *mockReceiverAlertInvestigationStore) CountActiveByAgents(ctx context.Context, agentIDs []string) (map[string]int, error) {
	return nil, nil
}
func (m *mockReceiverAlertInvestigationStore) DeleteAlertInvestigation(ctx context.Context, id string) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) ListAlertInvestigations(ctx context.Context, filter map[string]any) ([]store.AlertInvestigationRecord, error) {
	return nil, nil
}
func (m *mockReceiverAlertInvestigationStore) UpdateAlertInvestigationMessage(ctx context.Context, investigationID string, updateID string, message string) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) DeleteAlertInvestigationMessage(ctx context.Context, investigationID string, updateID string) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) SetAlertInvestigationUpdateMMPostID(ctx context.Context, investigationID string, updateID string, mmPostID string) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) SetAlertInvestigationUpdateSlackMessageTS(ctx context.Context, investigationID string, updateID string, slackMessageTS string) error {
	return nil
}
func (m *mockReceiverAlertInvestigationStore) GetAlertInvestigationByMMThread(ctx context.Context, mmThreadID string) (*store.AlertInvestigationRecord, error) {
	return nil, store.ErrInvestigationNotFound
}
func (m *mockReceiverAlertInvestigationStore) GetAlertInvestigationBySlackThread(ctx context.Context, channelID string, threadTS string) (*store.AlertInvestigationRecord, error) {
	return nil, store.ErrInvestigationNotFound
}
func (m *mockReceiverAlertInvestigationStore) FindSimilarAlertInvestigations(ctx context.Context, q store.SimilarAlertInvestigationsQuery) ([]store.AlertInvestigationRecord, error) {
	return nil, nil
}
func (m *mockReceiverAlertInvestigationStore) ListStalledAssignedAlertInvestigations(ctx context.Context, threshold time.Duration) ([]store.AlertInvestigationRecord, error) {
	return nil, nil
}
func (m *mockReceiverAlertInvestigationStore) ListStalledInvestigatingAlertInvestigations(ctx context.Context, threshold time.Duration) ([]store.AlertInvestigationRecord, error) {
	return nil, nil
}
func (m *mockReceiverAlertInvestigationStore) ResetStalledAssignedAlertInvestigations(timeout time.Duration) ([]string, error) {
	return nil, nil
}
func (m *mockReceiverAlertInvestigationStore) ResetStalledInvestigatingAlertInvestigations(timeout time.Duration) ([]string, error) {
	return nil, nil
}

type mockReceiverIncidentInvestigationStore struct {
	byID          map[string]*store.IncidentInvestigationRecord
	statusUpdates map[string]string
	updates       map[string][]store.InvestigationUpdate
}

func (m *mockReceiverIncidentInvestigationStore) CreateIncidentInvestigation(ctx context.Context, record store.IncidentInvestigationRecord) (*store.IncidentInvestigationRecord, error) {
	return &record, nil
}
func (m *mockReceiverIncidentInvestigationStore) GetIncidentInvestigation(ctx context.Context, id string) (*store.IncidentInvestigationRecord, error) {
	if rec := m.byID[id]; rec != nil {
		cp := *rec
		return &cp, nil
	}
	return nil, store.ErrInvestigationNotFound
}
func (m *mockReceiverIncidentInvestigationStore) GetActiveIncidentInvestigationByIncident(ctx context.Context, incidentNumber int64) (*store.IncidentInvestigationRecord, error) {
	return nil, store.ErrInvestigationNotFound
}
func (m *mockReceiverIncidentInvestigationStore) ListIncidentInvestigationsByIncident(ctx context.Context, incidentNumber int64) ([]store.IncidentInvestigationRecord, error) {
	return nil, nil
}
func (m *mockReceiverIncidentInvestigationStore) AddIncidentInvestigationUpdate(ctx context.Context, id string, update store.InvestigationUpdate) error {
	m.updates[id] = append(m.updates[id], update)
	return nil
}
func (m *mockReceiverIncidentInvestigationStore) UpdateIncidentInvestigationStatus(ctx context.Context, id string, status string) error {
	m.statusUpdates[id] = status
	return nil
}
func (m *mockReceiverIncidentInvestigationStore) ClaimPendingIncidentInvestigation(ctx context.Context, id string, agentID string, agentName string, agentType string) (*store.IncidentInvestigationRecord, error) {
	return nil, nil
}
func (m *mockReceiverIncidentInvestigationStore) ListPendingIncidentInvestigations(ctx context.Context, limit int64) ([]store.IncidentInvestigationRecord, error) {
	return nil, nil
}
func (m *mockReceiverIncidentInvestigationStore) SetIncidentInvestigationSummary(ctx context.Context, incidentInvestigationID string, summary *entschema.InvestigationSummary) error {
	return nil
}
func (m *mockReceiverIncidentInvestigationStore) SetIncidentInvestigationAssignee(ctx context.Context, id string, assigneeType string, assigneeID *uuid.UUID) error {
	return nil
}

type mockDedupCache struct {
	tracked map[string]bool
}

func newMockDedupCache() *mockDedupCache {
	return &mockDedupCache{tracked: make(map[string]bool)}
}

func (m *mockDedupCache) IsDuplicate(_ context.Context, fingerprint string) bool {
	return m.tracked[fingerprint]
}

func (m *mockDedupCache) MarkTracked(_ context.Context, fingerprint string) error {
	m.tracked[fingerprint] = true
	return nil
}

func (m *mockDedupCache) RemoveTracking(_ context.Context, fingerprint string) {
	delete(m.tracked, fingerprint)
}

type mockWebhookTokenStore struct {
	validTokens map[string]bool
	validateErr error
}

func newMockWebhookTokenStore(valid ...string) *mockWebhookTokenStore {
	m := &mockWebhookTokenStore{validTokens: make(map[string]bool)}
	for _, t := range valid {
		m.validTokens[t] = true
	}
	return m
}

func (m *mockWebhookTokenStore) CreateToken(name string, expiresAt *time.Time) (*store.WebhookTokenRecord, error) {
	return nil, nil
}

func (m *mockWebhookTokenStore) ListTokens() ([]store.WebhookTokenRecord, error) {
	return nil, nil
}

func (m *mockWebhookTokenStore) RevokeToken(id uuid.UUID) error {
	return nil
}

func (m *mockWebhookTokenStore) ValidateToken(token string) (bool, error) {
	if m.validateErr != nil {
		return false, m.validateErr
	}
	return m.validTokens[token], nil
}

func (m *mockWebhookTokenStore) Close() {}

func newTestReceiver(alertStore store.Store, tokenStore *mockWebhookTokenStore, dedup *mockDedupCache) *Receiver {
	var cache DedupCache
	if dedup != nil {
		cache = dedup
	}
	return NewReceiver(routing.NewEngine(nil), nil, nil, alertStore, tokenStore, cache)
}

func makeFiringPayload(fingerprint string) string {
	return `{
		"receiver": "test",
		"status": "firing",
		"alerts": [
			{
				"status": "firing",
				"labels": {"alertname": "TestAlert"},
				"annotations": {"summary": "test"},
				"startsAt": "2026-01-01T00:00:00Z",
				"fingerprint": "` + fingerprint + `"
			}
		]
	}`
}

func makeResolvedPayload(fingerprint string) string {
	return `{
		"receiver": "test",
		"status": "resolved",
		"alerts": [
			{
				"status": "resolved",
				"labels": {"alertname": "TestAlert"},
				"annotations": {"summary": "test"},
				"startsAt": "2026-01-01T00:00:00Z",
				"endsAt": "2026-01-01T01:00:00Z",
				"fingerprint": "` + fingerprint + `"
			}
		]
	}`
}

func TestWebhookDuplicateFingerprint(t *testing.T) {
	logger.Init("error", "")

	tokenStore := newMockWebhookTokenStore("valid-token")
	alertStore := newMockAlertStore()
	dedup := newMockDedupCache()
	receiver := newTestReceiver(alertStore, tokenStore, dedup)

	payload := makeFiringPayload("fp-dup-001")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/alerts", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	receiver.handleWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("first alert: status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPost, "/webhooks/alerts", strings.NewReader(payload))
	req2.Header.Set("Authorization", "Bearer valid-token")
	rec2 := httptest.NewRecorder()
	receiver.handleWebhook(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("duplicate alert: status = %d, want 200; body: %s", rec2.Code, rec2.Body.String())
	}

	if len(alertStore.records) != 1 {
		t.Fatalf("expected 1 stored alert, got %d", len(alertStore.records))
	}
}

func TestWebhookDuplicateFingerprintWithDedupCache(t *testing.T) {
	logger.Init("error", "")

	tokenStore := newMockWebhookTokenStore("valid-token")
	alertStore := newMockAlertStore()
	dedup := newMockDedupCache()
	receiver := newTestReceiver(alertStore, tokenStore, dedup)

	payload := makeFiringPayload("fp-cache-dup")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/alerts", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	receiver.handleWebhook(rec, req)

	if !dedup.tracked["fp-cache-dup"] {
		t.Fatal("expected fingerprint to be tracked in dedup cache after first alert")
	}

	alertStore2 := newMockAlertStore()
	alertStore2.records["fp-cache-dup"] = &store.AlertRecord{
		Fingerprint: "fp-cache-dup",
		Status:      "firing",
	}
	receiver.store = alertStore2

	payload2 := makeFiringPayload("fp-cache-dup")
	req2 := httptest.NewRequest(http.MethodPost, "/webhooks/alerts", strings.NewReader(payload2))
	req2.Header.Set("Authorization", "Bearer valid-token")
	rec2 := httptest.NewRecorder()
	receiver.handleWebhook(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("dedup cache skip: status = %d, want 200; body: %s", rec2.Code, rec2.Body.String())
	}
}

func TestWebhookMalformedJSON(t *testing.T) {
	logger.Init("error", "")

	tokenStore := newMockWebhookTokenStore("valid-token")
	receiver := newTestReceiver(nil, tokenStore, nil)

	tests := []struct {
		name string
		body string
		want int
	}{
		{"completely invalid", `{not json}`, http.StatusBadRequest},
		{"empty body", ``, http.StatusBadRequest},
		{"truncated JSON", `{"alerts": [{"fingerprint": "abc"`, http.StatusBadRequest},
		{"null body", `null`, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/webhooks/alerts", strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer valid-token")
			rec := httptest.NewRecorder()
			receiver.handleWebhook(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestWebhookInvalidToken(t *testing.T) {
	logger.Init("error", "")

	tests := []struct {
		name       string
		tokenStore *mockWebhookTokenStore
		authHeader string
		wantStatus int
	}{
		{
			name:       "wrong token",
			tokenStore: newMockWebhookTokenStore("correct-token"),
			authHeader: "Bearer wrong-token",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "empty bearer",
			tokenStore: newMockWebhookTokenStore("valid-token"),
			authHeader: "Bearer ",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "token store returns error",
			tokenStore: &mockWebhookTokenStore{validateErr: errors.New("db connection lost")},
			authHeader: "Bearer some-token",
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receiver := newTestReceiver(nil, tt.tokenStore, nil)
			req := httptest.NewRequest(http.MethodPost, "/webhooks/alerts", strings.NewReader(`{"alerts":[]}`))
			req.Header.Set("Authorization", tt.authHeader)
			rec := httptest.NewRecorder()
			receiver.handleWebhook(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestWebhookMissingToken(t *testing.T) {
	logger.Init("error", "")

	tokenStore := newMockWebhookTokenStore("valid-token")
	receiver := newTestReceiver(nil, tokenStore, nil)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/alerts", strings.NewReader(`{"alerts":[]}`))
	rec := httptest.NewRecorder()
	receiver.handleWebhook(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestWebhookMethodNotAllowed(t *testing.T) {
	logger.Init("error", "")

	tokenStore := newMockWebhookTokenStore("valid-token")
	receiver := newTestReceiver(nil, tokenStore, nil)

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req := httptest.NewRequest(method, "/webhooks/alerts", nil)
		rec := httptest.NewRecorder()
		receiver.handleWebhook(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("method %s: status = %d, want %d", method, rec.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestWebhookEmptyAlerts(t *testing.T) {
	logger.Init("error", "")

	tokenStore := newMockWebhookTokenStore("valid-token")
	alertStore := newMockAlertStore()
	receiver := newTestReceiver(alertStore, tokenStore, nil)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/alerts", strings.NewReader(`{"alerts":[]}`))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	receiver.handleWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	if len(alertStore.records) != 0 {
		t.Fatalf("expected 0 stored alerts, got %d", len(alertStore.records))
	}
}

func TestWebhookAlertWithEmptyFields(t *testing.T) {
	logger.Init("error", "")

	tokenStore := newMockWebhookTokenStore("valid-token")
	alertStore := newMockAlertStore()
	receiver := newTestReceiver(alertStore, tokenStore, nil)

	payload := `{
		"alerts": [{
			"status": "firing",
			"labels": {},
			"annotations": {},
			"startsAt": "",
			"fingerprint": ""
		}]
	}`

	req := httptest.NewRequest(http.MethodPost, "/webhooks/alerts", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	receiver.handleWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	if len(alertStore.records) != 1 {
		t.Fatalf("expected 1 stored alert (empty fingerprint), got %d", len(alertStore.records))
	}
}

func TestWebhookResolveNonExistent(t *testing.T) {
	logger.Init("error", "")

	tokenStore := newMockWebhookTokenStore("valid-token")
	alertStore := newMockAlertStore()
	receiver := newTestReceiver(alertStore, tokenStore, nil)

	payload := makeResolvedPayload("fp-noexist-999")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/alerts", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	receiver.handleWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	if len(alertStore.records) != 1 {
		t.Fatalf("expected 1 stored resolved alert (orphan resolve), got %d", len(alertStore.records))
	}

	for _, r := range alertStore.records {
		if r.Status != "resolved" {
			t.Fatalf("expected status=resolved, got %s", r.Status)
		}
	}
}

func TestWebhookResolveAlreadyResolved(t *testing.T) {
	logger.Init("error", "")

	tokenStore := newMockWebhookTokenStore("valid-token")
	alertStore := newMockAlertStore()
	alertStore.records["fp-already-resolved"] = &store.AlertRecord{
		Fingerprint: "fp-already-resolved",
		Status:      "resolved",
	}
	receiver := newTestReceiver(alertStore, tokenStore, nil)

	payload := makeResolvedPayload("fp-already-resolved")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/alerts", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	receiver.handleWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	r := alertStore.records["fp-already-resolved"]
	if r.Status != "resolved" {
		t.Fatalf("expected status to remain resolved, got %s", r.Status)
	}
}

// TestResolvedNotificationDoesNotCreatePhantomAfterResolve reproduces the bug
// where a resolved notification from Grafana for a fingerprint whose alert was
// already resolved created a brand-new phantom alert record (e.g. alert #37
// spawned after alert #36 was already resolved). A resolved notification must
// be idempotent: it must not create a new alert when an existing record (by any
// status) is already present for the fingerprint.
func TestResolvedNotificationDoesNotCreatePhantomAfterResolve(t *testing.T) {
	logger.Init("error", "")

	tokenStore := newMockWebhookTokenStore("valid-token")
	alertStore := newMockAlertStore()
	alertStore.records["fp-resolved-phantom"] = &store.AlertRecord{
		Fingerprint: "fp-resolved-phantom",
		Status:      "resolved",
	}
	receiver := newTestReceiver(alertStore, tokenStore, nil)

	payload := makeResolvedPayload("fp-resolved-phantom")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/alerts", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	receiver.handleWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	if got := alertStore.createCalls["fp-resolved-phantom"]; got != 0 {
		t.Fatalf("expected 0 Create calls for resolved notification on already-resolved fingerprint, got %d", got)
	}
}

func TestAutoResolvedAlertCompletesPromotedIncidentInvestigation(t *testing.T) {
	alertStore := newMockAlertStore()
	alertStore.records["fp1"] = &store.AlertRecord{Fingerprint: "fp1", AlertNumber: 7, Status: "resolved"}
	incidentID := uuid.New()
	incidentInvestigationID := uuid.New()
	alertInv := store.AlertInvestigationRecord{
		AlertInvestigationID:            "AINV-1",
		Status:                          store.AlertInvestigationStatusPromoted,
		PromotedIncidentID:              &incidentID,
		PromotedIncidentInvestigationID: &incidentInvestigationID,
		Alerts:                          []rabbitmq.CorrelatedAlert{{Fingerprint: "fp1", AlertNumber: 7}},
	}
	alertInvStore := &mockReceiverAlertInvestigationStore{
		byAlertNumber: map[int64][]store.AlertInvestigationRecord{7: {alertInv}},
	}
	incidentInvStore := &mockReceiverIncidentInvestigationStore{
		byID: map[string]*store.IncidentInvestigationRecord{
			incidentInvestigationID.String(): {
				IncidentInvestigationID: incidentInvestigationID.String(),
				Status:                  store.IncidentInvestigationStatusInvestigating,
				AgentID:                 "agent-1",
			},
		},
		statusUpdates: map[string]string{},
		updates:       map[string][]store.InvestigationUpdate{},
	}
	r := NewReceiver(routing.NewEngine(nil), nil, nil, alertStore, nil, nil)
	r.SetAlertInvestigationStore(alertInvStore)
	r.SetIncidentInvestigationStore(incidentInvStore)

	r.handleAutoResolvedInvestigation(context.Background(), types.Alert{Fingerprint: "fp1", Labels: map[string]string{"alertname": "HighCPU"}}, 7)

	if got := incidentInvStore.statusUpdates[incidentInvestigationID.String()]; got != store.IncidentInvestigationStatusComplete {
		t.Fatalf("incident investigation status = %q, want complete", got)
	}
	updates := incidentInvStore.updates[incidentInvestigationID.String()]
	if len(updates) != 1 {
		t.Fatalf("incident investigation updates = %d, want 1", len(updates))
	}
	if updates[0].Source != store.UpdateSourceSystem || !strings.Contains(updates[0].Message, "Grafana resolved") {
		t.Fatalf("unexpected completion update: %#v", updates[0])
	}
}

func TestProcessAlertsGetOpenError(t *testing.T) {
	logger.Init("error", "")

	alertStore := &mockAlertStoreWithError{getErr: errors.New("db connection lost")}
	dedup := newMockDedupCache()
	receiver := newTestReceiver(alertStore, newMockWebhookTokenStore("t"), dedup)

	payload := types.GrafanaAlertingPayload{
		Alerts: []types.Alert{
			{
				Status:      "firing",
				Fingerprint: "fp-store-err",
				Labels:      map[string]string{"alertname": "Test"},
				StartsAt:    "2026-01-01T00:00:00Z",
			},
		},
	}

	err := receiver.ProcessAlerts(context.Background(), payload)
	if err == nil {
		t.Fatal("expected error from ProcessAlerts when GetOpenByFingerprint fails")
	}
	if err.Error() != "db connection lost" {
		t.Fatalf("error = %q, want %q", err.Error(), "db connection lost")
	}
}

func TestProcessAlertsDuplicateFingerprintSkipped(t *testing.T) {
	logger.Init("error", "")

	alertStore := newMockAlertStore()
	dedup := newMockDedupCache()
	dedup.tracked["fp-dedup-skip"] = true

	receiver := newTestReceiver(alertStore, newMockWebhookTokenStore("t"), dedup)

	payload := types.GrafanaAlertingPayload{
		Alerts: []types.Alert{
			{
				Status:      "firing",
				Fingerprint: "fp-dedup-skip",
				Labels:      map[string]string{"alertname": "Test"},
				StartsAt:    "2026-01-01T00:00:00Z",
			},
		},
	}

	err := receiver.ProcessAlerts(context.Background(), payload)
	if err != nil {
		t.Fatalf("expected no error for deduped alert, got %v", err)
	}

	if len(alertStore.records) != 0 {
		t.Fatalf("expected 0 stored alerts (deduped), got %d", len(alertStore.records))
	}
}

func TestWebhookTokenExtraction(t *testing.T) {
	tests := []struct {
		name   string
		target string
		header string
		want   string
	}{
		{"bearer token", "/webhooks/alerts", "Bearer abc123", "abc123"},
		{"bearer with extra spaces", "/webhooks/alerts", "Bearer   abc123  ", "abc123"},
		{"query param", "/webhooks/alerts?token=xyz789", "", "xyz789"},
		{"header takes precedence over query", "/webhooks/alerts?token=query-val", "Bearer header-val", "header-val"},
		{"empty auth falls back to query", "/webhooks/alerts?token=fallback", "", "fallback"},
		{"no token at all", "/webhooks/alerts", "", ""},
		{"bearer with no value", "/webhooks/alerts", "Bearer", "Bearer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.target, nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			got := webhookTokenFromRequest(req)
			if got != tt.want {
				t.Fatalf("webhookTokenFromRequest = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProcessAlertsMultipleAlertsMixedStatus(t *testing.T) {
	logger.Init("error", "")

	alertStore := newMockAlertStore()
	alertStore.records["fp-mix-open"] = &store.AlertRecord{
		Fingerprint: "fp-mix-open",
		Status:      "firing",
	}
	dedup := newMockDedupCache()
	receiver := newTestReceiver(alertStore, newMockWebhookTokenStore("t"), dedup)

	payload := types.GrafanaAlertingPayload{
		Alerts: []types.Alert{
			{
				Status:      "firing",
				Fingerprint: "fp-mix-new",
				Labels:      map[string]string{"alertname": "NewAlert"},
				StartsAt:    "2026-01-01T00:00:00Z",
			},
			{
				Status:      "resolved",
				Fingerprint: "fp-mix-open",
				Labels:      map[string]string{"alertname": "ResolveExisting"},
				StartsAt:    "2026-01-01T00:00:00Z",
				EndsAt:      "2026-01-01T01:00:00Z",
			},
			{
				Status:      "resolved",
				Fingerprint: "fp-mix-orphan",
				Labels:      map[string]string{"alertname": "OrphanResolve"},
				StartsAt:    "2026-01-01T00:00:00Z",
				EndsAt:      "2026-01-01T01:00:00Z",
			},
		},
	}

	err := receiver.ProcessAlerts(context.Background(), payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, ok := alertStore.records["fp-mix-new"]
	if !ok {
		t.Fatal("expected new firing alert to be stored")
	}

	resolved := alertStore.records["fp-mix-open"]
	if resolved == nil || resolved.Status != "resolved" {
		t.Fatal("expected existing open alert to be resolved")
	}

	orphan := alertStore.records["fp-mix-orphan"]
	if orphan == nil || orphan.Status != "resolved" {
		t.Fatal("expected orphan resolved alert to be stored")
	}
}

func TestWebhookHealthEndpoint(t *testing.T) {
	logger.Init("error", "")

	receiver := newTestReceiver(nil, newMockWebhookTokenStore(), nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	receiver.HandleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("health: status = %d, want 200", rec.Code)
	}

	body := strings.TrimSpace(rec.Body.String())
	want := `{"status":"ok"}`
	if body != want {
		t.Fatalf("health body = %q, want %q", body, want)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
}

func TestWebhookAsyncPublishSuccess(t *testing.T) {
	logger.Init("error", "")

	tokenStore := newMockWebhookTokenStore("valid-token")
	alertStore := newMockAlertStore()
	receiver := newTestReceiver(alertStore, tokenStore, nil)
	receiver.SetPublisher(&mockAlertPublisher{shouldSucceed: true})

	req := httptest.NewRequest(http.MethodPost, "/webhooks/alerts", strings.NewReader(makeFiringPayload("fp-async")))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	receiver.handleWebhook(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body: %s", rec.Code, rec.Body.String())
	}

	if !strings.Contains(rec.Body.String(), "queued") {
		t.Fatalf("body = %q, want to contain 'queued'", rec.Body.String())
	}

	if len(alertStore.records) != 0 {
		t.Fatal("expected no direct store writes when async publish succeeds")
	}
}

func TestWebhookAsyncPublishFallback(t *testing.T) {
	logger.Init("error", "")

	tokenStore := newMockWebhookTokenStore("valid-token")
	alertStore := newMockAlertStore()
	receiver := newTestReceiver(alertStore, tokenStore, nil)
	receiver.SetPublisher(&mockAlertPublisher{shouldSucceed: false})

	req := httptest.NewRequest(http.MethodPost, "/webhooks/alerts", strings.NewReader(makeFiringPayload("fp-async-fallback")))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	receiver.handleWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (sync fallback); body: %s", rec.Code, rec.Body.String())
	}

	if len(alertStore.records) != 1 {
		t.Fatalf("expected 1 stored alert (sync fallback), got %d", len(alertStore.records))
	}
}

type mockAlertPublisher struct {
	shouldSucceed bool
}

func (m *mockAlertPublisher) PublishAlert(_ context.Context, _ types.GrafanaAlertingPayload) error {
	if m.shouldSucceed {
		return nil
	}
	return errors.New("queue unavailable")
}
