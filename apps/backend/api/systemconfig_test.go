package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"alga/config"
	"alga/store"
)

type capturingSystemConfigStore struct {
	mu  sync.Mutex
	cfg store.SystemConfigValues
}

func (m *capturingSystemConfigStore) Get() (*store.SystemConfigValues, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := m.cfg
	return &cp, nil
}

func (m *capturingSystemConfigStore) Save(cfg store.SystemConfigValues) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
	return nil
}

func (m *capturingSystemConfigStore) saved() store.SystemConfigValues {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

func newSystemConfigTestServer(t *testing.T, cfg *config.Config) (*Server, *httptest.ResponseRecorder, *http.ServeMux, *capturingSystemConfigStore) {
	t.Helper()
	sysCfgStore := &capturingSystemConfigStore{}
	srv, mux := newOnboardingTestServer(sysCfgStore)
	if cfg != nil {
		srv.mu.Lock()
		srv.cfg = cfg
		srv.mu.Unlock()
	}
	return srv, httptest.NewRecorder(), mux, sysCfgStore
}

func putSystemConfig(t *testing.T, mux *http.ServeMux, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := authRequest(http.MethodPut, "/api/v1/system/config", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestGetSystemConfig_IncludesIncidentSummary(t *testing.T) {
	cfg := &config.Config{
		IncidentSummaryEnabled:  true,
		IncidentSummaryInterval: 10 * time.Minute,
		IncidentSummaryIntervals: map[string]time.Duration{
			"critical": 5 * time.Minute,
		},
	}
	_, _, mux, _ := newSystemConfigTestServer(t, cfg)

	req := authRequest(http.MethodGet, "/api/v1/system/config", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := decodeResponse(t, rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["incident_summary_enabled"] != true {
		t.Fatalf("expected incident_summary_enabled=true, got %v", resp["incident_summary_enabled"])
	}
	if resp["incident_summary_interval"] != "10m0s" {
		t.Fatalf("expected incident_summary_interval=10m0s, got %v", resp["incident_summary_interval"])
	}
	intervals, ok := resp["incident_summary_intervals"].(map[string]any)
	if !ok {
		t.Fatalf("expected incident_summary_intervals object, got %T", resp["incident_summary_intervals"])
	}
	if intervals["critical"] != "5m0s" {
		t.Fatalf("expected critical=5m0s, got %v", intervals["critical"])
	}
}

func TestPutSystemConfig_IncidentSummaryRoundTrip(t *testing.T) {
	srv, _, mux, sysCfgStore := newSystemConfigTestServer(t, &config.Config{})

	var appliedEnabled bool
	var appliedInterval time.Duration
	var appliedSeverity map[string]time.Duration
	srv.SetSummaryConfigApplier(func(enabled bool, defaultInterval time.Duration, severityIntervals map[string]time.Duration) {
		appliedEnabled = enabled
		appliedInterval = defaultInterval
		appliedSeverity = severityIntervals
	})

	rec := putSystemConfig(t, mux, `{"incident_summary_enabled":true,"incident_summary_interval":"12m","incident_summary_intervals":{"critical":"4m","high":"8m"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	srv.mu.RLock()
	defer srv.mu.RUnlock()
	if !srv.cfg.IncidentSummaryEnabled {
		t.Fatal("expected cfg.IncidentSummaryEnabled=true")
	}
	if srv.cfg.IncidentSummaryInterval != 12*time.Minute {
		t.Fatalf("expected cfg.IncidentSummaryInterval=12m, got %v", srv.cfg.IncidentSummaryInterval)
	}
	if srv.cfg.IncidentSummaryIntervals["critical"] != 4*time.Minute {
		t.Fatalf("expected critical=4m, got %v", srv.cfg.IncidentSummaryIntervals["critical"])
	}

	saved := sysCfgStore.saved()
	if !saved.IncidentSummaryEnabled {
		t.Fatal("expected persisted IncidentSummaryEnabled=true")
	}
	if saved.IncidentSummaryInterval != "12m0s" {
		t.Fatalf("expected persisted interval=12m0s, got %q", saved.IncidentSummaryInterval)
	}
	if saved.IncidentSummaryIntervals["critical"] != "4m0s" {
		t.Fatalf("expected persisted critical=4m0s, got %v", saved.IncidentSummaryIntervals["critical"])
	}

	if !appliedEnabled || appliedInterval != 12*time.Minute {
		t.Fatalf("expected live apply enabled=true interval=12m, got enabled=%v interval=%v", appliedEnabled, appliedInterval)
	}
	if appliedSeverity["high"] != 8*time.Minute {
		t.Fatalf("expected live apply high=8m, got %v", appliedSeverity["high"])
	}
}

func TestPutSystemConfig_IncidentSummaryInvalidInterval(t *testing.T) {
	_, _, mux, _ := newSystemConfigTestServer(t, &config.Config{})

	rec := putSystemConfig(t, mux, `{"incident_summary_interval":"not-a-duration"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestPutSystemConfig_IncidentSummaryInvalidIntervalsValue(t *testing.T) {
	_, _, mux, _ := newSystemConfigTestServer(t, &config.Config{})

	rec := putSystemConfig(t, mux, `{"incident_summary_intervals":{"critical":123}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestPutSystemConfig_DoesNotWipeIncidentSummary locks in the fix for the
// latent bug where saving any field reset incident-summary config because the
// persisted dbCfg omitted it.
func TestPutSystemConfig_DoesNotWipeIncidentSummary(t *testing.T) {
	cfg := &config.Config{
		IncidentSummaryEnabled:  true,
		IncidentSummaryInterval: 10 * time.Minute,
	}
	_, _, mux, sysCfgStore := newSystemConfigTestServer(t, cfg)
	sysCfgStore.cfg.IncidentSummaryEnabled = true
	sysCfgStore.cfg.IncidentSummaryInterval = "10m0s"

	rec := putSystemConfig(t, mux, `{"log_level":"warn"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	saved := sysCfgStore.saved()
	if !saved.IncidentSummaryEnabled {
		t.Fatal("saving log_level wiped incident_summary_enabled")
	}
	if saved.IncidentSummaryInterval != "10m0s" {
		t.Fatalf("saving log_level wiped incident_summary_interval, got %q", saved.IncidentSummaryInterval)
	}
}

func TestPutSystemConfig_TriggerStatusValidation(t *testing.T) {
	t.Run("rejected", func(t *testing.T) {
		_, _, mux, _ := newSystemConfigTestServer(t, &config.Config{})
		rec := putSystemConfig(t, mux, `{"slack_incident_channel_trigger_status":"open"}`)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for open, got %d: %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("accepted", func(t *testing.T) {
		_, _, mux, _ := newSystemConfigTestServer(t, &config.Config{})
		rec := putSystemConfig(t, mux, `{"slack_incident_channel_trigger_status":"detected"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for detected, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestPutSystemConfig_UpdatesTimestamp(t *testing.T) {
	_, _, mux, _ := newSystemConfigTestServer(t, &config.Config{})

	// First GET should omit updated_at because nothing has been saved yet.
	req := authRequest(http.MethodGet, "/api/v1/system/config", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var first map[string]any
	if err := decodeResponse(t, rec.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := first["updated_at"]; ok {
		t.Fatalf("expected updated_at to be absent before any save, got %v", first["updated_at"])
	}

	// Saving any field should stamp updated_at.
	saveRec := putSystemConfig(t, mux, `{"log_level":"warn"}`)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", saveRec.Code, saveRec.Body.String())
	}

	req2 := authRequest(http.MethodGet, "/api/v1/system/config", nil)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec2.Code, rec2.Body.String())
	}
	var second map[string]any
	if err := decodeResponse(t, rec2.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode: %v", err)
	}
	raw, ok := second["updated_at"]
	if !ok {
		t.Fatal("expected updated_at to be present after a save")
	}
	parsed, err := time.Parse(time.RFC3339, raw.(string))
	if err != nil {
		t.Fatalf("expected updated_at to be RFC3339, got %v: %v", raw, err)
	}
	if parsed.IsZero() || time.Since(parsed) > 5*time.Second {
		t.Fatalf("expected updated_at to be recent, got %v", parsed)
	}
}

func TestGetSystemConfig_AuthSecretsMasked(t *testing.T) {
	installAPITestCrypto(t)
	cfg := &config.Config{
		GoogleClientID:     "google-client-id",
		GoogleClientSecret: "super-secret-value",
		OIDCClientID:       "oidc-client-id",
		OIDCClientSecret:   "oidc-secret-value",
	}
	_, _, mux, _ := newSystemConfigTestServer(t, cfg)

	req := authRequest(http.MethodGet, "/api/v1/system/config", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	if strings.Contains(body, "super-secret-value") {
		t.Fatal("Google client secret must not appear in GET response")
	}
	if strings.Contains(body, "oidc-secret-value") {
		t.Fatal("OIDC client secret must not appear in GET response")
	}

	var resp map[string]any
	if err := decodeResponse(t, rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["google_client_secret_set"] != true {
		t.Fatalf("expected google_client_secret_set=true, got %v", resp["google_client_secret_set"])
	}
	if resp["oidc_client_secret_set"] != true {
		t.Fatalf("expected oidc_client_secret_set=true, got %v", resp["oidc_client_secret_set"])
	}
	if resp["google_client_id"] != "google-client-id" {
		t.Fatalf("expected google_client_id=google-client-id, got %v", resp["google_client_id"])
	}
	if _, ok := resp["google_client_secret"]; ok {
		t.Fatal("google_client_secret must not be present in GET response")
	}
	if _, ok := resp["oidc_client_secret"]; ok {
		t.Fatal("oidc_client_secret must not be present in GET response")
	}
}

func TestPutSystemConfig_AuthRoundTrip(t *testing.T) {
	installAPITestCrypto(t)
	srv, _, mux, sysCfgStore := newSystemConfigTestServer(t, &config.Config{})

	rec := putSystemConfig(t, mux, `{
		"google_oauth_enabled": true,
		"google_client_id": "g-id",
		"google_client_secret": "g-secret",
		"google_oauth_redirect_url": "https://alga.example/callback",
		"oidc_enabled": true,
		"oidc_issuer_url": "https://issuer.example",
		"oidc_client_id": "o-id",
		"oidc_client_secret": "o-secret",
		"oidc_scopes": "openid email"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// In-memory config holds plaintext for runtime use.
	srv.mu.RLock()
	defer srv.mu.RUnlock()
	if srv.cfg.GoogleClientSecret != "g-secret" {
		t.Fatalf("expected cfg.GoogleClientSecret=g-secret, got %q", srv.cfg.GoogleClientSecret)
	}
	if srv.cfg.OIDCClientSecret != "o-secret" {
		t.Fatalf("expected cfg.OIDCClientSecret=o-secret, got %q", srv.cfg.OIDCClientSecret)
	}
	if srv.cfg.GoogleClientID != "g-id" {
		t.Fatalf("expected cfg.GoogleClientID=g-id, got %q", srv.cfg.GoogleClientID)
	}
	if !srv.cfg.OIDCEnabled {
		t.Fatal("expected cfg.OIDCEnabled=true")
	}
	if srv.cfg.OIDCIssuerURL != "https://issuer.example" {
		t.Fatalf("expected cfg.OIDCIssuerURL, got %q", srv.cfg.OIDCIssuerURL)
	}
	if srv.cfg.OIDCScopes != "openid email" {
		t.Fatalf("expected cfg.OIDCScopes=openid email, got %q", srv.cfg.OIDCScopes)
	}

	// Persisted store must hold ciphertext, not plaintext.
	saved := sysCfgStore.saved()
	if saved.GoogleClientSecretEnc == "" || strings.Contains(saved.GoogleClientSecretEnc, "g-secret") {
		t.Fatalf("expected encrypted Google secret, got %q", saved.GoogleClientSecretEnc)
	}
	if saved.OIDCClientSecretEnc == "" || strings.Contains(saved.OIDCClientSecretEnc, "o-secret") {
		t.Fatalf("expected encrypted OIDC secret, got %q", saved.OIDCClientSecretEnc)
	}
	if saved.GoogleClientID != "g-id" {
		t.Fatalf("expected persisted google_client_id=g-id, got %q", saved.GoogleClientID)
	}
	if saved.OIDCIssuerURL != "https://issuer.example" {
		t.Fatalf("expected persisted oidc_issuer_url, got %q", saved.OIDCIssuerURL)
	}
}

func TestPutSystemConfig_AuthEmptySecretPreserved(t *testing.T) {
	installAPITestCrypto(t)
	srv, _, mux, _ := newSystemConfigTestServer(t, &config.Config{
		GoogleClientSecret: "existing-secret",
		OIDCClientSecret:   "existing-oidc",
	})

	// Saving other auth fields with an empty secret must not clear the existing secret.
	rec := putSystemConfig(t, mux, `{"google_client_id":"new-id","google_client_secret":"","oidc_client_id":"new-oidc-id"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	srv.mu.RLock()
	defer srv.mu.RUnlock()
	if srv.cfg.GoogleClientSecret != "existing-secret" {
		t.Fatalf("expected GoogleClientSecret preserved, got %q", srv.cfg.GoogleClientSecret)
	}
	if srv.cfg.OIDCClientSecret != "existing-oidc" {
		t.Fatalf("expected OIDCClientSecret preserved, got %q", srv.cfg.OIDCClientSecret)
	}
	if srv.cfg.GoogleClientID != "new-id" {
		t.Fatalf("expected GoogleClientID=new-id, got %q", srv.cfg.GoogleClientID)
	}
	if srv.cfg.OIDCClientID != "new-oidc-id" {
		t.Fatalf("expected OIDCClientID=new-oidc-id, got %q", srv.cfg.OIDCClientID)
	}
}
