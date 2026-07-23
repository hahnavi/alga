package api

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"alga/config"
	"alga/routing"
	"alga/store"
)

type mockSystemConfigStore struct {
	mu  sync.Mutex
	cfg *store.SystemConfigValues
}

func (m *mockSystemConfigStore) Get() (*store.SystemConfigValues, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cfg == nil {
		return nil, nil
	}
	cp := *m.cfg
	return &cp, nil
}

func (m *mockSystemConfigStore) Save(cfg store.SystemConfigValues) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = &store.SystemConfigValues{
		OnboardingCompleted: cfg.OnboardingCompleted,
	}
	return nil
}

func newOnboardingTestServer(sysCfg store.SystemConfigStore) (*Server, *http.ServeMux) {
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
		&allowAllLoginLimiter{},
		&allowAllRateLimiter{},
		&mockAlertInvestigationStore{},
		&mockIncidentInvestigationStore{},
		nil,
		nil,
		nil,
		nil,
	)
	if sysCfg != nil {
		srv.SetSystemConfigStore(sysCfg)
	}
	mux := http.NewServeMux()
	srv.Register(mux)
	return srv, mux
}

func TestOnboardingStatus_NoStore(t *testing.T) {
	_, mux := newOnboardingTestServer(nil)

	req := authRequest(http.MethodGet, "/api/v1/onboarding/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]bool
	if err := decodeResponse(t, rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["completed"] != false {
		t.Fatalf("expected completed=false, got %v", resp["completed"])
	}
}

func TestOnboardingStatus_Completed(t *testing.T) {
	sysCfg := &mockSystemConfigStore{cfg: &store.SystemConfigValues{OnboardingCompleted: true}}
	_, mux := newOnboardingTestServer(sysCfg)

	req := authRequest(http.MethodGet, "/api/v1/onboarding/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]bool
	if err := decodeResponse(t, rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["completed"] != true {
		t.Fatalf("expected completed=true, got %v", resp["completed"])
	}
}

func TestOnboardingComplete_Success(t *testing.T) {
	sysCfg := &mockSystemConfigStore{}
	_, mux := newOnboardingTestServer(sysCfg)

	req := authRequest(http.MethodPost, "/api/v1/onboarding/complete", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]bool
	if err := decodeResponse(t, rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["completed"] != true {
		t.Fatalf("expected completed=true, got %v", resp["completed"])
	}

	saved, _ := sysCfg.Get()
	if saved == nil || !saved.OnboardingCompleted {
		t.Fatal("expected onboarding_completed to be persisted as true")
	}
}

func TestOnboardingComplete_NoStore(t *testing.T) {
	_, mux := newOnboardingTestServer(nil)

	req := authRequest(http.MethodPost, "/api/v1/onboarding/complete", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}
