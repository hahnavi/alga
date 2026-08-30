package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"alga/config"
	"alga/store"
)

// replaySessionStore scripts the session-store outcomes behind the refresh
// handler's reuse-detection branches and records family revocations.
type replaySessionStore struct {
	refreshResult *store.SessionRecord
	rotatedOut    *store.SessionRecord
	byRTResult    *store.SessionRecord
	byRTErr       error

	revokedUsers []uuid.UUID
}

func (s *replaySessionStore) CreateSession(uuid.UUID, string, string) (*store.SessionRecord, error) {
	return nil, nil
}
func (s *replaySessionStore) GetSession(string) (*store.SessionRecord, error) { return nil, nil }
func (s *replaySessionStore) GetSessionByRefreshToken(string) (*store.SessionRecord, error) {
	return nil, nil
}
func (s *replaySessionStore) RefreshSession(string, string, string) (*store.SessionRecord, error) {
	return s.refreshResult, nil
}
func (s *replaySessionStore) RefreshSessionByRefreshToken(string, string, string) (*store.SessionRecord, error) {
	return s.byRTResult, s.byRTErr
}
func (s *replaySessionStore) FindRotatedOutSession(id string) (*store.SessionRecord, error) {
	return s.rotatedOut, nil
}
func (s *replaySessionStore) DeleteSession(string) error         { return nil }
func (s *replaySessionStore) DeleteSessionByIDHash(string) error { return nil }
func (s *replaySessionStore) ListUserSessions(uuid.UUID) ([]store.SessionRecord, error) {
	return nil, nil
}
func (s *replaySessionStore) DeleteAllUserSessions(userID uuid.UUID) error {
	s.revokedUsers = append(s.revokedUsers, userID)
	return nil
}
func (s *replaySessionStore) DeleteExpired(context.Context) (int, error) { return 0, nil }

// replayUserStore resolves the session owner for the audit log.
type replayUserStore struct {
	store.UserStore
	user *store.UserRecord
}

func (u *replayUserStore) GetByID(uuid.UUID) (*store.UserRecord, error) { return u.user, nil }

func newReplayServer(sessionStore *replaySessionStore, audit *recordingAuditStore) *Server {
	owner := &store.UserRecord{ID: uuid.New(), Email: "owner@example.com", Role: "admin"}
	if sessionStore.rotatedOut != nil {
		owner.ID = sessionStore.rotatedOut.UserID
	} else if sessionStore.byRTResult != nil {
		owner.ID = sessionStore.byRTResult.UserID
	}
	return &Server{
		cfg:          &config.Config{},
		sessionStore: sessionStore,
		userStore:    &replayUserStore{user: owner},
		auditStore:   audit,
		ipExtractor:  newIPExtractor(&config.Config{}),
	}
}

func refreshRequest(cookies map[string]string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	for name, value := range cookies {
		req.AddCookie(&http.Cookie{Name: name, Value: value})
	}
	return req
}

// A replayed (rotated-out) session cookie revokes the whole session family and
// audits the replay instead of just 401ing.
func TestHandleRefreshSession_ReplayedCookieRevokesFamily(t *testing.T) {
	t.Parallel()

	ownerID := uuid.New()
	st := &replaySessionStore{
		rotatedOut: &store.SessionRecord{IDHash: "old-hash", UserID: ownerID},
	}
	audit := &recordingAuditStore{}
	s := newReplayServer(st, audit)

	w := httptest.NewRecorder()
	s.handleRefreshSession(w, refreshRequest(map[string]string{"alga_session": "stolen-old-cookie"}))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if len(st.revokedUsers) != 1 || st.revokedUsers[0] != ownerID {
		t.Fatalf("revoked users = %v, want exactly [%s]", st.revokedUsers, ownerID)
	}
	if len(audit.events) == 0 || audit.events[len(audit.events)-1] != store.AuditLoginFailed {
		t.Fatalf("audit events = %v, want a login_failed replay entry", audit.events)
	}
}

// A replayed refresh token (matches a recorded prev_refresh_token_hashes entry)
// likewise revokes the family via the optional alga_rt presentation path.
func TestHandleRefreshSession_ReplayedRefreshTokenRevokesFamily(t *testing.T) {
	t.Parallel()

	ownerID := uuid.New()
	st := &replaySessionStore{
		byRTResult: &store.SessionRecord{IDHash: "live-hash", UserID: ownerID},
		byRTErr:    store.ErrRefreshTokenReused,
	}
	audit := &recordingAuditStore{}
	s := newReplayServer(st, audit)

	w := httptest.NewRecorder()
	s.handleRefreshSession(w, refreshRequest(map[string]string{"alga_rt": "retired-token"}))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if len(st.revokedUsers) != 1 || st.revokedUsers[0] != ownerID {
		t.Fatalf("revoked users = %v, want exactly [%s]", st.revokedUsers, ownerID)
	}
	if len(audit.events) == 0 || audit.events[len(audit.events)-1] != store.AuditLoginFailed {
		t.Fatalf("audit events = %v, want a login_failed replay entry", audit.events)
	}
}

// A valid session cookie still refreshes normally and (re)issues both cookies.
func TestHandleRefreshSession_ValidCookieUnaffected(t *testing.T) {
	t.Parallel()

	ownerID := uuid.New()
	st := &replaySessionStore{
		refreshResult: &store.SessionRecord{
			ID:           "brand-new-session",
			RefreshToken: "brand-new-rt",
			UserID:       ownerID,
		},
	}
	audit := &recordingAuditStore{}
	s := newReplayServer(st, audit)

	w := httptest.NewRecorder()
	s.handleRefreshSession(w, refreshRequest(map[string]string{"alga_session": "live-cookie"}))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	setCookies := w.Result().Cookies()
	gotSession, gotRT := false, false
	for _, c := range setCookies {
		switch c.Name {
		case "alga_session":
			gotSession = c.Value == "brand-new-session"
		case "alga_rt":
			gotRT = c.Value == "brand-new-rt"
		}
	}
	if !gotSession || !gotRT {
		t.Fatalf("refresh must reissue alga_session and alga_rt cookies (session=%v rt=%v)", gotSession, gotRT)
	}
	if len(st.revokedUsers) != 0 {
		t.Fatalf("normal refresh must not revoke any family, revoked %v", st.revokedUsers)
	}
	if len(audit.events) == 0 || audit.events[len(audit.events)-1] != store.AuditSessionRefreshed {
		t.Fatalf("audit events = %v, want session_refreshed", audit.events)
	}
}

// A refresh token presented without reuse and without a session cookie rotates
// through the refresh-token path.
func TestHandleRefreshSession_LiveRefreshTokenRotates(t *testing.T) {
	t.Parallel()

	ownerID := uuid.New()
	st := &replaySessionStore{
		byRTResult: &store.SessionRecord{
			ID:           "rt-rotated-session",
			RefreshToken: "rt-rotated-token",
			UserID:       ownerID,
		},
	}
	audit := &recordingAuditStore{}
	s := newReplayServer(st, audit)

	w := httptest.NewRecorder()
	s.handleRefreshSession(w, refreshRequest(map[string]string{"alga_rt": "live-token"}))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if len(st.revokedUsers) != 0 {
		t.Fatalf("live-token rotation must not revoke anything, revoked %v", st.revokedUsers)
	}
	if len(audit.events) == 0 || audit.events[len(audit.events)-1] != store.AuditSessionRefreshed {
		t.Fatalf("audit events = %v, want session_refreshed", audit.events)
	}
}

// Unknown credentials get the plain 401 with no revocation side effects.
func TestHandleRefreshSession_UnknownCookieNoRevocation(t *testing.T) {
	t.Parallel()

	st := &replaySessionStore{}
	audit := &recordingAuditStore{}
	s := newReplayServer(st, audit)

	w := httptest.NewRecorder()
	s.handleRefreshSession(w, refreshRequest(map[string]string{"alga_session": "unknown"}))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if len(st.revokedUsers) != 0 {
		t.Fatalf("unknown cookie must not revoke anything, revoked %v", st.revokedUsers)
	}
	if errors.Is(nil, store.ErrRefreshTokenReused) {
		t.Fatal("sanity: errors.Is(nil, sentinel) must be false")
	}
}
