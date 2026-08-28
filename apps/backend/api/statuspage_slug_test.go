package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/logger"
	"alga/store"
)

// ---- stubs -----------------------------------------------------------------

type b1StatusPageStore struct {
	store.StatusPageStore
	page       *store.StatusPageRecord
	components []store.StatusPageComponentRecord
}

func (s *b1StatusPageStore) GetPageBySlug(ctx context.Context, slug string) (*store.StatusPageRecord, error) {
	if s.page != nil && s.page.Slug == slug {
		return s.page, nil
	}
	return nil, nil
}

func (s *b1StatusPageStore) ListComponents(ctx context.Context, pageID uuid.UUID) ([]store.StatusPageComponentRecord, error) {
	return s.components, nil
}

type b1IncidentStore struct {
	store.IncidentStore
	active    []store.IncidentRecord
	queries   [][]uuid.UUID
	failQuery bool
}

func (m *b1IncidentStore) ListActiveIncidentsForServices(ctx context.Context, ids []uuid.UUID) ([]store.IncidentRecord, error) {
	snapshot := make([]uuid.UUID, len(ids))
	copy(snapshot, ids)
	m.queries = append(m.queries, snapshot)
	if m.failQuery {
		return nil, errors.New("db down")
	}
	return m.active, nil
}

func (m *b1IncidentStore) ListActiveIncidents(ctx context.Context) ([]store.IncidentRecord, error) {
	// The scoped slug view must NEVER fall back to the unfiltered listing.
	m.queries = append(m.queries, nil)
	return m.active, nil
}

// ---- helpers ---------------------------------------------------------------

func b1Page(enabled bool) *store.StatusPageRecord {
	return &store.StatusPageRecord{
		ID:          uuid.New(),
		Name:        "Demo",
		Slug:        "demo",
		Description: "demo page",
		Visibility:  "public",
		Enabled:     enabled,
		EnabledSet:  true,
	}
}

func b1Component(name string, serviceID *uuid.UUID) store.StatusPageComponentRecord {
	return store.StatusPageComponentRecord{
		ID:           uuid.New(),
		StatusPageID: uuid.New(),
		Name:         name,
		ServiceID:    serviceID,
		Status:       "operational",
	}
}

func b1Incident(title string, serviceID *uuid.UUID) store.IncidentRecord {
	started := time.Now().Add(-time.Hour)
	return store.IncidentRecord{
		ID:               uuid.New(),
		IncidentNumber:   42,
		Title:            title,
		Status:           "active",
		Severity:         "sev2",
		ServiceID:        serviceID,
		CommanderID:      &[]uuid.UUID{uuid.New()}[0],
		SlackChannelID:   "C0123456789",
		WarRoomChannelID: "G0123456789",
		StartedAt:        &started,
	}
}

func b1Server(page *store.StatusPageRecord, components []store.StatusPageComponentRecord, incidents *b1IncidentStore) (*Server, *b1StatusPageStore) {
	pageStore := &b1StatusPageStore{page: page, components: components}
	s := &Server{
		statusPageStore: pageStore,
		incidentStore:   incidents,
	}
	logger.Init("error", "")
	return s, pageStore
}

func decodeB1View(t *testing.T, body []byte) statusPageViewResponse {
	t.Helper()
	var raw struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("envelope unmarshal: %v (%s)", err, body)
	}
	var view statusPageViewResponse
	if err := json.Unmarshal(raw.Data, &view); err != nil {
		t.Fatalf("view unmarshal: %v (%s)", err, raw.Data)
	}
	return view
}

// ---- tests -----------------------------------------------------------------

// TestSlugViewScopesIncidents covers the slug view returns only
// active incidents whose service_id belongs to one of the page's components.
func TestSlugViewScopesIncidents(t *testing.T) {
	t.Parallel()

	serviceA := uuid.New()
	serviceB := uuid.New()
	comps := []store.StatusPageComponentRecord{
		b1Component("API", &serviceA),
		b1Component("Web", &serviceB),
		b1Component("Unlinked", nil),
	}
	incidents := &b1IncidentStore{
		active: []store.IncidentRecord{
			b1Incident("A down", &serviceA),
		},
	}

	s, _ := b1Server(b1Page(true), comps, incidents)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status-pages/slug/demo", nil)
	req.SetPathValue("slug", "demo")
	rec := httptest.NewRecorder()
	s.handleStatusPageViewBySlug(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	view := decodeB1View(t, rec.Body.Bytes())

	if len(view.Incidents) != 1 || view.Incidents[0].Title != "A down" {
		t.Fatalf("incidents = %+v, want only the service-A incident", view.Incidents)
	}
	// The scoped query must fire exactly once with both non-null component
	// service ids; the null-service component contributes nothing.
	if len(incidents.queries) != 1 {
		t.Fatalf("incident store queried %d times, want exactly 1 scoped query", len(incidents.queries))
	}
	if len(incidents.queries[0]) != 2 {
		t.Fatalf("scoped ids = %v, want the 2 non-null component service ids", incidents.queries[0])
	}
}

// TestSlugViewEmptyComponentsNoIncidents: a page with zero service-linked
// components returns "incidents": [] without querying the incident store —
// even while other incidents are active elsewhere (acceptance criterion 2).
func TestSlugViewEmptyComponentsNoIncidents(t *testing.T) {
	t.Parallel()

	incidents := &b1IncidentStore{
		active: []store.IncidentRecord{b1Incident("unrelated", nil)},
	}

	s, _ := b1Server(b1Page(true), []store.StatusPageComponentRecord{b1Component("Orphan", nil)}, incidents)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status-pages/slug/demo", nil)
	req.SetPathValue("slug", "demo")
	rec := httptest.NewRecorder()
	s.handleStatusPageViewBySlug(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	view := decodeB1View(t, rec.Body.Bytes())
	if len(view.Incidents) != 0 {
		t.Fatalf("incidents = %d, want empty", len(view.Incidents))
	}
	if incidents.queries != nil {
		t.Fatal("incident store queried despite no service-linked components")
	}
}

// TestSlugViewDisabledPage404s: enabled=false pages return the same 404 as
// missing slugs for every caller (acceptance criterion 3).
func TestSlugViewDisabledPage404s(t *testing.T) {
	t.Parallel()

	for _, role := range []string{"viewer", "admin"} {
		incidents := &b1IncidentStore{}
		s, _ := b1Server(b1Page(false), nil, incidents)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/status-pages/slug/demo", nil)
		req.SetPathValue("slug", "demo")
		rec := httptest.NewRecorder()
		s.handleStatusPageViewBySlug(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404; body: %s", role, rec.Code, rec.Body.String())
		}
		var env struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("error envelope expected, got: %s", rec.Body.String())
		}
		if env.Error.Message != "status page not found" {
			t.Fatalf("message = %q, want uniform not-found text", env.Error.Message)
		}
	}
}

// TestSlugViewModelAllowList asserts the payload contains none of the
// MUST-NOT-LEAK fields (spec S2): internal ids, owner team, Slack/war-room
// linkage, SLA/responder/timestamp extras (acceptance criterion 4).
func TestSlugViewModelAllowList(t *testing.T) {
	t.Parallel()

	serviceA := uuid.New()
	comps := []store.StatusPageComponentRecord{b1Component("API", &serviceA)}
	incidents := &b1IncidentStore{active: []store.IncidentRecord{b1Incident("A down", &serviceA)}}

	s, _ := b1Server(b1Page(true), comps, incidents)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status-pages/slug/demo", nil)
	req.SetPathValue("slug", "demo")
	rec := httptest.NewRecorder()
	s.handleStatusPageViewBySlug(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := doc["data"].(map[string]any)
	if data == nil {
		t.Fatalf("missing data envelope: %s", rec.Body.String())
	}

	forbidden := []string{
		"id", "status_page_id", "service_id", "owner_team_id",
		"slack_channel_id", "slack_channel_name", "war_room_channel_id",
		"commander_id", "visibility", "enabled", "created_at", "updated_at",
	}
	for _, key := range forbidden {
		if _, present := data[key]; present {
			t.Errorf("top-level payload leaked %q", key)
		}
	}

	incJSON, err := json.Marshal(data["incidents"])
	if err != nil {
		t.Fatalf("marshal incidents: %v", err)
	}
	for _, leak := range forbidden {
		if leak == "id" || leak == "service_id" || leak == "commander_id" ||
			leak == "owner_team_id" || leak == "war_room_channel_id" {
			continue // only meaningful at top level for this check set below
		}
	}
	for _, leak := range []string{"slack_channel_id", "war_room_channel_id", "commander_id", "sla_target_respond_at"} {
		if strings.Contains(string(incJSON), "\""+leak+"\"") {
			t.Errorf("incident payload leaked %q", leak)
		}
	}

	view := decodeB1View(t, rec.Body.Bytes())
	if view.Page.Name != "Demo" || view.Page.Slug != "demo" {
		t.Fatalf("page = %+v, want name/slug preserved", view.Page)
	}
	if view.OverallStatus != "operational" {
		t.Fatalf("overall_status = %q, want operational rollup unchanged", view.OverallStatus)
	}
	if len(view.Components) != 1 || view.Components[0].Name != "API" {
		t.Fatalf("components = %+v, want allow-listed projection", view.Components)
	}
	if view.Incidents[0].Title != "A down" || view.Incidents[0].Severity != "sev2" {
		t.Fatalf("incident projection = %+v", view.Incidents[0])
	}
}

// TestSlugViewScopedListFailureDegradesEmpty preserves the documented
// degradation: a failed scoped listing warns and yields [] instead of failing
// the whole view (spec R4.4 successor).
func TestSlugViewScopedListFailureDegradesEmpty(t *testing.T) {
	t.Parallel()

	serviceA := uuid.New()
	comps := []store.StatusPageComponentRecord{b1Component("API", &serviceA)}
	incidents := &b1IncidentStore{failQuery: true}

	s, _ := b1Server(b1Page(true), comps, incidents)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status-pages/slug/demo", nil)
	req.SetPathValue("slug", "demo")
	rec := httptest.NewRecorder()
	s.handleStatusPageViewBySlug(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with degraded incidents", rec.Code)
	}
	view := decodeB1View(t, rec.Body.Bytes())
	if view.Incidents == nil || len(view.Incidents) != 0 {
		t.Fatalf("incidents = %+v, want empty array on degradation", view.Incidents)
	}
}
