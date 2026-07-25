package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"alga/config"
	"alga/routing"
	"alga/store"

	"github.com/google/uuid"
)

type authMockUserStore struct {
	mu           sync.Mutex
	usersByEmail map[string]*store.UserRecord
	usersByID    map[uuid.UUID]*store.UserRecord
	passwords    map[string]string
	locked       map[string]bool
}

func newAuthMockUserStore() *authMockUserStore {
	return &authMockUserStore{
		usersByEmail: make(map[string]*store.UserRecord),
		usersByID:    make(map[uuid.UUID]*store.UserRecord),
		passwords:    make(map[string]string),
		locked:       make(map[string]bool),
	}
}

func (m *authMockUserStore) addUser(email, password, role string) *store.UserRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.New()
	now := time.Now()
	u := &store.UserRecord{
		ID:        id,
		Email:     email,
		Role:      role,
		CreatedAt: now,
		UpdatedAt: now,
	}
	m.usersByEmail[email] = u
	m.usersByID[id] = u
	m.passwords[email] = password
	return u
}

func (m *authMockUserStore) CreateUser(email, password, role string) (*store.UserRecord, error) {
	return m.addUser(email, password, role), nil
}

func (m *authMockUserStore) GetByEmail(email string) (*store.UserRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.usersByEmail[email], nil
}

func (m *authMockUserStore) GetByID(id uuid.UUID) (*store.UserRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.usersByID[id], nil
}

func (m *authMockUserStore) ListUsers() ([]store.UserRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]store.UserRecord, 0, len(m.usersByEmail))
	for _, u := range m.usersByEmail {
		out = append(out, *u)
	}
	return out, nil
}

func (m *authMockUserStore) UpdateUser(id uuid.UUID, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.usersByID[id]
	if !ok {
		return fmt.Errorf("user not found")
	}
	if v, ok := updates["password"].(string); ok {
		m.passwords[u.Email] = v
	}
	if v, ok := updates["email"].(string); ok {
		delete(m.usersByEmail, u.Email)
		m.passwords[v] = m.passwords[u.Email]
		delete(m.passwords, u.Email)
		u.Email = v
		m.usersByEmail[v] = u
	}
	if v, ok := updates["full_name"].(string); ok {
		u.FullName = v
	}
	if v, ok := updates["role"].(string); ok {
		u.Role = v
	}
	return nil
}

func (m *authMockUserStore) DeleteUser(id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.usersByID[id]
	if !ok {
		return nil
	}
	delete(m.usersByID, id)
	delete(m.usersByEmail, u.Email)
	delete(m.passwords, u.Email)
	return nil
}

func (m *authMockUserStore) CountAdmins() (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int64
	for _, u := range m.usersByEmail {
		if u.Role == "admin" {
			count++
		}
	}
	return count, nil
}

func (m *authMockUserStore) Authenticate(email, password string) (*store.UserRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.usersByEmail[email]
	if !ok {
		return nil, store.ErrInvalidCredentials
	}
	if m.locked[email] {
		return nil, store.ErrAccountLocked
	}
	if m.passwords[email] != password {
		return nil, store.ErrInvalidCredentials
	}
	return u, nil
}

func (m *authMockUserStore) RecordFailedLogin(email string) error                    { return nil }
func (m *authMockUserStore) RecordSuccessfulLogin(userID uuid.UUID, ip string) error { return nil }
func (m *authMockUserStore) UnlockAccount(userID uuid.UUID) error                    { return nil }
func (m *authMockUserStore) CountUsers() (int64, error)                              { return int64(len(m.usersByEmail)), nil }
func (m *authMockUserStore) GetNotificationPreferences(ctx context.Context, userID string) (map[string]any, error) {
	return map[string]any{}, nil
}
func (m *authMockUserStore) UpdateNotificationPreferences(ctx context.Context, userID string, prefs map[string]any) error {
	return nil
}
func (m *authMockUserStore) GetByGoogleID(googleID string) (*store.UserRecord, error) {
	return nil, nil
}
func (m *authMockUserStore) GetBySlackUserID(slackUserID string) (*store.UserRecord, error) {
	return nil, nil
}
func (m *authMockUserStore) UpdateGoogleID(userID uuid.UUID, googleID string) error {
	return nil
}
func (m *authMockUserStore) ClearGoogleID(userID uuid.UUID) error {
	return nil
}
func (m *authMockUserStore) SetSlackIdentity(ctx context.Context, userID uuid.UUID, slackUserID, slackDisplayName string) error {
	return nil
}
func (m *authMockUserStore) ClearSlackIdentity(ctx context.Context, userID uuid.UUID) error {
	return nil
}

type authMockSessionStore struct {
	mu       sync.Mutex
	sessions map[string]*store.SessionRecord
	nextID   int
}

func newAuthMockSessionStore() *authMockSessionStore {
	return &authMockSessionStore{
		sessions: make(map[string]*store.SessionRecord),
	}
}

func (m *authMockSessionStore) CreateSession(userID uuid.UUID, ip, userAgent string) (*store.SessionRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	id := fmt.Sprintf("session-%d", m.nextID)
	rec := &store.SessionRecord{
		ID:        id,
		UserID:    userID,
		IP:        ip,
		UserAgent: userAgent,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	m.sessions[id] = rec
	return rec, nil
}

func (m *authMockSessionStore) GetSession(id string) (*store.SessionRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id], nil
}

func (m *authMockSessionStore) GetSessionByRefreshToken(token string) (*store.SessionRecord, error) {
	return nil, nil
}

func (m *authMockSessionStore) RefreshSession(sessionID string, ip, userAgent string) (*store.SessionRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	s.IP = ip
	s.UserAgent = userAgent
	return s, nil
}

func (m *authMockSessionStore) DeleteSession(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	return nil
}

func (m *authMockSessionStore) DeleteAllUserSessions(userID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, s := range m.sessions {
		if s.UserID == userID {
			delete(m.sessions, id)
		}
	}
	return nil
}

func (m *authMockSessionStore) DeleteExpired(_ context.Context) (int, error) { return 0, nil }

type recordingAuditStore struct {
	mu     sync.Mutex
	events []store.AuditRecord
}

func newRecordingAuditStore() *recordingAuditStore {
	return &recordingAuditStore{}
}

func (r *recordingAuditStore) Log(event store.AuditEvent, userID *uuid.UUID, username, ip, userAgent string, success bool, details map[string]any) {
	r.LogEntity(event, userID, username, ip, userAgent, success, details, "", nil)
}

func (r *recordingAuditStore) LogEntity(event store.AuditEvent, userID *uuid.UUID, username, ip, userAgent string, success bool, details map[string]any, entityType string, entityID *uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, store.AuditRecord{
		Event:      event,
		UserID:     userID,
		Username:   username,
		IP:         ip,
		UserAgent:  userAgent,
		Success:    success,
		Details:    details,
		EntityType: entityType,
		EntityID:   entityID,
	})
}

func (r *recordingAuditStore) Query(filter map[string]any) ([]store.AuditRecord, error) {
	return nil, nil
}

func (r *recordingAuditStore) GetRecentEvents(limit int) ([]store.AuditRecord, error) {
	return nil, nil
}

func (r *recordingAuditStore) hasEvent(event store.AuditEvent) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.events {
		if e.Event == event {
			return true
		}
	}
	return false
}

type allowAllLoginLimiter struct{}

func (l *allowAllLoginLimiter) CheckLoginAllowed(ip string) (bool, int, *time.Time) {
	return true, 5, nil
}
func (l *allowAllLoginLimiter) Reset(ip string) {}
func (l *allowAllLoginLimiter) Stop()           {}

type allowAllRateLimiter struct{}

func (l *allowAllRateLimiter) Allow(ip string) bool { return true }
func (l *allowAllRateLimiter) Stop()                {}

type recordingLoginLimiter struct {
	mu          sync.Mutex
	checkCalls  int
	resetCalls  int
	resetIPs    []string
	allow       bool
	remaining   int
	lockedUntil *time.Time
}

func newRecordingLoginLimiter(allow bool) *recordingLoginLimiter {
	return &recordingLoginLimiter{
		allow:     allow,
		remaining: 5,
	}
}

func (l *recordingLoginLimiter) CheckLoginAllowed(ip string) (bool, int, *time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.checkCalls++
	return l.allow, l.remaining, l.lockedUntil
}

func (l *recordingLoginLimiter) Reset(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.resetCalls++
	l.resetIPs = append(l.resetIPs, ip)
}
func (l *recordingLoginLimiter) Stop() {}

func newAuthTestServer(
	userStore store.UserStore,
	sessionStore store.SessionStore,
	auditStore store.AuditStore,
	loginLimiter LoginRateLimiting,
) (*Server, *http.ServeMux) {
	if userStore == nil {
		userStore = newAuthMockUserStore()
	}
	if sessionStore == nil {
		sessionStore = newAuthMockSessionStore()
	}
	if auditStore == nil {
		auditStore = newRecordingAuditStore()
	}
	if loginLimiter == nil {
		loginLimiter = &allowAllLoginLimiter{}
	}
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
		func(*routing.Engine) {},
		loginLimiter,
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

func TestHandleLoginSuccess(t *testing.T) {
	userStore := newAuthMockUserStore()
	seeded := userStore.addUser("admin@alga.local", "P@ssw0rd", "admin")
	seeded.Phone = "+14155551234"
	auditStore := newRecordingAuditStore()
	sessionStore := newAuthMockSessionStore()
	_, mux := newAuthTestServer(userStore, sessionStore, auditStore, nil)

	body := bytes.NewBufferString(`{"email":"admin@alga.local","password":"P@ssw0rd"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := decodeResponse(t, rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["email"] != "admin@alga.local" {
		t.Fatalf("expected email admin@alga.local, got %v", resp["email"])
	}
	if resp["role"] != "admin" {
		t.Fatalf("expected role admin, got %v", resp["role"])
	}
	if resp["phone"] != "+14155551234" {
		t.Fatalf("expected phone +14155551234 in response, got %v", resp["phone"])
	}
	if _, ok := resp["csrf_token"]; !ok {
		t.Fatal("expected csrf_token in response")
	}

	var sessionCookie, csrfCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		switch c.Name {
		case "alga_session":
			sessionCookie = c
		case "alga_csrf":
			csrfCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatal("expected alga_session cookie")
	}
	if csrfCookie == nil {
		t.Fatal("expected alga_csrf cookie")
	}
	if csrfCookie.HttpOnly {
		t.Fatal("csrf cookie should not be HttpOnly")
	}

	if !auditStore.hasEvent(store.AuditLoginSuccess) {
		t.Fatal("expected AuditLoginSuccess audit event")
	}
}

func TestHandleLoginInvalidCredentials(t *testing.T) {
	userStore := newAuthMockUserStore()
	userStore.addUser("admin@alga.local", "P@ssw0rd", "admin")
	auditStore := newRecordingAuditStore()
	_, mux := newAuthTestServer(userStore, nil, auditStore, nil)

	body := bytes.NewBufferString(`{"email":"admin@alga.local","password":"wrong"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	if !auditStore.hasEvent(store.AuditLoginFailed) {
		t.Fatal("expected AuditLoginFailed audit event")
	}
}

func TestHandleLoginMissingFields(t *testing.T) {
	_, mux := newAuthTestServer(nil, nil, nil, nil)

	body := bytes.NewBufferString(`{"email":"admin@alga.local"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandleLoginMethodNotAllowed(t *testing.T) {
	_, mux := newAuthTestServer(nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestHandleLoginRateLimited(t *testing.T) {
	userStore := newAuthMockUserStore()
	userStore.addUser("admin@alga.local", "P@ssw0rd", "admin")
	auditStore := newRecordingAuditStore()
	lockedUntil := time.Now().Add(30 * time.Minute)
	limiter := &recordingLoginLimiter{allow: false, lockedUntil: &lockedUntil}
	_, mux := newAuthTestServer(userStore, nil, auditStore, limiter)

	body := bytes.NewBufferString(`{"email":"admin@alga.local","password":"P@ssw0rd"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}

	if !auditStore.hasEvent(store.AuditLoginFailed) {
		t.Fatal("expected AuditLoginFailed audit event for rate limited login")
	}
}

func TestHandleLoginLimiterResetOnSuccess(t *testing.T) {
	userStore := newAuthMockUserStore()
	userStore.addUser("admin@alga.local", "P@ssw0rd", "admin")
	limiter := newRecordingLoginLimiter(true)
	_, mux := newAuthTestServer(userStore, nil, nil, limiter)

	body := bytes.NewBufferString(`{"email":"admin@alga.local","password":"P@ssw0rd"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if limiter.resetCalls == 0 {
		t.Fatal("expected limiter.Reset to be called on successful login")
	}
}

func TestHandleLogoutSuccess(t *testing.T) {
	userStore := newAuthMockUserStore()
	userStore.addUser("admin@alga.local", "P@ssw0rd", "admin")
	sessionStore := newAuthMockSessionStore()
	auditStore := newRecordingAuditStore()
	_, mux := newAuthTestServer(userStore, sessionStore, auditStore, nil)

	loginBody := bytes.NewBufferString(`{"email":"admin@alga.local","password":"P@ssw0rd"}`)
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", loginBody)
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login: expected 200, got %d", loginRec.Code)
	}

	var loginResp map[string]any
	_ = decodeResponse(t, loginRec.Body.Bytes(), &loginResp)
	csrfToken, _ := loginResp["csrf_token"].(string)

	var sessionCookie, csrfCookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		switch c.Name {
		case "alga_session":
			sessionCookie = c
		case "alga_csrf":
			csrfCookie = c
		}
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutReq.AddCookie(&http.Cookie{Name: "alga_session", Value: sessionCookie.Value})
	logoutReq.AddCookie(&http.Cookie{Name: "alga_csrf", Value: csrfCookie.Value})
	logoutReq.Header.Set("X-CSRF-Token", csrfToken)
	logoutRec := httptest.NewRecorder()
	mux.ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", logoutRec.Code)
	}

	var clearedSession, clearedCSRF bool
	for _, c := range logoutRec.Result().Cookies() {
		if c.Name == "alga_session" && c.MaxAge < 0 {
			clearedSession = true
		}
		if c.Name == "alga_csrf" && c.MaxAge < 0 {
			clearedCSRF = true
		}
	}
	if !clearedSession {
		t.Fatal("expected alga_session cookie to be cleared")
	}
	if !clearedCSRF {
		t.Fatal("expected alga_csrf cookie to be cleared")
	}

	if !auditStore.hasEvent(store.AuditLogout) {
		t.Fatal("expected AuditLogout audit event")
	}
}

func TestHandleLogoutRequiresCSRF(t *testing.T) {
	userStore := newAuthMockUserStore()
	userStore.addUser("admin@alga.local", "P@ssw0rd", "admin")
	_, mux := newAuthTestServer(userStore, nil, nil, nil)

	loginBody := bytes.NewBufferString(`{"email":"admin@alga.local","password":"P@ssw0rd"}`)
	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", loginBody)
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginReq)

	var sessionCookie *http.Cookie
	for _, c := range loginRec.Result().Cookies() {
		if c.Name == "alga_session" {
			sessionCookie = c
		}
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	logoutReq.AddCookie(&http.Cookie{Name: "alga_session", Value: sessionCookie.Value})
	logoutRec := httptest.NewRecorder()
	mux.ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", logoutRec.Code)
	}
}

func TestHandleGetCurrentUser(t *testing.T) {
	userStore := newAuthMockUserStore()
	testUser := userStore.addUser("admin@alga.local", "P@ssw0rd", "admin")
	sessionStore := newAuthMockSessionStore()
	sessionStore.sessions["existing-session"] = &store.SessionRecord{
		ID:        "existing-session",
		UserID:    testUser.ID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	_, mux := newAuthTestServer(userStore, sessionStore, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: "alga_session", Value: "existing-session"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	_ = decodeResponse(t, rec.Body.Bytes(), &resp)
	if resp["email"] != "admin@alga.local" {
		t.Fatalf("expected email admin@alga.local, got %v", resp["email"])
	}
	if resp["role"] != "admin" {
		t.Fatalf("expected role admin, got %v", resp["role"])
	}
}

func TestHandleGetCurrentUserUnauthenticated(t *testing.T) {
	_, mux := newAuthTestServer(nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestHandleRefreshSession(t *testing.T) {
	userStore := newAuthMockUserStore()
	testUser := userStore.addUser("admin@alga.local", "P@ssw0rd", "admin")
	sessionStore := newAuthMockSessionStore()
	sessionStore.sessions["refresh-session"] = &store.SessionRecord{
		ID:        "refresh-session",
		UserID:    testUser.ID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	auditStore := newRecordingAuditStore()
	_, mux := newAuthTestServer(userStore, sessionStore, auditStore, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "alga_session", Value: "refresh-session"})
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if !auditStore.hasEvent(store.AuditSessionRefreshed) {
		t.Fatal("expected AuditSessionRefreshed audit event")
	}
}

func TestValidateCSRFToken(t *testing.T) {
	srv, _ := newAuthTestServer(nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "alga_csrf", Value: "token123"})
	req.Header.Set("X-CSRF-Token", "token123")
	if !srv.validateCSRFToken(req) {
		t.Fatal("expected CSRF validation to pass with matching tokens")
	}

	req2 := httptest.NewRequest(http.MethodPost, "/test", nil)
	req2.AddCookie(&http.Cookie{Name: "alga_csrf", Value: "token123"})
	req2.Header.Set("X-CSRF-Token", "different")
	if srv.validateCSRFToken(req2) {
		t.Fatal("expected CSRF validation to fail with mismatched tokens")
	}

	req3 := httptest.NewRequest(http.MethodPost, "/test", nil)
	req3.AddCookie(&http.Cookie{Name: "alga_csrf", Value: "token123"})
	if srv.validateCSRFToken(req3) {
		t.Fatal("expected CSRF validation to fail with missing header")
	}

	req4 := httptest.NewRequest(http.MethodPost, "/test", nil)
	req4.Header.Set("X-CSRF-Token", "token123")
	if srv.validateCSRFToken(req4) {
		t.Fatal("expected CSRF validation to fail with missing cookie")
	}

	req5 := httptest.NewRequest(http.MethodGet, "/test", nil)
	if !srv.validateCSRFToken(req5) {
		t.Fatal("expected CSRF validation to be skipped for GET")
	}
}

func TestIsStateChangingMethod(t *testing.T) {
	for _, tt := range []struct {
		method string
		want   bool
	}{
		{http.MethodPost, true},
		{http.MethodPut, true},
		{http.MethodPatch, true},
		{http.MethodDelete, true},
		{http.MethodGet, false},
		{http.MethodHead, false},
		{http.MethodOptions, false},
	} {
		if got := isStateChangingMethod(tt.method); got != tt.want {
			t.Errorf("isStateChangingMethod(%q) = %v, want %v", tt.method, got, tt.want)
		}
	}
}

func TestValidatePasswordPolicy(t *testing.T) {
	for _, tt := range []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"valid", "P@ssw0rd", false},
		{"too short", "Ab1!", true},
		{"no upper", "aaaaaa1!", true},
		{"no lower", "AAAAAA1!", true},
		{"no digit", "Aaaaaa!!", true},
		{"no special", "Aaaaaa11", true},
		{"too long (Argon2 DoS bound)", strings.Repeat("A", maxPasswordLength+1) + "a1!", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePasswordPolicy(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("validatePasswordPolicy(%q) error = %v, wantErr %v", tt.password, err, tt.wantErr)
			}
		})
	}
}

func TestHandleChangePasswordWrongCurrent(t *testing.T) {
	userStore := newAuthMockUserStore()
	testUser := userStore.addUser("admin@alga.local", "correctP@ss1!", "admin")
	sessionStore := newAuthMockSessionStore()
	sessionStore.sessions["change-pw-session"] = &store.SessionRecord{
		ID:        "change-pw-session",
		UserID:    testUser.ID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	_, mux := newAuthTestServer(userStore, sessionStore, nil, nil)

	body := bytes.NewBufferString(`{"current_password":"wrongP@ss1!","new_password":"NewP@ssw0rd!"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", body)
	req.AddCookie(&http.Cookie{Name: "alga_session", Value: "change-pw-session"})
	req.AddCookie(&http.Cookie{Name: "alga_csrf", Value: "test-csrf-token"})
	req.Header.Set("X-CSRF-Token", "test-csrf-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleChangePasswordPolicyViolation(t *testing.T) {
	userStore := newAuthMockUserStore()
	testUser := userStore.addUser("admin@alga.local", "correctP@ss1!", "admin")
	sessionStore := newAuthMockSessionStore()
	sessionStore.sessions["change-pw-session"] = &store.SessionRecord{
		ID:        "change-pw-session",
		UserID:    testUser.ID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	_, mux := newAuthTestServer(userStore, sessionStore, nil, nil)

	body := bytes.NewBufferString(`{"current_password":"correctP@ss1!","new_password":"weak"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/change-password", body)
	req.AddCookie(&http.Cookie{Name: "alga_session", Value: "change-pw-session"})
	req.AddCookie(&http.Cookie{Name: "alga_csrf", Value: "test-csrf-token"})
	req.Header.Set("X-CSRF-Token", "test-csrf-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body: %s", rec.Code, rec.Body.String())
	}
}
