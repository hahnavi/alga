package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"alga/api/platform"
	"alga/config"
	"alga/store"
)

func newTestUserSlackHandler(cfg *config.Config, userStore store.UserStore, integrationStore store.IntegrationStore, auditStore store.AuditStore) *userSlackHandler {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if auditStore == nil {
		auditStore = &mockAuditStore{}
	}
	return newUserSlackHandler(cfg, userStore, integrationStore, auditStore, newMemoryOAuthStateStore())
}

func TestUserSlackAuthorize_MethodNotAllowed(t *testing.T) {
	h := newTestUserSlackHandler(nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me/slack/authorize", nil)
	w := httptest.NewRecorder()
	h.handleAuthorize(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestUserSlackAuthorize_Unauthenticated(t *testing.T) {
	h := newTestUserSlackHandler(nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/slack/authorize", nil)
	w := httptest.NewRecorder()
	h.handleAuthorize(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestUserSlackAuthorize_WorkspaceNotConnected(t *testing.T) {
	cfg := &config.Config{}
	is := &mockIntegrationStore{cfg: &store.IntegrationConfig{}}
	h := newTestUserSlackHandler(cfg, nil, is, nil)

	user := &store.UserRecord{ID: uuid.New(), Email: "user@test.com", Role: "operator"}
	ctx := platform.WithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/slack/authorize", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.handleAuthorize(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUserSlackAuthorize_ReturnsAuthURL(t *testing.T) {
	cfg := &config.Config{
		SlackClientID:     "test-client-id",
		SlackClientSecret: "test-client-secret",
		SlackBotToken:     "xoxb-test-token",
	}
	is := &mockIntegrationStore{cfg: &store.IntegrationConfig{}}
	h := newTestUserSlackHandler(cfg, nil, is, nil)

	user := &store.UserRecord{ID: uuid.New(), Email: "user@test.com", Role: "operator"}
	ctx := platform.WithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/slack/authorize", nil).WithContext(ctx)
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

	if !strings.HasPrefix(authURL, "https://slack.com/oauth/authorize?") {
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

	if !strings.HasPrefix(parsed.Query().Get("state"), "user-slack:") {
		t.Fatalf("expected state to start with 'user-slack:', got %s", parsed.Query().Get("state"))
	}

	if got := parsed.Query().Get("scope"); got != "identity.basic" {
		t.Fatalf("expected scope=identity.basic, got %s", got)
	}
}

func TestUserSlackAuthorize_ReturnsAuthURLFromIntegrationStore(t *testing.T) {
	cfg := &config.Config{
		SlackBotToken: "xoxb-test-token",
	}
	is := &mockIntegrationStore{cfg: &store.IntegrationConfig{
		SlackClientID:     "stored-id",
		SlackClientSecret: "stored-secret",
		SlackBotToken:     "xoxb-stored-token",
	}}
	h := newTestUserSlackHandler(cfg, nil, is, nil)

	user := &store.UserRecord{ID: uuid.New(), Email: "user@test.com", Role: "operator"}
	ctx := platform.WithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/slack/authorize", nil).WithContext(ctx)
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

func TestUserSlackCallback_MethodNotAllowed(t *testing.T) {
	h := newTestUserSlackHandler(nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/slack/callback", nil)
	w := httptest.NewRecorder()
	h.handleCallback(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestUserSlackCallback_MissingCodeOrState(t *testing.T) {
	h := newTestUserSlackHandler(nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me/slack/callback?code=abc", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	h.handleCallback(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "missing_code_or_state") {
		t.Fatalf("expected missing_code_or_state, got %s", loc)
	}
}

func TestUserSlackCallback_InvalidState(t *testing.T) {
	h := newTestUserSlackHandler(nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me/slack/callback?code=abc&state=invalid", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	h.handleCallback(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "invalid_or_expired_state") {
		t.Fatalf("expected invalid_or_expired_state, got %s", loc)
	}
}

func TestUserSlackDisconnect_MethodNotAllowed(t *testing.T) {
	h := newTestUserSlackHandler(nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me/slack/disconnect", nil)
	w := httptest.NewRecorder()
	h.handleDisconnect(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestUserSlackDisconnect_Unauthenticated(t *testing.T) {
	h := newTestUserSlackHandler(nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/slack/disconnect", nil)
	w := httptest.NewRecorder()
	h.handleDisconnect(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestUserSlackDisconnect_Success(t *testing.T) {
	us := &mockUserStore{}
	h := newTestUserSlackHandler(nil, us, nil, nil)

	user := &store.UserRecord{ID: uuid.New(), Email: "user@test.com", Role: "operator"}
	ctx := platform.WithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/slack/disconnect", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.handleDisconnect(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	if err := decodeResponse(t, w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body["status"] != "disconnected" {
		t.Fatalf("expected status=disconnected, got %s", body["status"])
	}
}
