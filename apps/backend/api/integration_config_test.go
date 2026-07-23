package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"alga/config"
	"alga/routing"
	"alga/store"
)

// newIntegrationTestServer builds a Server with a mutable integration store so
// PUT behavior can be asserted. It mirrors newOnboardingTestServer but injects
// the provided integration store and config.
func newIntegrationTestServer(t *testing.T, integ *mockIntegrationStore, cfg *config.Config) (*Server, *http.ServeMux) {
	t.Helper()
	userStore := &mockUserStore{users: []store.UserRecord{testAdminUser}}
	sessionStore := &mockSessionStore{
		sessions: map[string]*store.SessionRecord{
			"test-session-id": {ID: "test-session-id", UserID: testAdminUser.ID, ExpiresAt: time.Now().Add(24 * time.Hour)},
		},
	}
	if integ == nil {
		integ = &mockIntegrationStore{}
	}
	srv := NewServer(
		cfg,
		&mockStore{},
		&mockWebhookTokenStore{tokens: map[string]store.WebhookTokenRecord{}},
		&mockAgentTokenStore{},
		userStore,
		sessionStore,
		&mockAuditStore{},
		integ,
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
	mux := http.NewServeMux()
	srv.Register(mux)
	return srv, mux
}

func putIntegrations(t *testing.T, mux *http.ServeMux, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := authRequest(http.MethodPut, "/api/v1/integrations", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// TestPutIntegrations_RejectEnablingInactiveProvider verifies that enabling the
// provider that is not the effective voice provider is rejected.
func TestPutIntegrations_RejectEnablingInactiveProvider(t *testing.T) {
	integ := &mockIntegrationStore{}
	cfg := &config.Config{VoiceProvider: "twilio"}
	_, mux := newIntegrationTestServer(t, integ, cfg)

	// Telnyx is not the active provider; enabling it must fail.
	rec := putIntegrations(t, mux, `{"telnyx":{"provider_enabled":true}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when enabling inactive provider, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestPutIntegrations_ForcesInactiveProviderDisabled verifies that a successful
// save of the active provider leaves the inactive provider disabled in the row.
func TestPutIntegrations_ForcesInactiveProviderDisabled(t *testing.T) {
	integ := &mockIntegrationStore{cfg: &store.IntegrationConfig{
		TelnyxDisabled: false, // stale: telnyx enabled while not active
	}}
	cfg := &config.Config{VoiceProvider: "twilio"}
	_, mux := newIntegrationTestServer(t, integ, cfg)

	// Save twilio (active) without touching telnyx enable flag. The backend
	// should keep telnyx disabled in the persisted row even though existing
	// had it enabled.
	rec := putIntegrations(t, mux, `{"twilio":{"provider_enabled":true}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	saved := integ.cfg
	if saved == nil {
		t.Fatal("expected saved config, got nil")
	}
	if !saved.TelnyxDisabled {
		t.Errorf("expected inactive telnyx provider to be forced disabled")
	}
	if saved.TwilioDisabled {
		t.Errorf("expected active twilio provider to remain enabled")
	}
	if saved.VoiceProvider != "twilio" {
		t.Errorf("expected voice_provider=twilio, got %q", saved.VoiceProvider)
	}
}

// TestPutIntegrations_SwitchVoiceProvider verifies that switching the unlocked
// voice_provider persists the new value and disables the newly-inactive
// provider.
func TestPutIntegrations_SwitchVoiceProvider(t *testing.T) {
	integ := &mockIntegrationStore{cfg: &store.IntegrationConfig{}}
	cfg := &config.Config{VoiceProvider: "twilio"}
	_, mux := newIntegrationTestServer(t, integ, cfg)

	rec := putIntegrations(t, mux, `{"voice_provider":"telnyx"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	saved := integ.cfg
	if saved == nil {
		t.Fatal("expected saved config, got nil")
	}
	if saved.VoiceProvider != "telnyx" {
		t.Errorf("expected voice_provider=telnyx, got %q", saved.VoiceProvider)
	}
	if !saved.TwilioDisabled {
		t.Errorf("expected twilio forced disabled after switching to telnyx")
	}
}

// TestGetIntegrations_ReportsActiveProvider verifies the GET response surfaces
// the active flag per provider and the resolved voice_provider.
func TestGetIntegrations_ReportsActiveProvider(t *testing.T) {
	cfg := &config.Config{VoiceProvider: "telnyx"}
	_, mux := newIntegrationTestServer(t, nil, cfg)

	req := authRequest(http.MethodGet, "/api/v1/integrations", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := decodeResponse(t, rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["voice_provider"] != "telnyx" {
		t.Errorf("expected voice_provider=telnyx, got %v", resp["voice_provider"])
	}
	twilio, _ := resp["twilio"].(map[string]any)
	if twilio["active"] != false {
		t.Errorf("expected twilio active=false, got %v", twilio["active"])
	}
	telnyx, _ := resp["telnyx"].(map[string]any)
	if telnyx["active"] != true {
		t.Errorf("expected telnyx active=true, got %v", telnyx["active"])
	}
}

// TestPutIntegrations_VoiceProviderEnvLocked verifies that VOICE_PROVIDER env
// var locks the selector: attempts to change it via the API are rejected.
func TestPutIntegrations_VoiceProviderEnvLocked(t *testing.T) {
	t.Setenv("VOICE_PROVIDER", "twilio")
	integ := &mockIntegrationStore{cfg: &store.IntegrationConfig{}}
	cfg := &config.Config{VoiceProvider: "twilio"}
	_, mux := newIntegrationTestServer(t, integ, cfg)

	rec := putIntegrations(t, mux, `{"voice_provider":"telnyx"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 when changing env-locked voice_provider, got %d: %s", rec.Code, rec.Body.String())
	}
}
