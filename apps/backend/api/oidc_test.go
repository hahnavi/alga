package api

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/store"
)

type mockOIDCProviderStore struct {
	mu        sync.Mutex
	providers map[uuid.UUID]*store.OIDCProviderRecord
	secrets   map[uuid.UUID]string
}

func newMockOIDCProviderStore() *mockOIDCProviderStore {
	return &mockOIDCProviderStore{
		providers: map[uuid.UUID]*store.OIDCProviderRecord{},
		secrets:   map[uuid.UUID]string{},
	}
}

func (m *mockOIDCProviderStore) CreateProvider(_ context.Context, record *store.OIDCProviderRecord, clientSecret string) (*store.OIDCProviderRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.New()
	now := time.Now().UTC()
	out := *record
	out.ID = id
	out.CreatedAt = now
	out.UpdatedAt = now
	out.ClientSecretConfigured = clientSecret != ""
	m.providers[id] = &out
	m.secrets[id] = clientSecret
	return &out, nil
}

func (m *mockOIDCProviderStore) UpdateProvider(_ context.Context, id uuid.UUID, patch *store.OIDCProviderRecord, clientSecret *string) (*store.OIDCProviderRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.providers[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	if patch.Name != "" {
		existing.Name = patch.Name
	}
	if patch.Issuer != "" {
		existing.Issuer = patch.Issuer
	}
	if patch.ClientID != "" {
		existing.ClientID = patch.ClientID
	}
	if patch.Scopes != nil {
		existing.Scopes = patch.Scopes
	}
	if patch.EnabledSet {
		existing.Enabled = patch.Enabled
	}
	if clientSecret != nil {
		m.secrets[id] = *clientSecret
		existing.ClientSecretConfigured = *clientSecret != ""
	}
	existing.UpdatedAt = time.Now().UTC()
	cp := *existing
	return &cp, nil
}

func (m *mockOIDCProviderStore) DeleteProvider(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.providers[id]; !ok {
		return fmt.Errorf("oidc provider not found: %w", store.ErrNotFound)
	}
	delete(m.providers, id)
	delete(m.secrets, id)
	return nil
}

func (m *mockOIDCProviderStore) GetProvider(_ context.Context, id uuid.UUID) (*store.OIDCProviderRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.providers[id]
	if !ok {
		return nil, fmt.Errorf("oidc provider not found: %w", store.ErrNotFound)
	}
	cp := *existing
	return &cp, nil
}

func (m *mockOIDCProviderStore) GetProviderWithSecret(_ context.Context, id uuid.UUID) (*store.OIDCProviderRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.providers[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	cp := *existing
	cp.SetClientSecret(m.secrets[id])
	return &cp, nil
}

func (m *mockOIDCProviderStore) ListProviders(_ context.Context, q store.OIDCProviderQuery) ([]store.OIDCProviderRecord, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var items []store.OIDCProviderRecord
	for _, p := range m.providers {
		if q.Enabled != nil && p.Enabled != *q.Enabled {
			continue
		}
		items = append(items, *p)
	}
	return items, int64(len(items)), nil
}

func (m *mockOIDCProviderStore) ListEnabledProviders(_ context.Context) ([]store.OIDCProviderRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var items []store.OIDCProviderRecord
	for _, p := range m.providers {
		if p.Enabled {
			items = append(items, *p)
		}
	}
	return items, nil
}

type mockOIDCIdentityStore struct {
	mu         sync.Mutex
	identities map[uuid.UUID]*store.OIDCIdentityRecord
}

func newMockOIDCIdentityStore() *mockOIDCIdentityStore {
	return &mockOIDCIdentityStore{identities: map[uuid.UUID]*store.OIDCIdentityRecord{}}
}

func (m *mockOIDCIdentityStore) CreateLink(_ context.Context, record *store.OIDCIdentityRecord) (*store.OIDCIdentityRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.New()
	now := time.Now().UTC()
	out := *record
	out.ID = id
	out.CreatedAt = now
	out.UpdatedAt = now
	m.identities[id] = &out
	return &out, nil
}

func (m *mockOIDCIdentityStore) GetByProviderSubject(_ context.Context, providerID uuid.UUID, subject string) (*store.OIDCIdentityRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, i := range m.identities {
		if i.ProviderID == providerID && i.Subject == subject {
			cp := *i
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockOIDCIdentityStore) ListByUser(_ context.Context, userID uuid.UUID) ([]store.OIDCIdentityRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var items []store.OIDCIdentityRecord
	for _, i := range m.identities {
		if i.UserID == userID {
			items = append(items, *i)
		}
	}
	return items, nil
}

func (m *mockOIDCIdentityStore) Delete(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.identities, id)
	return nil
}

func newOIDCTestServer(t *testing.T, ps store.OIDCProviderStore, is store.OIDCIdentityStore) (*Server, *http.ServeMux) {
	t.Helper()
	if ps == nil {
		ps = newMockOIDCProviderStore()
	}
	if is == nil {
		is = newMockOIDCIdentityStore()
	}
	srv, mux := newTestServer(&mockStore{})
	srv.oidcProviderStore = ps
	srv.oidcIdentityStore = is
	srv.InitOIDCHandler()
	return srv, mux
}

func TestOIDCHandlerCreateProvider(t *testing.T) {
	_, mux := newOIDCTestServer(t, nil, nil)

	body := bytes.NewBufferString(`{"name":"Google","issuer":"https://accounts.google.com","client_id":"cid","client_secret":"csec"}`)
	req := authRequest(http.MethodPost, "/api/v1/oidc/providers", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var got store.OIDCProviderRecord
	if err := decodeResponse(t, rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "Google" {
		t.Fatalf("name = %q", got.Name)
	}
	if !got.ClientSecretConfigured {
		t.Fatal("expected client_secret_configured=true")
	}
}

func TestOIDCHandlerListProviders(t *testing.T) {
	ps := newMockOIDCProviderStore()
	_, _ = ps.CreateProvider(context.Background(), &store.OIDCProviderRecord{
		Name: "P1", Issuer: "https://p1.example.com", ClientID: "c1", Enabled: true,
	}, "s1")

	_, mux := newOIDCTestServer(t, ps, nil)

	req := authRequest(http.MethodGet, "/api/v1/oidc/providers", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Items []store.OIDCProviderRecord `json:"items"`
		Total int64                      `json:"total"`
	}
	if err := decodeResponse(t, rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(resp.Items))
	}
	if resp.Items[0].Name != "P1" {
		t.Fatalf("name = %q", resp.Items[0].Name)
	}
}

func TestOIDCHandlerUpdateProvider(t *testing.T) {
	ps := newMockOIDCProviderStore()
	created, _ := ps.CreateProvider(context.Background(), &store.OIDCProviderRecord{
		Name: "Old", Issuer: "https://old.example.com", ClientID: "c", Enabled: true,
	}, "s")

	_, mux := newOIDCTestServer(t, ps, nil)

	body := bytes.NewBufferString(`{"name":"New","enabled":false}`)
	req := authRequest(http.MethodPut, "/api/v1/oidc/providers/"+created.ID.String(), body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got store.OIDCProviderRecord
	if err := decodeResponse(t, rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "New" {
		t.Fatalf("name = %q, want New", got.Name)
	}
	if got.Enabled {
		t.Fatal("expected enabled=false")
	}
}

func TestOIDCHandlerDeleteProvider(t *testing.T) {
	ps := newMockOIDCProviderStore()
	created, _ := ps.CreateProvider(context.Background(), &store.OIDCProviderRecord{
		Name: "Doomed", Issuer: "https://d.example.com", ClientID: "c", Enabled: true,
	}, "s")

	_, mux := newOIDCTestServer(t, ps, nil)

	req := authRequest(http.MethodDelete, "/api/v1/oidc/providers/"+created.ID.String(), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	req2 := authRequest(http.MethodGet, "/api/v1/oidc/providers/"+created.ID.String(), nil)
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", rec2.Code)
	}
}

func TestOIDCHandlerPublicProvidersList(t *testing.T) {
	ps := newMockOIDCProviderStore()
	_, _ = ps.CreateProvider(context.Background(), &store.OIDCProviderRecord{
		Name: "Enabled", Issuer: "https://e.example.com", ClientID: "c", Enabled: true,
	}, "s")
	_, _ = ps.CreateProvider(context.Background(), &store.OIDCProviderRecord{
		Name: "Disabled", Issuer: "https://d.example.com", ClientID: "c2", Enabled: false,
	}, "s2")

	_, mux := newOIDCTestServer(t, ps, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oidc/providers", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := decodeResponse(t, rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 enabled provider, got %d", len(got))
	}
	if got[0].Name != "Enabled" {
		t.Fatalf("name = %q, want Enabled", got[0].Name)
	}
}
