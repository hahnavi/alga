package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"alga/automation"
	"alga/config"
	"alga/rbac"
	"alga/store"
)

type recordingAutomationStore struct {
	store.AutomationStore
	created int
}

func (s *recordingAutomationStore) CreateRule(_ context.Context, d automation.Definition, _ *uuid.UUID) (*store.AutomationRuleRecord, error) {
	s.created++
	return &store.AutomationRuleRecord{ID: uuid.New(), Name: d.Name, TriggerType: d.TriggerType}, nil
}

func TestAutomationCreateRouteAuthenticationAndRBAC(t *testing.T) {
	definition := `{"name":"Notify team","trigger_type":"alert.created","actions":[{"type":"webhook","url":"https://hooks.example.com"}]}`
	fake := &recordingAutomationStore{}
	audit := &recordingAuditStore{}
	s := &Server{cfg: &config.Config{}, automationStore: fake, auditStore: audit, ipExtractor: newIPExtractor(&config.Config{})}

	unauthorized := httptest.NewRequest(http.MethodPost, "/api/v1/automation/rules", strings.NewReader(definition))
	unauthorized.Header.Set("Content-Type", "application/json")
	unauthorized.AddCookie(&http.Cookie{Name: "alga_csrf", Value: "csrf-token"})
	unauthorized.Header.Set("X-CSRF-Token", "csrf-token")
	w := httptest.NewRecorder()
	gateMiddleware(gateTestDeps(nil, gateTestUser("admin")), s.handleCreateAutomationRule, rbac.AutomationWrite)(w, unauthorized)
	if w.Code != http.StatusUnauthorized || fake.created != 0 {
		t.Fatalf("unauthorized: status=%d calls=%d", w.Code, fake.created)
	}

	viewer := gateSessionRequest(http.MethodPost, "/api/v1/automation/rules")
	viewer.Body = io.NopCloser(strings.NewReader(definition))
	viewer.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	gateMiddleware(gateTestDeps(nil, gateTestUser("viewer")), s.handleCreateAutomationRule, rbac.AutomationWrite)(w, viewer)
	if w.Code != http.StatusForbidden || fake.created != 0 {
		t.Fatalf("viewer: status=%d calls=%d", w.Code, fake.created)
	}

	operator := gateSessionRequest(http.MethodPost, "/api/v1/automation/rules")
	operator.Body = io.NopCloser(strings.NewReader(definition))
	operator.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	gateMiddleware(gateTestDeps(nil, gateTestUser("operator")), s.handleCreateAutomationRule, rbac.AutomationWrite)(w, operator)
	if w.Code != http.StatusCreated || fake.created != 1 {
		t.Fatalf("operator: status=%d calls=%d body=%s", w.Code, fake.created, w.Body.String())
	}
	if len(audit.events) != 1 || audit.events[0] != store.AuditAutomationRuleCreated {
		t.Fatalf("audit=%v", audit.events)
	}
}

func TestAutomationCreateInvalidAndMissingStore(t *testing.T) {
	s := &Server{automationStore: &recordingAutomationStore{}}
	w := httptest.NewRecorder()
	s.handleCreateAutomationRule(w, httptest.NewRequest(http.MethodPost, "/api/v1/automation/rules", strings.NewReader("not json")))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid body: %d", w.Code)
	}
	s.automationStore = nil
	w = httptest.NewRecorder()
	s.handleCreateAutomationRule(w, httptest.NewRequest(http.MethodPost, "/api/v1/automation/rules", strings.NewReader("{}")))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing store: %d", w.Code)
	}
}
