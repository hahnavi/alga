package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/api/agent"
	"alga/api/platform"
	"alga/config"
	"alga/routing"
	"alga/store"
)

// stubPlaybookStore is a minimal PlaybookStore for the playbook capability test.
type stubPlaybookStore struct{}

func (s *stubPlaybookStore) Create(context.Context, *store.PlaybookRecord, []store.PlaybookStepRecord) (*store.PlaybookRecord, error) {
	return nil, nil
}
func (s *stubPlaybookStore) Get(context.Context, uuid.UUID) (*store.PlaybookRecord, []store.PlaybookStepRecord, error) {
	return nil, nil, nil
}
func (s *stubPlaybookStore) Update(context.Context, uuid.UUID, *store.PlaybookRecord) error {
	return nil
}
func (s *stubPlaybookStore) Delete(context.Context, uuid.UUID) error { return nil }
func (s *stubPlaybookStore) List(context.Context, store.PlaybookFilter, int, int) ([]*store.PlaybookRecord, int64, error) {
	return nil, 0, nil
}
func (s *stubPlaybookStore) AddStep(context.Context, *store.PlaybookStepRecord) (*store.PlaybookStepRecord, error) {
	return nil, nil
}
func (s *stubPlaybookStore) UpdateStep(context.Context, uuid.UUID, *store.PlaybookStepRecord) error {
	return nil
}
func (s *stubPlaybookStore) DeleteStep(context.Context, uuid.UUID) error { return nil }
func (s *stubPlaybookStore) ReorderSteps(context.Context, uuid.UUID, []store.StepOrder) error {
	return nil
}
func (s *stubPlaybookStore) FindMatching(context.Context, map[string]string) ([]*store.PlaybookRecord, error) {
	return []*store.PlaybookRecord{}, nil
}

// TestAgentPlaybooksRequiresInvestigateCapability verifies M6: an agent token
// without the Investigate capability is forbidden from listing playbooks
// (ASVS V4.1, SPEC gap M6). Previously any valid agent token could list
// playbooks regardless of its capabilities.
func TestAgentPlaybooksRequiresInvestigateCapability(t *testing.T) {
	// Agent token with ONLY Communicate capability (no Investigate).
	agentTok := &testAgentTokenStore{
		validToken:   "playbooks-agent-token",
		agentID:      uuid.New(),
		name:         "comms-only-agent",
		capabilities: []string{"communicate"},
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
		nil, nil, nil, nil,
		func(*routing.Engine) {},
		NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute),
		NewRateLimiter(10, 20),
		&mockAlertInvestigationStore{},
		&mockIncidentInvestigationStore{},
		nil, nil, nil, nil,
	)
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	srv.SetAgentService(agent.NewService(
		nil, executor, nil, srv.agentTokenStore, nil, nil, nil, nil,
		platform.AuthDeps{}, platform.AgentRateLimitDeps{}, nil,
		agent.WithAlertStores(&mockStore{}, nil, nil, nil, &stubPlaybookStore{}),
	))

	mux := http.NewServeMux()
	srv.Register(mux)

	req := agentAuthRequest(http.MethodGet, "/api/v1/agent/playbooks", nil, agentTok.validToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for agent without investigate capability listing playbooks, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestTwilioCallbackRejectsBadSignature verifies M7 (SPEC §8): a Twilio
// callback with a bad/missing signature is rejected. The handler already
// verified HMAC-SHA1 signatures; this adds the missing regression test.
func TestTwilioCallbackRejectsBadSignature(t *testing.T) {
	srv, mux := newTestServer(nil)
	// Configure an auth token so signature verification is active.
	srv.cfg.TwilioAuthToken = "twilio-secret-auth-token"

	body := strings.NewReader("CallSid=CA123&Digits=1")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/twilio/callback?incident=1&user=u", body)
	// No X-Twilio-Signature header (or a wrong one) -> must be rejected.
	req.Header.Set("X-Twilio-Signature", "bogus-signature")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for bad Twilio signature, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid signature") {
		t.Fatalf("expected 'invalid signature' in body, got: %s", rec.Body.String())
	}
}

// TestTwilioCallbackRejectsWhenUnconfigured verifies M7: when the Twilio auth
// token is not configured, the callback fails closed (503) rather than
// accepting unsigned requests.
func TestTwilioCallbackRejectsWhenUnconfigured(t *testing.T) {
	srv, mux := newTestServer(nil)
	srv.cfg.TwilioAuthToken = "" // not configured

	req := httptest.NewRequest(http.MethodPost, "/api/v1/twilio/callback?incident=1", bytes.NewReader([]byte("CallSid=CA123")))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when twilio not configured, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestSecurityHeadersIncludeCSPAndPermissionsPolicy verifies M3: the backend
// emits Content-Security-Policy and Permissions-Policy on every response
// (ASVS V12.4, SPEC gap M3).
func TestSecurityHeadersIncludeCSPAndPermissionsPolicy(t *testing.T) {
	captured := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := SecurityHeaders(captured)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Fatalf("expected strict CSP, got: %q", csp)
	}
	if pp := rec.Header().Get("Permissions-Policy"); !strings.Contains(pp, "camera=()") {
		t.Fatalf("expected Permissions-Policy disabling features, got: %q", pp)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("expected X-Content-Type-Options nosniff")
	}
}

// TestHSTSEmittedOnHTTPSIndependentOfSecureCookies verifies M3: HSTS is
// emitted on HTTPS requests regardless of the SecureCookies flag (ASVS V12.5,
// SPEC gap M3).
func TestHSTSEmittedOnHTTPSIndependentOfSecureCookies(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	h := StrictTransportSecurity(next)

	// HTTPS request (TLS) -> HSTS present.
	req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	req.TLS = &tls.ConnectionState{} //nolint:exhaustruct // test-only zero value is fine
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if hsts := rec.Header().Get("Strict-Transport-Security"); !strings.Contains(hsts, "max-age") || !strings.Contains(hsts, "preload") {
		t.Fatalf("expected HSTS with preload on HTTPS, got: %q", hsts)
	}

	// HTTPS via X-Forwarded-Proto -> HSTS present.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("X-Forwarded-Proto", "https")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if hsts := rec2.Header().Get("Strict-Transport-Security"); !strings.Contains(hsts, "max-age") {
		t.Fatalf("expected HSTS on X-Forwarded-Proto=https, got: %q", hsts)
	}

	// Plain HTTP -> no HSTS (never emit over HTTP).
	req3 := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	if hsts := rec3.Header().Get("Strict-Transport-Security"); hsts != "" {
		t.Fatalf("HSTS must not be emitted over HTTP, got: %q", hsts)
	}
}
