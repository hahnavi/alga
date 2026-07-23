package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"

	"alga/store"
)

type mockStatusPageStore struct {
	mu         sync.Mutex
	pages      map[uuid.UUID]*store.StatusPageRecord
	bySlug     map[string]uuid.UUID
	components map[uuid.UUID]*store.StatusPageComponentRecord
	compByPage map[uuid.UUID][]uuid.UUID
}

func newMockStatusPageStore() *mockStatusPageStore {
	return &mockStatusPageStore{
		pages:      map[uuid.UUID]*store.StatusPageRecord{},
		bySlug:     map[string]uuid.UUID{},
		components: map[uuid.UUID]*store.StatusPageComponentRecord{},
		compByPage: map[uuid.UUID][]uuid.UUID{},
	}
}

func (m *mockStatusPageStore) CreatePage(ctx context.Context, r *store.StatusPageRecord) (*store.StatusPageRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.bySlug[r.Slug]; exists {
		return nil, errSPDup()
	}
	r.ID = uuid.New()
	cp := *r
	m.pages[r.ID] = &cp
	m.bySlug[r.Slug] = r.ID
	return &cp, nil
}
func (m *mockStatusPageStore) UpdatePage(ctx context.Context, id uuid.UUID, patch *store.StatusPageRecord) (*store.StatusPageRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ex, ok := m.pages[id]
	if !ok {
		return nil, errSPNotFound()
	}
	if patch.Name != "" {
		ex.Name = patch.Name
	}
	if patch.EnabledSet {
		ex.Enabled = patch.Enabled
	}
	cp := *ex
	return &cp, nil
}
func (m *mockStatusPageStore) DeletePage(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.pages[id]; !ok {
		return errSPNotFound()
	}
	if slug := m.pages[id].Slug; slug != "" {
		delete(m.bySlug, slug)
	}
	for _, cid := range m.compByPage[id] {
		delete(m.components, cid)
	}
	delete(m.compByPage, id)
	delete(m.pages, id)
	return nil
}
func (m *mockStatusPageStore) GetPage(ctx context.Context, id uuid.UUID) (*store.StatusPageRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.pages[id]
	if !ok {
		return nil, nil
	}
	cp := *r
	return &cp, nil
}
func (m *mockStatusPageStore) GetPageBySlug(ctx context.Context, slug string) (*store.StatusPageRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.bySlug[slug]
	if !ok {
		return nil, nil
	}
	cp := *m.pages[id]
	return &cp, nil
}
func (m *mockStatusPageStore) ListPages(ctx context.Context, q store.StatusPageQuery) ([]store.StatusPageRecord, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]store.StatusPageRecord, 0, len(m.pages))
	for _, p := range m.pages {
		out = append(out, *p)
	}
	return out, int64(len(out)), nil
}
func (m *mockStatusPageStore) ListComponents(ctx context.Context, pageID uuid.UUID) ([]store.StatusPageComponentRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []store.StatusPageComponentRecord{}
	for _, cid := range m.compByPage[pageID] {
		out = append(out, *m.components[cid])
	}
	return out, nil
}
func (m *mockStatusPageStore) CreateComponent(ctx context.Context, r *store.StatusPageComponentRecord) (*store.StatusPageComponentRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r.ID = uuid.New()
	cp := *r
	m.components[r.ID] = &cp
	m.compByPage[r.StatusPageID] = append(m.compByPage[r.StatusPageID], r.ID)
	return &cp, nil
}
func (m *mockStatusPageStore) UpdateComponent(ctx context.Context, id uuid.UUID, patch *store.StatusPageComponentRecord) (*store.StatusPageComponentRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ex, ok := m.components[id]
	if !ok {
		return nil, errSPNotFound()
	}
	if patch.Status != "" {
		ex.Status = patch.Status
	}
	if patch.Name != "" {
		ex.Name = patch.Name
	}
	cp := *ex
	return &cp, nil
}
func (m *mockStatusPageStore) DeleteComponent(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ex, ok := m.components[id]
	if !ok {
		return errSPNotFound()
	}
	delete(m.components, id)
	list := m.compByPage[ex.StatusPageID]
	for i, cid := range list {
		if cid == id {
			m.compByPage[ex.StatusPageID] = append(list[:i], list[i+1:]...)
			break
		}
	}
	return nil
}
func (m *mockStatusPageStore) GetComponent(ctx context.Context, id uuid.UUID) (*store.StatusPageComponentRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.components[id]
	if !ok {
		return nil, nil
	}
	cp := *r
	return &cp, nil
}

type spTestErr struct{ msg string }

func (e spTestErr) Error() string { return e.msg }

func errSPNotFound() error { return spTestErr{"status page not found"} }
func errSPDup() error      { return spTestErr{"unique constraint: duplicate key value"} }

func newStatusPageTestServer(t *testing.T, sp *mockStatusPageStore) (*Server, *http.ServeMux) {
	t.Helper()
	if sp == nil {
		sp = newMockStatusPageStore()
	}
	srv, mux := newTestServer(nil)
	srv.statusPageStore = sp
	return srv, mux
}

func TestStatusPageHandlerCreateAndSlugValidation(t *testing.T) {
	_, mux := newStatusPageTestServer(t, nil)

	// Valid create.
	body := bytes.NewBufferString(`{"name":"Platform","slug":"platform"}`)
	req := authRequest(http.MethodPost, "/api/v1/status-pages", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Invalid slug (uppercase).
	body = bytes.NewBufferString(`{"name":"Bad","slug":"Bad Slug"}`)
	req = authRequest(http.MethodPost, "/api/v1/status-pages", body)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid slug, got %d", rec.Code)
	}
}

func TestStatusPageHandlerViewBySlugReturnsComponentsAndStatus(t *testing.T) {
	sp := newMockStatusPageStore()
	_, mux := newStatusPageTestServer(t, sp)

	page, err := sp.CreatePage(context.Background(), &store.StatusPageRecord{
		Name: "Platform", Slug: "platform", Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	if _, err := sp.CreateComponent(context.Background(), &store.StatusPageComponentRecord{
		StatusPageID: page.ID, Name: "API", Status: store.StatusComponentOperational,
	}); err != nil {
		t.Fatalf("CreateComponent: %v", err)
	}
	if _, err := sp.CreateComponent(context.Background(), &store.StatusPageComponentRecord{
		StatusPageID: page.ID, Name: "DB", Status: store.StatusComponentMajorOutage,
	}); err != nil {
		t.Fatalf("CreateComponent 2: %v", err)
	}

	req := authRequest(http.MethodGet, "/api/v1/status-pages/slug/platform", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var view struct {
		OverallStatus string `json:"overall_status"`
		Components    []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"components"`
		Incidents []any `json:"incidents"`
	}
	if err := decodeResponse(t, rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.OverallStatus != store.StatusComponentMajorOutage {
		t.Fatalf("overall status = %q, want major_outage", view.OverallStatus)
	}
	if len(view.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(view.Components))
	}
	if view.Incidents == nil {
		t.Fatal("expected incidents array (even if empty)")
	}
}

func TestStatusPageHandlerViewUnknownSlugReturns404(t *testing.T) {
	_, mux := newStatusPageTestServer(t, nil)

	req := authRequest(http.MethodGet, "/api/v1/status-pages/slug/nope", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestStatusPageHandlerComponentCreateRequiresWrite(t *testing.T) {
	sp := newMockStatusPageStore()
	_, mux := newStatusPageTestServer(t, sp)

	page, err := sp.CreatePage(context.Background(), &store.StatusPageRecord{
		Name: "Ops", Slug: "ops", Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	body := bytes.NewBufferString(`{"name":"API","status":"operational"}`)
	req := authRequest(http.MethodPost, "/api/v1/status-pages/"+page.ID.String()+"/components", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	comps, _ := sp.ListComponents(context.Background(), page.ID)
	if len(comps) != 1 {
		t.Fatalf("expected 1 component stored, got %d", len(comps))
	}
}
