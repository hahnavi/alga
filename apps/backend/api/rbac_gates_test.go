package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/api/platform"
	"alga/config"
	"alga/rbac"
	"alga/store"
)

// gateMiddleware returns a ready middleware for the given deps with no
// permission gate; gated variants pass perms at registration time in prod.
func gateMiddleware(deps platform.AuthDeps, next http.HandlerFunc, perms ...rbac.Permission) http.HandlerFunc {
	return platform.AuthMiddleware(deps, next, perms...)
}

// stubPATStore returns a fixed PAT record for ValidateToken and satisfies the
// full PersonalAccessTokenStore interface for middleware tests.
type stubPATStore struct {
	record *store.PATRecord
}

func (s *stubPATStore) CreateToken(_ uuid.UUID, _ string, _ []string, _ *time.Time) (*store.PATRecord, error) {
	return nil, nil
}

func (s *stubPATStore) ListByUser(uuid.UUID) ([]store.PATRecord, error) { return nil, nil }
func (s *stubPATStore) ListAll() ([]store.PATRecord, error)             { return nil, nil }
func (s *stubPATStore) RevokeToken(uuid.UUID, uuid.UUID) error          { return nil }
func (s *stubPATStore) RevokeTokenAdmin(uuid.UUID) error                { return nil }
func (s *stubPATStore) Close()                                          {}

func (s *stubPATStore) ValidateToken(string) (*store.PATRecord, error) {
	return s.record, nil
}

// stubSessionStoreForGate hands back one canned session regardless of cookie.
type stubSessionStoreForGate struct {
	session *store.SessionRecord
	user    *store.UserRecord
}

func (s *stubSessionStoreForGate) CreateSession(uuid.UUID, string, string) (*store.SessionRecord, error) {
	return nil, nil
}
func (s *stubSessionStoreForGate) GetSession(string) (*store.SessionRecord, error) {
	return s.session, nil
}

func (s *stubSessionStoreForGate) GetSessionByRefreshToken(string) (*store.SessionRecord, error) {
	return nil, nil
}
func (s *stubSessionStoreForGate) RefreshSession(string, string, string) (*store.SessionRecord, error) {
	return nil, nil
}
func (s *stubSessionStoreForGate) RefreshSessionByRefreshToken(string, string, string) (*store.SessionRecord, error) {
	return nil, nil
}
func (s *stubSessionStoreForGate) FindRotatedOutSession(string) (*store.SessionRecord, error) {
	return nil, nil
}
func (s *stubSessionStoreForGate) DeleteSession(string) error         { return nil }
func (s *stubSessionStoreForGate) DeleteSessionByIDHash(string) error { return nil }
func (s *stubSessionStoreForGate) ListUserSessions(uuid.UUID) ([]store.SessionRecord, error) {
	return nil, nil
}
func (s *stubSessionStoreForGate) DeleteAllUserSessions(uuid.UUID) error      { return nil }
func (s *stubSessionStoreForGate) DeleteExpired(context.Context) (int, error) { return 0, nil }
func (s *stubSessionStoreForGate) Close()                                     {}

// fixedIPExtractor returns a constant client IP.
type fixedIPExtractor struct{}

func (fixedIPExtractor) ClientIP(*http.Request) string { return "10.9.9.9" }

func gateTestUser(role string) *store.UserRecord {
	return &store.UserRecord{ID: uuid.New(), Email: "gate@example.com", Role: role}
}

// gateUserStore returns a fixed user from GetByID (used by both the PAT and
// session middleware paths).
type gateUserStore struct {
	store.UserStore
	user *store.UserRecord
}

func (s *gateUserStore) GetByID(uuid.UUID) (*store.UserRecord, error) { return s.user, nil }

// gateTestDeps builds auth middleware deps. For PAT requests, sessionUser is
// the PAT owner loaded by the middleware; for cookie requests it is the user
// behind the canned session.
func gateTestDeps(pat *store.PATRecord, sessionUser *store.UserRecord) platform.AuthDeps {
	var uid uuid.UUID
	if sessionUser != nil {
		uid = sessionUser.ID
	}
	return platform.AuthDeps{
		UserStore: &gateUserStore{user: sessionUser},
		SessionStore: &stubSessionStoreForGate{
			session: &store.SessionRecord{IDHash: "h", UserID: uid},
			user:    sessionUser,
		},
		PersonalAccessTokenStore: &stubPATStore{record: pat},
		AuditStore:               &recordingAuditStore{},
		IPExtractor:              fixedIPExtractor{},
	}
}

// gateSessionRequest builds an authenticated cookie request with a matching
// CSRF pair (required for state-changing methods).
func gateSessionRequest(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.AddCookie(&http.Cookie{Name: "alga_session", Value: "test-session"})
	req.AddCookie(&http.Cookie{Name: "alga_csrf", Value: "csrf-token"})
	req.Header.Set("X-CSRF-Token", "csrf-token")
	return req
}

// gatePATRequest builds a PAT-authenticated request.
func gatePATRequest(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Authorization", "Bearer alga_pat_test")
	return req
}

func TestGatedRoutesRequirePermission(t *testing.T) {
	t.Parallel()

	viewer := gateTestUser("viewer")
	deps := gateTestDeps(nil, viewer)

	// One representative route per gated group. Each must reject a viewer-less
	// role (here: an operator-scoped PAT intersection is simulated separately)
	// and accept the roles granted by rbac.Roles.
	routes := map[string]rbac.Permission{
		"/api/v1/routes":                  rbac.RoutesRead,
		"/api/v1/integrations":            rbac.IntegrationsRead,
		"/api/v1/channels":                rbac.ChannelsRead,
		"/api/v1/destinations":            rbac.ChannelsRead,
		"/api/v1/dashboard/stats":         rbac.DashboardRead,
		"/api/v1/dashboard/daily-summary": rbac.DashboardRead,
		"/api/v1/alerts":                  rbac.AlertsRead,
		"/api/v1/knowledge":               rbac.KnowledgeRead,
		"/api/v1/memories":                rbac.MemoriesRead,
		"/api/v1/maintenance-windows":     rbac.RoutesRead,
	}

	for path, perm := range routes {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			mw := func(w http.ResponseWriter, r *http.Request) {
				gateMiddleware(deps, func(w http.ResponseWriter, _ *http.Request) {
					writeStatus(w, "ok")
				}, perm)(w, r)
			}
			req := gateSessionRequest(http.MethodGet, path)
			w := httptest.NewRecorder()
			mw(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("viewer GET %s = %d, want 200 (body=%s)", path, w.Code, w.Body.String())
			}
		})
	}
}

func TestDailySummaryPostRequiresSystemConfigWrite(t *testing.T) {
	t.Parallel()

	// Viewer holds DashboardRead but NOT SystemConfigWrite: GET passes, POST
	// must be rejected in-handler before any regeneration work happens.
	viewer := gateTestUser("viewer")
	deps := gateTestDeps(nil, viewer)
	s := &Server{
		cfg:         &config.Config{},
		ipExtractor: newIPExtractor(&config.Config{}),
		auditStore:  &recordingAuditStore{},
	}

	mw := func(w http.ResponseWriter, r *http.Request) {
		gateMiddleware(deps, s.handleDailySummary, rbac.DashboardRead)(w, r)
	}

	get := gateSessionRequest(http.MethodGet, "/api/v1/dashboard/daily-summary")
	w := httptest.NewRecorder()
	mw(w, get)
	if w.Code != http.StatusOK {
		t.Fatalf("viewer GET daily-summary = %d, want 200", w.Code)
	}

	post := gateSessionRequest(http.MethodPost, "/api/v1/dashboard/daily-summary")
	wp := httptest.NewRecorder()
	mw(wp, post)
	if wp.Code != http.StatusForbidden {
		t.Fatalf("viewer POST daily-summary = %d, want 403 (body=%s)", wp.Code, wp.Body.String())
	}

	// Admin holds SystemConfigWrite and passes the in-handler check.
	admin := gateTestUser("admin")
	depsAdmin := gateTestDeps(nil, admin)
	sa := &Server{
		cfg:         &config.Config{},
		ipExtractor: newIPExtractor(&config.Config{}),
		auditStore:  &recordingAuditStore{},
	}
	mwAdmin := func(w http.ResponseWriter, r *http.Request) {
		gateMiddleware(depsAdmin, sa.handleDailySummary, rbac.DashboardRead)(w, r)
	}
	postAdmin := gateSessionRequest(http.MethodPost, "/api/v1/dashboard/daily-summary")
	wa := httptest.NewRecorder()
	mwAdmin(wa, postAdmin)
	if wa.Code == http.StatusForbidden {
		t.Fatalf("admin POST daily-summary unexpectedly 403")
	}
}

func TestPATIntersectionEnforcedAtGate(t *testing.T) {
	t.Parallel()

	// A PAT scoped to exactly alerts:read on an admin user must reach alerts
	// but be denied on integrations now that the gate carries a permission.
	user := gateTestUser("admin")
	pat := &store.PATRecord{ID: uuid.New(), UserID: user.ID, Permissions: []string{"alerts:read"}}
	deps := gateTestDeps(pat, user)

	req := gatePATRequest(http.MethodGet, "/api/v1/integrations")
	w := httptest.NewRecorder()
	gateMiddleware(deps, func(w http.ResponseWriter, _ *http.Request) {
		writeStatus(w, "ok")
	}, rbac.IntegrationsRead)(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("alerts-only PAT on /integrations = %d, want 403", w.Code)
	}

	reqOK := gatePATRequest(http.MethodGet, "/api/v1/alerts")
	wOK := httptest.NewRecorder()
	gateMiddleware(deps, func(w http.ResponseWriter, _ *http.Request) {
		writeStatus(w, "ok")
	}, rbac.AlertsRead)(wOK, reqOK)
	if wOK.Code != http.StatusOK {
		t.Fatalf("alerts-only PAT on /alerts = %d, want 200", wOK.Code)
	}
}

func TestSystemConfigMatchesFrontendGates(t *testing.T) {
	t.Parallel()

	// /api/v1/system/config is consumed only by admin-only settings pages
	// (router.ts gates them with requiredPermission "system:read", a role
	// permission held by admin alone), so the new gate must admit admins and
	// deny viewers - matching the documented RBAC contract.
	adminDeps := gateTestDeps(nil, gateTestUser("admin"))
	req := gateSessionRequest(http.MethodGet, "/api/v1/system/config")
	w := httptest.NewRecorder()
	gateMiddleware(adminDeps, func(w http.ResponseWriter, _ *http.Request) {
		writeStatus(w, "ok")
	}, rbac.SystemConfigRead)(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin GET system/config = %d, want 200", w.Code)
	}

	viewerDeps := gateTestDeps(nil, gateTestUser("viewer"))
	reqV := gateSessionRequest(http.MethodGet, "/api/v1/system/config")
	wv := httptest.NewRecorder()
	gateMiddleware(viewerDeps, func(w http.ResponseWriter, _ *http.Request) {
		writeStatus(w, "ok")
	}, rbac.SystemConfigRead)(wv, reqV)
	if wv.Code != http.StatusForbidden {
		t.Fatalf("viewer GET system/config = %d, want 403", wv.Code)
	}
}

func TestAnonymousStillUnauthorized(t *testing.T) {
	t.Parallel()

	deps := gateTestDeps(nil, gateTestUser("viewer"))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/routes", nil)
	w := httptest.NewRecorder()
	gateMiddleware(deps, func(http.ResponseWriter, *http.Request) {})(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous request = %d, want 401", w.Code)
	}
}
