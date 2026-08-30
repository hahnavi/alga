package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/api/platform"
	"alga/config"
	"alga/store"
)

// sessionAPISessionStore backs the self-service session endpoints in tests.
type sessionAPISessionStore struct {
	sessions []store.SessionRecord
	deleted  []string // id hashes passed to DeleteSessionByIDHash
}

func (s *sessionAPISessionStore) CreateSession(uuid.UUID, string, string) (*store.SessionRecord, error) {
	return nil, nil
}
func (s *sessionAPISessionStore) GetSession(id string) (*store.SessionRecord, error) {
	for i := range s.sessions {
		if s.sessions[i].IDHash == id {
			rec := s.sessions[i]
			return &rec, nil
		}
	}
	return nil, nil
}
func (s *sessionAPISessionStore) GetSessionByRefreshToken(string) (*store.SessionRecord, error) {
	return nil, nil
}
func (s *sessionAPISessionStore) RefreshSession(string, string, string) (*store.SessionRecord, error) {
	return nil, nil
}
func (s *sessionAPISessionStore) RefreshSessionByRefreshToken(string, string, string) (*store.SessionRecord, error) {
	return nil, nil
}
func (s *sessionAPISessionStore) FindRotatedOutSession(string) (*store.SessionRecord, error) {
	return nil, nil
}
func (s *sessionAPISessionStore) DeleteSession(string) error { return nil }
func (s *sessionAPISessionStore) DeleteSessionByIDHash(idHash string) error {
	s.deleted = append(s.deleted, idHash)
	kept := s.sessions[:0]
	for _, sess := range s.sessions {
		if sess.IDHash != idHash {
			kept = append(kept, sess)
		}
	}
	s.sessions = kept
	return nil
}
func (s *sessionAPISessionStore) ListUserSessions(userID uuid.UUID) ([]store.SessionRecord, error) {
	out := make([]store.SessionRecord, 0, len(s.sessions))
	for _, sess := range s.sessions {
		if sess.UserID == userID && sess.ExpiresAt.After(time.Now()) {
			out = append(out, sess)
		}
	}
	return out, nil
}
func (s *sessionAPISessionStore) DeleteAllUserSessions(uuid.UUID) error { return nil }
func (s *sessionAPISessionStore) DeleteExpired(context.Context) (int, error) {
	return 0, nil
}

type sessionAPIUserStore struct {
	store.UserStore
	user *store.UserRecord
}

func (u *sessionAPIUserStore) GetByID(uuid.UUID) (*store.UserRecord, error) { return u.user, nil }

func newSessionAPIServer(t *testing.T, sessions []store.SessionRecord, currentHash string) (*Server, *sessionAPISessionStore, *recordingAuditStore) {
	t.Helper()
	st := &sessionAPISessionStore{sessions: sessions}
	user := &store.UserRecord{ID: uuid.New(), Email: "owner@example.com", Role: "admin"}
	for _, sess := range sessions {
		if sess.IDHash == currentHash {
			user.ID = sess.UserID
		}
	}
	audit := &recordingAuditStore{}
	s := &Server{
		sessionStore: st,
		userStore:    &sessionAPIUserStore{user: user},
		auditStore:   audit,
		ipExtractor:  newIPExtractor(&config.Config{}),
	}
	return s, st, audit
}

// sessionAPIRequest builds an authenticated request whose context carries the
// user and the current session's id digest, mirroring what AuthMiddleware
// injects on the cookie path.
func sessionAPIRequest(method, path string, user *store.UserRecord, currentHash string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	ctx := platform.WithUser(req.Context(), user)
	ctx = platform.WithSessionIDHash(ctx, currentHash)
	return req.WithContext(ctx)
}

func sessionList(t *testing.T, w *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	var payload struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid list payload: %v", err)
	}
	return payload.Items
}

// Listing returns only the caller's sessions and marks exactly one current.
func TestHandleListSessions(t *testing.T) {
	t.Parallel()
	owner := uuid.New()
	now := time.Now()
	sessions := []store.SessionRecord{
		{IDHash: "hash-a", UserID: owner, ExpiresAt: now.Add(time.Hour), LastUsedAt: now.Add(-time.Minute)},
		{IDHash: "hash-b", UserID: owner, ExpiresAt: now.Add(time.Hour), LastUsedAt: now},
		{IDHash: "expired", UserID: owner, ExpiresAt: now.Add(-time.Hour), LastUsedAt: now},
	}
	s, _, _ := newSessionAPIServer(t, sessions, "hash-b")

	req := sessionAPIRequest(http.MethodGet, "/api/v1/auth/sessions", &store.UserRecord{ID: owner, Role: "admin"}, "hash-b")
	w := httptest.NewRecorder()
	s.handleListSessions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	items := sessionList(t, w)
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2 (expired excluded)", len(items))
	}
	currentCount := 0
	for _, item := range items {
		if item["current"] == true {
			currentCount++
		}
		if item["id"] == "expired" {
			t.Fatal("expired session appeared in listing")
		}
	}
	if currentCount != 1 {
		t.Fatalf("current flags = %d, want exactly 1", currentCount)
	}
}

// Listing without a session in context (PAT-authenticated) is rejected.
func TestHandleListSessions_NoSession(t *testing.T) {
	t.Parallel()
	s, _, _ := newSessionAPIServer(t, nil, "")

	w := httptest.NewRecorder()
	s.handleListSessions(w, httptest.NewRequest(http.MethodGet, "/api/v1/auth/sessions", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

// Revoking the current session is rejected and audited.
func TestHandleRevokeSession_CurrentRejected(t *testing.T) {
	t.Parallel()
	owner := uuid.New()
	now := time.Now()
	sessions := []store.SessionRecord{
		{IDHash: "hash-a", UserID: owner, ExpiresAt: now.Add(time.Hour)},
		{IDHash: "hash-b", UserID: owner, ExpiresAt: now.Add(time.Hour)},
	}
	s, st, audit := newSessionAPIServer(t, sessions, "hash-b")

	req := sessionAPIRequest(http.MethodDelete, "/api/v1/auth/sessions/hash-b", &store.UserRecord{ID: owner, Role: "admin"}, "hash-b")
	w := httptest.NewRecorder()
	s.handleRevokeSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if len(st.deleted) != 0 {
		t.Fatalf("deleted = %v, want none", st.deleted)
	}
	if len(audit.events) == 0 || audit.events[len(audit.events)-1] != store.AuditSessionRevoked {
		t.Fatalf("audit events = %v, want session_revoked rejection", audit.events)
	}
}

// Revoking a foreign or unknown session id is a 404 without deleting anything.
func TestHandleRevokeSession_ForeignNotFound(t *testing.T) {
	t.Parallel()
	owner := uuid.New()
	now := time.Now()
	sessions := []store.SessionRecord{
		{IDHash: "hash-a", UserID: owner, ExpiresAt: now.Add(time.Hour)},
	}
	s, st, _ := newSessionAPIServer(t, sessions, "hash-a")

	req := sessionAPIRequest(http.MethodDelete, "/api/v1/auth/sessions/foreign-hash", &store.UserRecord{ID: owner, Role: "admin"}, "hash-a")
	w := httptest.NewRecorder()
	s.handleRevokeSession(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if len(st.deleted) != 0 {
		t.Fatalf("deleted = %v, want none", st.deleted)
	}
}

// Revoking one of the caller's other sessions deletes it and audits success.
func TestHandleRevokeSession_Success(t *testing.T) {
	t.Parallel()
	owner := uuid.New()
	now := time.Now()
	sessions := []store.SessionRecord{
		{IDHash: "hash-a", UserID: owner, ExpiresAt: now.Add(time.Hour)},
		{IDHash: "hash-b", UserID: owner, ExpiresAt: now.Add(time.Hour)},
	}
	s, st, audit := newSessionAPIServer(t, sessions, "hash-a")

	req := sessionAPIRequest(http.MethodDelete, "/api/v1/auth/sessions/hash-b", &store.UserRecord{ID: owner, Role: "admin"}, "hash-a")
	w := httptest.NewRecorder()
	s.handleRevokeSession(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(st.deleted) != 1 || st.deleted[0] != "hash-b" {
		t.Fatalf("deleted = %v, want [hash-b]", st.deleted)
	}
	if len(audit.events) == 0 || audit.events[len(audit.events)-1] != store.AuditSessionRevoked {
		t.Fatalf("audit events = %v, want session_revoked", audit.events)
	}
}

// Revoke-all preserves the current session and returns the revoked count.
func TestHandleRevokeOtherSessions(t *testing.T) {
	t.Parallel()
	owner := uuid.New()
	now := time.Now()
	sessions := []store.SessionRecord{
		{IDHash: "hash-a", UserID: owner, ExpiresAt: now.Add(time.Hour)},
		{IDHash: "hash-b", UserID: owner, ExpiresAt: now.Add(time.Hour)},
		{IDHash: "hash-c", UserID: owner, ExpiresAt: now.Add(time.Hour)},
	}
	s, st, audit := newSessionAPIServer(t, sessions, "hash-a")

	req := sessionAPIRequest(http.MethodDelete, "/api/v1/auth/sessions", &store.UserRecord{ID: owner, Role: "admin"}, "hash-a")
	w := httptest.NewRecorder()
	s.handleRevokeOtherSessions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var payload struct {
		Revoked int `json:"revoked"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	if payload.Revoked != 2 {
		t.Fatalf("revoked = %d, want 2", payload.Revoked)
	}
	for _, h := range st.deleted {
		if h == "hash-a" {
			t.Fatal("current session was deleted")
		}
	}
	if len(st.sessions) != 1 || st.sessions[0].IDHash != "hash-a" {
		t.Fatalf("remaining = %v, want only the current session", st.sessions)
	}
	if len(audit.events) == 0 || audit.events[len(audit.events)-1] != store.AuditSessionsRevokedAll {
		t.Fatalf("audit events = %v, want sessions_revoked_all", audit.events)
	}
}
