package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/store"
)

type mockHeartbeatStore struct {
	mu        sync.Mutex
	records   map[uuid.UUID]*store.HeartbeatRecord
	tokenIdx  map[string]uuid.UUID
	createErr error
}

func newMockHeartbeatStore() *mockHeartbeatStore {
	return &mockHeartbeatStore{
		records:  map[uuid.UUID]*store.HeartbeatRecord{},
		tokenIdx: map[string]uuid.UUID{},
	}
}

func (m *mockHeartbeatStore) Create(ctx context.Context, record *store.HeartbeatRecord) (*store.HeartbeatRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.createErr != nil {
		return nil, m.createErr
	}
	id := uuid.New()
	record.ID = id
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now
	if record.PingToken == "" {
		record.PingToken = "alga_hb_test_" + id.String()
	}
	cp := *record
	m.records[id] = &cp
	m.tokenIdx[record.PingToken] = id
	return record, nil
}

func (m *mockHeartbeatStore) Update(ctx context.Context, id uuid.UUID, patch *store.HeartbeatRecord) (*store.HeartbeatRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.records[id]
	if !ok {
		return nil, errHeartbeatNotFoundMock
	}
	if patch.Name != "" {
		existing.Name = patch.Name
	}
	if patch.IntervalSeconds > 0 {
		existing.IntervalSeconds = patch.IntervalSeconds
	}
	if patch.EnabledSet {
		existing.Enabled = patch.Enabled
	}
	existing.UpdatedAt = time.Now().UTC()
	cp := *existing
	return &cp, nil
}

func (m *mockHeartbeatStore) Delete(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.records[id]; !ok {
		return errHeartbeatNotFoundMock
	}
	delete(m.records, id)
	return nil
}

func (m *mockHeartbeatStore) Get(ctx context.Context, id uuid.UUID) (*store.HeartbeatRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return nil, errHeartbeatNotFoundMock
	}
	cp := *r
	return &cp, nil
}

func (m *mockHeartbeatStore) List(ctx context.Context, q store.HeartbeatQuery) ([]store.HeartbeatRecord, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]store.HeartbeatRecord, 0, len(m.records))
	for _, r := range m.records {
		if q.Enabled != nil && r.Enabled != *q.Enabled {
			continue
		}
		cp := *r
		out = append(out, cp)
	}
	return out, int64(len(out)), nil
}

func (m *mockHeartbeatStore) RegenerateToken(ctx context.Context, id uuid.UUID) (*store.HeartbeatRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return nil, errHeartbeatNotFoundMock
	}
	r.PingToken = "alga_hb_test_regen_" + uuid.NewString()
	m.tokenIdx[r.PingToken] = id
	cp := *r
	return &cp, nil
}

func (m *mockHeartbeatStore) GetByPingToken(ctx context.Context, token string) (*store.HeartbeatRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.tokenIdx[token]
	if !ok {
		return nil, nil
	}
	r := m.records[id]
	cp := *r
	return &cp, nil
}

func (m *mockHeartbeatStore) RecordPing(ctx context.Context, id uuid.UUID, now time.Time) (*store.HeartbeatRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return nil, errHeartbeatNotFoundMock
	}
	r.LastPingAt = &now
	r.Status = store.HeartbeatStatusHealthy
	cp := *r
	return &cp, nil
}

func (m *mockHeartbeatStore) ListExpired(ctx context.Context, now time.Time) ([]store.HeartbeatRecord, error) {
	return nil, nil
}

func (m *mockHeartbeatStore) MarkExpired(ctx context.Context, id uuid.UUID, now time.Time) (*store.HeartbeatRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return nil, errHeartbeatNotFoundMock
	}
	r.Status = store.HeartbeatStatusExpired
	cp := *r
	return &cp, nil
}

var errHeartbeatNotFoundMock = errHeartbeatTestNotFound{}

type errHeartbeatTestNotFound struct{}

func (errHeartbeatTestNotFound) Error() string { return "heartbeat not found" }

func newHeartbeatTestServer(t *testing.T, hb *mockHeartbeatStore) (*Server, *http.ServeMux) {
	t.Helper()
	if hb == nil {
		hb = newMockHeartbeatStore()
	}
	alerts := &mockStore{}
	srv, mux := newTestServer(alerts)
	srv.heartbeatStore = hb
	return srv, mux
}

func TestHeartbeatHandlerCreateReturnsToken(t *testing.T) {
	hb := newMockHeartbeatStore()
	_, mux := newHeartbeatTestServer(t, hb)

	body := bytes.NewBufferString(`{"name":"api","interval_seconds":60,"severity":"critical"}`)
	req := authRequest(http.MethodPost, "/api/v1/heartbeats", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var got store.HeartbeatRecord
	if err := decodeResponse(t, rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PingToken == "" {
		t.Fatal("expected ping_token to be returned once")
	}
	if got.Name != "api" {
		t.Fatalf("name = %q", got.Name)
	}
}

func TestHeartbeatHandlerPublicPingRequiresNoAuth(t *testing.T) {
	hb := newMockHeartbeatStore()
	srv, mux := newHeartbeatTestServer(t, hb)

	// Seed a heartbeat via the store directly (bypassing auth).
	created, err := hb.Create(context.Background(), &store.HeartbeatRecord{
		Name: "worker", IntervalSeconds: 60, Enabled: true,
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Ping uses an UNAUTHENTICATED request (no session/CSRF cookies) — it is a
	// public, token-gated endpoint.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeats/ping/"+created.PingToken, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	got, _ := hb.Get(context.Background(), created.ID)
	if got == nil || got.Status != store.HeartbeatStatusHealthy {
		t.Fatalf("expected healthy after ping, got %+v", got)
	}
	if srv.heartbeatStore == nil {
		t.Fatal("store wiring broken")
	}
}

func TestHeartbeatHandlerPingBogusTokenReturns404(t *testing.T) {
	_, mux := newHeartbeatTestServer(t, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeats/ping/alga_hb_does_not_exist", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown token, got %d", rec.Code)
	}
}

// trackingAlertStore (defined in http_test.go) is reused here to observe that
// the recovery path invokes ResolveAlertByUser.

func TestHeartbeatHandlerPingResolvesOpenAlert(t *testing.T) {
	hb := newMockHeartbeatStore()
	tracker := &trackingAlertStore{mockStore: mockStore{byFP: map[string]store.AlertRecord{}}}
	srv, mux := newTestServer(nil)
	srv.alertStore = tracker
	srv.heartbeatStore = hb

	created, err := hb.Create(context.Background(), &store.HeartbeatRecord{
		Name: "expiring", IntervalSeconds: 60, Enabled: true,
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Seed an open (firing) alert for the heartbeat fingerprint, simulating a
	// prior expired-heartbeat alert that should be resolved on recovery.
	fp := "heartbeat:" + created.ID.String()
	tracker.byFP[fp] = store.AlertRecord{Fingerprint: fp, Status: "firing"}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeats/ping/"+created.PingToken, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if len(tracker.resolved) != 1 || tracker.resolved[0] != fp {
		t.Fatalf("expected recovery to resolve fingerprint %q, got %+v", fp, tracker.resolved)
	}
}

func TestHeartbeatHandlerRegenerateToken(t *testing.T) {
	hb := newMockHeartbeatStore()
	_, mux := newHeartbeatTestServer(t, hb)

	created, err := hb.Create(context.Background(), &store.HeartbeatRecord{
		Name: "rot", IntervalSeconds: 60, Enabled: true,
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	old := created.PingToken

	req := authRequest(http.MethodPost, "/api/v1/heartbeats/"+created.ID.String()+"/regenerate-token", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got store.HeartbeatRecord
	if err := decodeResponse(t, rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PingToken == "" || got.PingToken == old {
		t.Fatal("expected a new distinct token")
	}
}

func TestHeartbeatHandlerCreateRejectsInvalidInput(t *testing.T) {
	_, mux := newHeartbeatTestServer(t, nil)

	// Missing name and interval.
	body := bytes.NewBufferString(`{}`)
	req := authRequest(http.MethodPost, "/api/v1/heartbeats", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty input, got %d", rec.Code)
	}

	// Invalid severity.
	body = bytes.NewBufferString(`{"name":"x","interval_seconds":60,"severity":"p9"}`)
	req = authRequest(http.MethodPost, "/api/v1/heartbeats", body)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid severity, got %d", rec.Code)
	}
}

func TestHeartbeatHandlerListRequiresAuth(t *testing.T) {
	_, mux := newHeartbeatTestServer(t, nil)

	// No auth cookies: must be rejected.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/heartbeats", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", rec.Code)
	}

	// Authenticated: allowed.
	req = authRequest(http.MethodGet, "/api/v1/heartbeats", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with auth, got %d", rec.Code)
	}
}
