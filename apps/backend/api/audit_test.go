package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"alga/config"
	"alga/rbac"
	"alga/store"
)

// auditQueryFake serves canned Query results; the embedded nil interface keeps
// the rest of the AuditStore surface out of the test's concern.
type auditQueryFake struct {
	store.AuditStore
	records   []store.AuditRecord
	total     int64
	gotFilter map[string]any
}

func (f *auditQueryFake) Query(filter map[string]any) ([]store.AuditRecord, int64, error) {
	f.gotFilter = filter
	return f.records, f.total, nil
}

func newAuditEventsTestServer(fake *auditQueryFake) *Server {
	return &Server{
		cfg:         &config.Config{},
		ipExtractor: newIPExtractor(&config.Config{}),
		auditStore:  fake,
	}
}

// TestC9AuditEventsForbiddenWithoutAuditRead proves the endpoint rejects roles
// lacking audit:read (viewer) at the permission gate.
func TestC9AuditEventsForbiddenWithoutAuditRead(t *testing.T) {
	t.Parallel()

	viewer := gateTestUser("viewer")
	deps := gateTestDeps(nil, viewer)
	s := newAuditEventsTestServer(&auditQueryFake{})

	req := gateSessionRequest(http.MethodGet, "/api/v1/audit-events")
	w := httptest.NewRecorder()
	gateMiddleware(deps, s.handleListAuditEvents, rbac.AuditRead)(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer GET audit-events = %d, want 403 (body=%s)", w.Code, w.Body.String())
	}
}

// TestC9AuditEventsListSuccess covers the success path for a role holding
// audit:read (operator), including pagination plumbing and filter passthrough.
func TestC9AuditEventsListSuccess(t *testing.T) {
	t.Parallel()

	operator := gateTestUser("operator")
	deps := gateTestDeps(nil, operator)
	fake := &auditQueryFake{
		records: []store.AuditRecord{
			{ID: uuid.New(), Event: store.AuditLoginSuccess, Username: "a@b.c", Success: true},
			{ID: uuid.New(), Event: store.AuditUserSlackLinked, Username: "a@b.c", Success: true},
		},
		total: 2,
	}
	s := newAuditEventsTestServer(fake)

	req := gateSessionRequest(http.MethodGet, "/api/v1/audit-events?limit=2&skip=4&event=login_success")
	w := httptest.NewRecorder()
	gateMiddleware(deps, s.handleListAuditEvents, rbac.AuditRead)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("operator GET audit-events = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	var body struct {
		Data struct {
			Items []store.AuditRecord `json:"items"`
			Total int64               `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Data.Items) != 2 || body.Data.Total != 2 {
		t.Fatalf("items=%d total=%d, want 2/2", len(body.Data.Items), body.Data.Total)
	}
	if fake.gotFilter["$limit"] != int64(2) || fake.gotFilter["$skip"] != int64(4) {
		t.Fatalf("filter limit/skip = %v/%v, want 2/4", fake.gotFilter["$limit"], fake.gotFilter["$skip"])
	}
	if fake.gotFilter["event"] != "login_success" {
		t.Fatalf("filter event = %v, want login_success", fake.gotFilter["event"])
	}
}

// TestC9AuditEventsValidationAndGuards covers the invalid entity_id rejection
// and the missing-store 503.
func TestC9AuditEventsValidationAndGuards(t *testing.T) {
	t.Parallel()

	s := newAuditEventsTestServer(&auditQueryFake{})

	badReq := gateSessionRequest(http.MethodGet, "/api/v1/audit-events?entity_id=not-a-uuid")
	w := httptest.NewRecorder()
	s.handleListAuditEvents(w, badReq)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid entity_id = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}

	noStore := &Server{cfg: &config.Config{}, ipExtractor: newIPExtractor(&config.Config{})}
	w2 := httptest.NewRecorder()
	noStore.handleListAuditEvents(w2, gateSessionRequest(http.MethodGet, "/api/v1/audit-events"))
	if w2.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing audit store = %d, want 503", w2.Code)
	}
}
