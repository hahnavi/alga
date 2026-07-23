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

type googleTestUserStore struct {
	users       map[uuid.UUID]*store.UserRecord
	byGoogle    map[string]*store.UserRecord
	updateCalls int
	clearCalls  int
	updateFail  error
	clearFail   error
	getByIDErr  error
}

func newGoogleTestUserStore(users ...*store.UserRecord) *googleTestUserStore {
	m := &googleTestUserStore{
		users:    map[uuid.UUID]*store.UserRecord{},
		byGoogle: map[string]*store.UserRecord{},
	}
	for _, u := range users {
		m.users[u.ID] = u
		if u.GoogleID != "" {
			m.byGoogle[u.GoogleID] = u
		}
	}
	return m
}

func (m *googleTestUserStore) CreateUser(email, password, role string) (*store.UserRecord, error) {
	return nil, nil
}
func (m *googleTestUserStore) GetByEmail(email string) (*store.UserRecord, error) {
	return nil, nil
}
func (m *googleTestUserStore) GetByID(id uuid.UUID) (*store.UserRecord, error) {
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	if u, ok := m.users[id]; ok {
		return u, nil
	}
	return nil, store.ErrUserNotFound
}
func (m *googleTestUserStore) ListUsers() ([]store.UserRecord, error)                { return nil, nil }
func (m *googleTestUserStore) UpdateUser(id uuid.UUID, updates map[string]any) error { return nil }
func (m *googleTestUserStore) DeleteUser(id uuid.UUID) error                         { return nil }
func (m *googleTestUserStore) CountAdmins() (int64, error)                           { return 1, nil }
func (m *googleTestUserStore) Authenticate(email, password string) (*store.UserRecord, error) {
	return nil, nil
}
func (m *googleTestUserStore) RecordFailedLogin(email string) error                    { return nil }
func (m *googleTestUserStore) RecordSuccessfulLogin(userID uuid.UUID, ip string) error { return nil }
func (m *googleTestUserStore) UnlockAccount(userID uuid.UUID) error                    { return nil }
func (m *googleTestUserStore) CountUsers() (int64, error)                              { return int64(len(m.users)), nil }
func (m *googleTestUserStore) GetNotificationPreferences(ctx context.Context, userID string) (map[string]any, error) {
	return map[string]any{}, nil
}
func (m *googleTestUserStore) UpdateNotificationPreferences(ctx context.Context, userID string, prefs map[string]any) error {
	return nil
}
func (m *googleTestUserStore) GetByGoogleID(googleID string) (*store.UserRecord, error) {
	if u, ok := m.byGoogle[googleID]; ok {
		return u, nil
	}
	return nil, nil
}
func (m *googleTestUserStore) GetBySlackUserID(slackUserID string) (*store.UserRecord, error) {
	return nil, nil
}
func (m *googleTestUserStore) UpdateGoogleID(userID uuid.UUID, googleID string) error {
	m.updateCalls++
	if m.updateFail != nil {
		return m.updateFail
	}
	if u, ok := m.users[userID]; ok {
		u.GoogleID = googleID
		m.byGoogle[googleID] = u
	}
	return nil
}
func (m *googleTestUserStore) ClearGoogleID(userID uuid.UUID) error {
	m.clearCalls++
	if m.clearFail != nil {
		return m.clearFail
	}
	if u, ok := m.users[userID]; ok {
		delete(m.byGoogle, u.GoogleID)
		u.GoogleID = ""
	}
	return nil
}
func (m *googleTestUserStore) SetSlackIdentity(ctx context.Context, userID uuid.UUID, slackUserID, slackDisplayName string) error {
	return nil
}
func (m *googleTestUserStore) ClearSlackIdentity(ctx context.Context, userID uuid.UUID) error {
	return nil
}

func newTestUserGoogleHandler(cfg *config.Config, us *googleTestUserStore, audit *mockAuditStore) *userGoogleHandler {
	if cfg == nil {
		cfg = &config.Config{}
	}
	if audit == nil {
		audit = &mockAuditStore{}
	}
	if us == nil {
		us = newGoogleTestUserStore()
	}
	return newUserGoogleHandler(cfg, us, audit, newMemoryOAuthStateStore())
}

func TestUserGoogleAuthorize_MethodNotAllowed(t *testing.T) {
	h := newTestUserGoogleHandler(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me/google/authorize", nil)
	w := httptest.NewRecorder()
	h.handleAuthorize(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestUserGoogleAuthorize_Unauthenticated(t *testing.T) {
	h := newTestUserGoogleHandler(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/google/authorize", nil)
	w := httptest.NewRecorder()
	h.handleAuthorize(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestUserGoogleAuthorize_NotConfigured(t *testing.T) {
	h := newTestUserGoogleHandler(&config.Config{}, nil, nil)
	user := &store.UserRecord{ID: uuid.New(), Email: "user@test.com", Role: "operator"}
	ctx := platform.WithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/google/authorize", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.handleAuthorize(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUserGoogleAuthorize_AlreadyLinked(t *testing.T) {
	cfg := &config.Config{
		GoogleOAuthEnabled: true,
		GoogleClientID:     "client-id",
		GoogleClientSecret: "client-secret",
	}
	user := &store.UserRecord{ID: uuid.New(), Email: "user@test.com", GoogleID: "g-1"}
	us := newGoogleTestUserStore(user)
	h := newTestUserGoogleHandler(cfg, us, nil)
	ctx := platform.WithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/google/authorize", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.handleAuthorize(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "already linked") {
		t.Fatalf("expected 'already linked' message, got %s", w.Body.String())
	}
}

func TestUserGoogleAuthorize_ReturnsAuthURL(t *testing.T) {
	cfg := &config.Config{
		GoogleOAuthEnabled: true,
		GoogleClientID:     "client-id",
		GoogleClientSecret: "client-secret",
	}
	user := &store.UserRecord{ID: uuid.New(), Email: "user@test.com", Role: "operator"}
	us := newGoogleTestUserStore(user)
	h := newTestUserGoogleHandler(cfg, us, nil)

	ctx := platform.WithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/google/authorize", nil).WithContext(ctx)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	h.handleAuthorize(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]string
	if err := decodeResponse(t, w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	authURL, ok := body["url"]
	if !ok {
		t.Fatalf("missing url field")
	}
	if !strings.HasPrefix(authURL, "https://accounts.google.com/o/oauth2/v2/auth?") {
		t.Fatalf("expected google auth URL prefix, got %s", authURL)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if got := parsed.Query().Get("client_id"); got != "client-id" {
		t.Fatalf("expected client_id=client-id, got %s", got)
	}
	state := parsed.Query().Get("state")
	if state == "" {
		t.Fatalf("expected non-empty state")
	}
	if !strings.HasPrefix(state, "user-google:") {
		t.Fatalf("expected state prefix 'user-google:', got %s", state)
	}
	if got := parsed.Query().Get("redirect_uri"); !strings.Contains(got, "/api/v1/users/me/google/callback") {
		t.Fatalf("expected redirect_uri to point at bind callback, got %s", got)
	}
}

func TestUserGoogleAuthorize_AppendsBindSuffixToConfiguredRedirect(t *testing.T) {
	cfg := &config.Config{
		GoogleOAuthEnabled:     true,
		GoogleClientID:         "client-id",
		GoogleClientSecret:     "client-secret",
		GoogleOAuthRedirectURL: "https://app.example.com/api/v1/auth/google",
	}
	user := &store.UserRecord{ID: uuid.New(), Email: "user@test.com", Role: "operator"}
	us := newGoogleTestUserStore(user)
	h := newTestUserGoogleHandler(cfg, us, nil)

	ctx := platform.WithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/google/authorize", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.handleAuthorize(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]string
	if err := decodeResponse(t, w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	parsed, err := url.Parse(body["url"])
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	got := parsed.Query().Get("redirect_uri")
	if got != "https://app.example.com/api/v1/auth/google/bind" {
		t.Fatalf("expected /bind suffix on redirect_uri, got %s", got)
	}
}

func TestUserGoogleCallback_MethodNotAllowed(t *testing.T) {
	h := newTestUserGoogleHandler(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/google/callback", nil)
	w := httptest.NewRecorder()
	h.handleCallback(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestUserGoogleCallback_NotConfigured(t *testing.T) {
	h := newTestUserGoogleHandler(&config.Config{}, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me/google/callback?code=abc&state=user-google:x:y", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	h.handleCallback(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "google_not_configured") {
		t.Fatalf("expected google_not_configured redirect, got %s", w.Header().Get("Location"))
	}
}

func TestUserGoogleCallback_MissingCodeOrState(t *testing.T) {
	cfg := &config.Config{GoogleOAuthEnabled: true, GoogleClientID: "cid", GoogleClientSecret: "csecret"}
	h := newTestUserGoogleHandler(cfg, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me/google/callback?code=abc", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	h.handleCallback(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "missing_code_or_state") {
		t.Fatalf("expected missing_code_or_state, got %s", w.Header().Get("Location"))
	}
}

func TestUserGoogleCallback_InvalidState(t *testing.T) {
	cfg := &config.Config{GoogleOAuthEnabled: true, GoogleClientID: "cid", GoogleClientSecret: "csecret"}
	h := newTestUserGoogleHandler(cfg, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me/google/callback?code=abc&state=garbage", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	h.handleCallback(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "invalid_or_expired_state") {
		t.Fatalf("expected invalid_or_expired_state, got %s", w.Header().Get("Location"))
	}
}

func TestUserGoogleCallback_InvalidStateFormat(t *testing.T) {
	cfg := &config.Config{GoogleOAuthEnabled: true, GoogleClientID: "cid", GoogleClientSecret: "csecret"}
	stateStore := newMemoryOAuthStateStore()
	_ = stateStore.Set("google:abc") // wrong prefix, but valid state token
	h := newUserGoogleHandler(cfg, nil, nil, stateStore)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me/google/callback?code=abc&state=google:abc", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	h.handleCallback(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "invalid_state_format") {
		t.Fatalf("expected invalid_state_format, got %s", w.Header().Get("Location"))
	}
}

func TestUserGoogleCallback_UserNotFound(t *testing.T) {
	cfg := &config.Config{GoogleOAuthEnabled: true, GoogleClientID: "cid", GoogleClientSecret: "csecret"}
	stateStore := newMemoryOAuthStateStore()
	missingID := uuid.New()
	state := "user-google:" + missingID.String() + ":rand"
	_ = stateStore.Set(state)
	h := newUserGoogleHandler(cfg, newGoogleTestUserStore(), nil, stateStore)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me/google/callback?code=abc&state="+url.QueryEscape(state), nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()
	h.handleCallback(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "user_not_found") {
		t.Fatalf("expected user_not_found, got %s", w.Header().Get("Location"))
	}
}

func TestUserGoogleDisconnect_MethodNotAllowed(t *testing.T) {
	h := newTestUserGoogleHandler(nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me/google/disconnect", nil)
	w := httptest.NewRecorder()
	h.handleDisconnect(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestUserGoogleDisconnect_Unauthenticated(t *testing.T) {
	h := newTestUserGoogleHandler(nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/google/disconnect", nil)
	w := httptest.NewRecorder()
	h.handleDisconnect(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestUserGoogleDisconnect_NoOpWhenNotLinked(t *testing.T) {
	user := &store.UserRecord{ID: uuid.New(), Email: "user@test.com", Role: "operator"}
	us := newGoogleTestUserStore(user)
	h := newTestUserGoogleHandler(nil, us, nil)
	ctx := platform.WithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/google/disconnect", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.handleDisconnect(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if us.clearCalls != 0 {
		t.Fatalf("expected no clear call, got %d", us.clearCalls)
	}
}

func TestUserGoogleDisconnect_Success(t *testing.T) {
	user := &store.UserRecord{ID: uuid.New(), Email: "user@test.com", Role: "operator", GoogleID: "g-1"}
	us := newGoogleTestUserStore(user)
	h := newTestUserGoogleHandler(nil, us, nil)
	ctx := platform.WithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/google/disconnect", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.handleDisconnect(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if us.clearCalls != 1 {
		t.Fatalf("expected clear call, got %d", us.clearCalls)
	}
	if user.GoogleID != "" {
		t.Fatalf("expected google_id cleared, got %q", user.GoogleID)
	}
}

func TestUserGoogleDisconnect_StoreError(t *testing.T) {
	user := &store.UserRecord{ID: uuid.New(), Email: "user@test.com", Role: "operator", GoogleID: "g-1"}
	us := newGoogleTestUserStore(user)
	us.clearFail = errStoreBoom
	h := newTestUserGoogleHandler(nil, us, nil)
	ctx := platform.WithUser(context.Background(), user)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/google/disconnect", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	h.handleDisconnect(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

var errStoreBoom = &boomError{}

type boomError struct{}

func (e *boomError) Error() string { return "boom" }
