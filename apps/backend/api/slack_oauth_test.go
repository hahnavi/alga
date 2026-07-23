package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"alga/config"
	"alga/store"
)

func newTestOAuthHandler(cfg *config.Config, integrationStore store.IntegrationStore) *slackOAuthHandler {
	if cfg == nil {
		cfg = &config.Config{}
	}
	return &slackOAuthHandler{
		cfg:              cfg,
		integrationStore: integrationStore,
		slackClient:      &noopSlackReconfigurable{},
		stateStore:       newMemoryOAuthStateStore(),
		rebuildFn:        func() {},
	}
}

type noopSlackReconfigurable struct{}

func (n *noopSlackReconfigurable) Reconfigure(botToken string) {}

func TestMemoryOAuthStateStore(t *testing.T) {
	s := newMemoryOAuthStateStore()

	err := s.Set("test-state")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	valid, err := s.Validate("test-state")
	if err != nil || !valid {
		t.Fatalf("expected valid=true, got valid=%v err=%v", valid, err)
	}

	valid, err = s.Validate("test-state")
	if err != nil || valid {
		t.Fatalf("expected valid=false (single-use), got valid=%v err=%v", valid, err)
	}
}

func TestMemoryOAuthStateStore_UnknownState(t *testing.T) {
	s := newMemoryOAuthStateStore()

	valid, err := s.Validate("nonexistent")
	if err != nil || valid {
		t.Fatalf("expected valid=false for unknown state, got valid=%v err=%v", valid, err)
	}
}

func TestHandleAuthorize_MissingCredentials(t *testing.T) {
	h := newTestOAuthHandler(nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/slack/oauth/authorize", nil)
	w := httptest.NewRecorder()
	h.handleAuthorize(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleAuthorize_ReturnsAuthURL(t *testing.T) {
	cfg := &config.Config{
		SlackClientID:     "test-client-id",
		SlackClientSecret: "test-client-secret",
	}
	h := newTestOAuthHandler(cfg, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/slack/oauth/authorize", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	h.handleAuthorize(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	if err := decodeResponse(t, w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	authURL, ok := body["url"]
	if !ok {
		t.Fatalf("expected 'url' field in response")
	}

	if !strings.HasPrefix(authURL, "https://slack.com/oauth/v2/authorize?") {
		t.Fatalf("expected URL to start with slack OAuth prefix, got %s", authURL)
	}

	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}

	if got := parsed.Query().Get("client_id"); got != "test-client-id" {
		t.Fatalf("expected client_id=test-client-id, got %s", got)
	}

	if got := parsed.Query().Get("state"); got == "" {
		t.Fatalf("expected non-empty state parameter")
	}

	if got := parsed.Query().Get("scope"); got == "" {
		t.Fatalf("expected non-empty scope parameter")
	} else if !strings.Contains(got, "chat:write.customize") {
		t.Fatalf("expected scope to include chat:write.customize, got %s", got)
	}
}

func TestHandleAuthorize_MethodNotAllowed(t *testing.T) {
	h := newTestOAuthHandler(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/slack/oauth/authorize", nil)
	w := httptest.NewRecorder()
	h.handleAuthorize(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleCallback_SlackError(t *testing.T) {
	h := newTestOAuthHandler(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/slack/oauth/callback?error=access_denied", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	h.handleCallback(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "slack_oauth=error") {
		t.Fatalf("expected redirect to contain slack_oauth=error, got %s", loc)
	}
	if !strings.Contains(loc, "message=access_denied") {
		t.Fatalf("expected redirect to contain message=access_denied, got %s", loc)
	}
}

func TestHandleCallback_InvalidState(t *testing.T) {
	h := newTestOAuthHandler(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/slack/oauth/callback?code=abc&state=invalid", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	h.handleCallback(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "slack_oauth=error") {
		t.Fatalf("expected redirect to contain slack_oauth=error, got %s", loc)
	}
	if !strings.Contains(loc, "message=invalid_or_expired_state") {
		t.Fatalf("expected redirect to contain message=invalid_or_expired_state, got %s", loc)
	}
}

func TestHandleCallback_MissingCodeOrState(t *testing.T) {
	h := newTestOAuthHandler(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/slack/oauth/callback?code=abc", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	h.handleCallback(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "message=missing_code_or_state") {
		t.Fatalf("expected missing_code_or_state, got %s", loc)
	}
}

func TestHandleCallback_MethodNotAllowed(t *testing.T) {
	h := newTestOAuthHandler(nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/slack/oauth/callback", nil)
	w := httptest.NewRecorder()
	h.handleCallback(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleCallback_StateSingleUse(t *testing.T) {
	cfg := &config.Config{
		SlackClientID:     "cid",
		SlackClientSecret: "csecret",
	}
	h := newTestOAuthHandler(cfg, nil)

	authReq := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/slack/oauth/authorize", nil)
	authReq.Host = "localhost:8080"
	authW := httptest.NewRecorder()
	h.handleAuthorize(authW, authReq)

	var authBody map[string]string
	if err := decodeResponse(t, authW.Body.Bytes(), &authBody); err != nil {
		t.Fatalf("decode auth response: %v", err)
	}

	parsed, err := url.Parse(authBody["url"])
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	state := parsed.Query().Get("state")

	cbReq1 := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/slack/oauth/callback?code=abc&state="+state, nil)
	cbReq1.Host = "localhost:8080"
	cbW1 := httptest.NewRecorder()
	h.handleCallback(cbW1, cbReq1)

	loc1 := cbW1.Header().Get("Location")
	if strings.Contains(loc1, "invalid_or_expired_state") {
		t.Fatalf("first validate should succeed, got %s", loc1)
	}

	cbReq2 := httptest.NewRequest(http.MethodGet, "/api/v1/integrations/slack/oauth/callback?code=abc&state="+state, nil)
	cbReq2.Host = "localhost:8080"
	cbW2 := httptest.NewRecorder()
	h.handleCallback(cbW2, cbReq2)

	loc2 := cbW2.Header().Get("Location")
	if !strings.Contains(loc2, "invalid_or_expired_state") {
		t.Fatalf("second validate should fail (single-use), got %s", loc2)
	}
}

func TestHandleAuthorize_UsesIntegrationStoreCredentials(t *testing.T) {
	cfg := &config.Config{}
	is := &mockIntegrationStore{
		cfg: &store.IntegrationConfig{
			SlackClientID:     "stored-id",
			SlackClientSecret: "stored-secret",
		},
	}
	h := newTestOAuthHandler(cfg, is)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/integrations/slack/oauth/authorize", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	h.handleAuthorize(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	if err := decodeResponse(t, w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	parsed, err := url.Parse(body["url"])
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}

	if got := parsed.Query().Get("client_id"); got != "stored-id" {
		t.Fatalf("expected client_id from integration store, got %s", got)
	}
}
