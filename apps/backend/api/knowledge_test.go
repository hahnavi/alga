package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"alga/config"
	"alga/routing"
	"alga/store"

	"github.com/google/uuid"
)

// mockKnowledgeStore is a minimal store.KnowledgeStore for handler tests.
// It only populates Get meaningfully; the other methods return zero values.
type mockKnowledgeStore struct {
	notes map[string]*store.KnowledgeNote
	getID string
}

func (m *mockKnowledgeStore) Create(_ context.Context, note *store.KnowledgeNote) (*store.KnowledgeNote, error) {
	return note, nil
}
func (m *mockKnowledgeStore) Update(_ context.Context, _ string, patch *store.KnowledgeNote) (*store.KnowledgeNote, error) {
	return patch, nil
}
func (m *mockKnowledgeStore) Delete(_ context.Context, _ string) error { return nil }
func (m *mockKnowledgeStore) Get(_ context.Context, id string) (*store.KnowledgeNote, error) {
	m.getID = id
	if m.notes == nil {
		return nil, nil
	}
	return m.notes[id], nil
}
func (m *mockKnowledgeStore) List(_ context.Context, _ store.KnowledgeQuery) ([]store.KnowledgeNote, int64, error) {
	return nil, 0, nil
}
func (m *mockKnowledgeStore) Match(_ context.Context, _ map[string]string, _ int) ([]store.KnowledgeNote, error) {
	return nil, nil
}

func newAgentKnowledgeTestServer(t *testing.T, kb store.KnowledgeStore) (*Server, *http.ServeMux) {
	t.Helper()
	agentTok := &testAgentTokenStore{
		validToken: "secret-agent-token",
		agentID:    uuid.New(),
		name:       "test-agent",
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
		nil,
		nil,
		nil,
		nil,
		func(*routing.Engine) {},
		NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute),
		NewRateLimiter(10, 20),
		&mockAlertInvestigationStore{},
		&mockIncidentInvestigationStore{},
		nil,
		nil,
		nil,
		nil,
	)
	srv.SetKnowledgeStore(kb)
	mux := http.NewServeMux()
	srv.Register(mux)
	return srv, mux
}

func TestAgentKnowledgeByIDReturnsFullNote(t *testing.T) {
	noteID := uuid.MustParse("00000000-0000-0000-0000-0000000000aa")
	fullBody := "## PostgreSQLDown Recovery\n\nFull runbook body that is much longer than the 200-char search preview."
	kb := &mockKnowledgeStore{
		notes: map[string]*store.KnowledgeNote{
			noteID.String(): {
				ID:           noteID,
				Kind:         store.KnowledgeKindRunbook,
				Title:        "PostgreSQLDown on Patroni-managed nodes",
				BodyMarkdown: fullBody,
			},
		},
	}
	_, mux := newAgentKnowledgeTestServer(t, kb)

	req := agentAuthRequest(http.MethodGet, "/api/v1/agent/knowledge/"+noteID.String(), nil, "secret-agent-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got store.KnowledgeNote
	if err := decodeResponse(t, rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode note: %v", err)
	}
	if got.BodyMarkdown != fullBody {
		t.Fatalf("expected full body returned unchanged, got %q", got.BodyMarkdown)
	}
	if got.Title != "PostgreSQLDown on Patroni-managed nodes" {
		t.Fatalf("unexpected title %q", got.Title)
	}
}

func TestAgentKnowledgeByIDNotFound(t *testing.T) {
	missingID := uuid.MustParse("00000000-0000-0000-0000-0000000000bb")
	kb := &mockKnowledgeStore{notes: map[string]*store.KnowledgeNote{}}
	_, mux := newAgentKnowledgeTestServer(t, kb)

	req := agentAuthRequest(http.MethodGet, "/api/v1/agent/knowledge/"+missingID.String(), nil, "secret-agent-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing note, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentKnowledgeByIDRequiresBearer(t *testing.T) {
	noteID := uuid.MustParse("00000000-0000-0000-0000-0000000000cc")
	kb := &mockKnowledgeStore{notes: map[string]*store.KnowledgeNote{
		noteID.String(): {ID: noteID, Title: "x", BodyMarkdown: "y"},
	}}
	_, mux := newAgentKnowledgeTestServer(t, kb)

	req := agentAuthRequest(http.MethodGet, "/api/v1/agent/knowledge/"+noteID.String(), nil, "")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without bearer, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentKnowledgeByIDInvalidID(t *testing.T) {
	kb := &mockKnowledgeStore{notes: map[string]*store.KnowledgeNote{}}
	_, mux := newAgentKnowledgeTestServer(t, kb)

	req := agentAuthRequest(http.MethodGet, "/api/v1/agent/knowledge/not-a-uuid", nil, "secret-agent-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound && rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 404 or 400 for invalid id, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentKnowledgeByIDRejectsNonGet(t *testing.T) {
	noteID := uuid.MustParse("00000000-0000-0000-0000-0000000000dd")
	kb := &mockKnowledgeStore{notes: map[string]*store.KnowledgeNote{
		noteID.String(): {ID: noteID, Title: "x", BodyMarkdown: "y"},
	}}
	_, mux := newAgentKnowledgeTestServer(t, kb)

	req := agentAuthRequest(http.MethodDelete, "/api/v1/agent/knowledge/"+noteID.String(), bytes.NewBufferString(`{}`), "secret-agent-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for DELETE on agent knowledge by-id, got %d: %s", rec.Code, rec.Body.String())
	}
}
