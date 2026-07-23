package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/config"
	algacrypto "alga/crypto"
	"alga/routing"
	"alga/secretprovider"
	"alga/store"
)

// --- mocks ---

type mockCredentialProviderStore struct {
	providers map[uuid.UUID]*store.CredentialProviderRecord
	configs   map[uuid.UUID]map[string]string
}

func (m *mockCredentialProviderStore) CreateProvider(_ context.Context, rec *store.CredentialProviderRecord, cfg map[string]string) (*store.CredentialProviderRecord, error) {
	if m.providers == nil {
		m.providers = map[uuid.UUID]*store.CredentialProviderRecord{}
	}
	if m.configs == nil {
		m.configs = map[uuid.UUID]map[string]string{}
	}
	rec.ID = uuid.New()
	rec.ConfigConfigured = len(cfg) > 0
	m.providers[rec.ID] = rec
	m.configs[rec.ID] = cfg
	out := *rec
	return &out, nil
}
func (m *mockCredentialProviderStore) UpdateProvider(_ context.Context, id uuid.UUID, patch *store.CredentialProviderRecord, cfg *map[string]string) (*store.CredentialProviderRecord, error) {
	ex, ok := m.providers[id]
	if !ok {
		return nil, errString("credential provider not found")
	}
	if patch.Name != "" {
		ex.Name = patch.Name
	}
	if patch.Type != "" {
		ex.Type = patch.Type
	}
	if patch.EnabledSet {
		ex.Enabled = patch.Enabled
	}
	if cfg != nil {
		m.configs[id] = *cfg
		ex.ConfigConfigured = len(*cfg) > 0
	}
	out := *ex
	return &out, nil
}
func (m *mockCredentialProviderStore) DeleteProvider(_ context.Context, id uuid.UUID) error {
	if _, ok := m.providers[id]; !ok {
		return errString("credential provider not found")
	}
	delete(m.providers, id)
	delete(m.configs, id)
	return nil
}
func (m *mockCredentialProviderStore) GetProvider(_ context.Context, id uuid.UUID) (*store.CredentialProviderRecord, error) {
	if ex, ok := m.providers[id]; ok {
		out := *ex
		return &out, nil
	}
	return nil, nil
}
func (m *mockCredentialProviderStore) GetProviderByName(_ context.Context, name string) (*store.CredentialProviderRecord, error) {
	for _, p := range m.providers {
		if p.Name == name {
			out := *p
			return &out, nil
		}
	}
	return nil, nil
}
func (m *mockCredentialProviderStore) GetProviderWithConfig(_ context.Context, id uuid.UUID) (*store.CredentialProviderRecord, error) {
	ex, ok := m.providers[id]
	if !ok {
		return nil, nil
	}
	out := *ex
	out.Config = m.configs[id]
	return &out, nil
}
func (m *mockCredentialProviderStore) ListProviders(_ context.Context, q store.CredentialProviderQuery) ([]store.CredentialProviderRecord, int64, error) {
	var out []store.CredentialProviderRecord
	for _, p := range m.providers {
		if q.Type != "" && p.Type != q.Type {
			continue
		}
		if q.Enabled != nil && p.Enabled != *q.Enabled {
			continue
		}
		out = append(out, *p)
	}
	return out, int64(len(out)), nil
}
func (m *mockCredentialProviderStore) SeedDefaultInternalProvider(_ context.Context) error {
	return nil
}

type mockSharedSecretStore struct {
	secrets map[uuid.UUID]*store.SharedSecretRecord
}

func (m *mockSharedSecretStore) CreateSecret(_ context.Context, rec *store.SharedSecretRecord, value string) (*store.SharedSecretRecord, error) {
	if m.secrets == nil {
		m.secrets = map[uuid.UUID]*store.SharedSecretRecord{}
	}
	rec.ID = uuid.New()
	rec.ValueConfigured = value != ""
	if rec.AllowedAgentIDs == nil {
		rec.AllowedAgentIDs = []uuid.UUID{}
	}
	m.secrets[rec.ID] = rec
	out := *rec
	return &out, nil
}
func (m *mockSharedSecretStore) UpdateSecret(_ context.Context, id uuid.UUID, patch *store.SharedSecretUpdate, value *string) (*store.SharedSecretRecord, error) {
	ex, ok := m.secrets[id]
	if !ok {
		return nil, errString("shared secret not found")
	}
	if patch.Name != nil {
		ex.Name = *patch.Name
	}
	if patch.Description != nil {
		ex.Description = *patch.Description
	}
	if patch.AllowedAgentIDs != nil {
		ex.AllowedAgentIDs = *patch.AllowedAgentIDs
	}
	if value != nil {
		ex.ValueConfigured = *value != ""
	}
	out := *ex
	return &out, nil
}
func (m *mockSharedSecretStore) DeleteSecret(_ context.Context, id uuid.UUID) error {
	if _, ok := m.secrets[id]; !ok {
		return errString("shared secret not found")
	}
	delete(m.secrets, id)
	return nil
}
func (m *mockSharedSecretStore) GetSecretByID(_ context.Context, id uuid.UUID) (*store.SharedSecretRecord, error) {
	if ex, ok := m.secrets[id]; ok {
		out := *ex
		return &out, nil
	}
	return nil, nil
}
func (m *mockSharedSecretStore) GetSecretBySecretID(_ context.Context, secretID string) (*store.SharedSecretRecord, error) {
	for _, s := range m.secrets {
		if s.SecretID == secretID {
			out := *s
			return &out, nil
		}
	}
	return nil, nil
}
func (m *mockSharedSecretStore) ListSecrets(_ context.Context, _ store.SharedSecretQuery) ([]store.SharedSecretRecord, int64, error) {
	out := make([]store.SharedSecretRecord, 0, len(m.secrets))
	for _, s := range m.secrets {
		out = append(out, *s)
	}
	return out, int64(len(out)), nil
}

type errString string

func (e errString) Error() string { return string(e) }

// --- test server ---

func newSharedSecretsTestServer(t *testing.T, ps store.CredentialProviderStore, ss store.SharedSecretStore, reg *secretprovider.Registry) (*Server, *http.ServeMux) {
	t.Helper()
	agentTok := &testAgentTokenStore{
		validToken: "secret-agent-token",
		agentID:    uuid.MustParse("00000000-0000-0000-0000-0000000000a1"),
		name:       "test-agent",
	}
	userStore := &mockUserStore{users: []store.UserRecord{testAdminUser}}
	sessionStore := &mockSessionStore{
		sessions: map[string]*store.SessionRecord{
			"test-session-id": {ID: "test-session-id", UserID: testAdminUser.ID, ExpiresAt: time.Now().Add(24 * time.Hour)},
		},
	}
	srv := NewServer(
		&config.Config{}, &mockStore{}, &mockWebhookTokenStore{tokens: map[string]store.WebhookTokenRecord{}},
		agentTok, userStore, sessionStore, &mockAuditStore{}, &mockIntegrationStore{}, &mockRouteRulesStore{},
		24*time.Hour, nil, nil, nil, nil, func(*routing.Engine) {},
		NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute), NewRateLimiter(10, 20),
		&mockAlertInvestigationStore{}, &mockIncidentInvestigationStore{}, nil,
		nil, nil, nil,
	)
	srv.SetCredentialStores(ps, ss, reg)
	mux := http.NewServeMux()
	srv.Register(mux)
	return srv, mux
}

func installAPITestCrypto(t *testing.T) {
	t.Helper()
	t.Setenv("SECRET_PEPPER", "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=")
	t.Setenv("ENCRYPTION_KEYS", "1:MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=")
	k, err := algacrypto.LoadFromEnv()
	if err != nil {
		t.Fatalf("crypto.LoadFromEnv: %v", err)
	}
	algacrypto.SetDefault(k)
}

// --- provider admin tests ---

func TestCreateAndListCredentialProviders(t *testing.T) {
	ps := &mockCredentialProviderStore{}
	_, mux := newSharedSecretsTestServer(t, ps, &mockSharedSecretStore{}, nil)

	body := bytes.NewBufferString(`{"name":"prod-vault","type":"hashicorp_vault","enabled":true,"config":{"address":"https://vault.example","token":"hvs.x"}}`)
	req := authRequest(http.MethodPost, "/api/v1/credential-providers", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created store.CredentialProviderRecord
	if err := decodeResponse(t, rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Type != "hashicorp_vault" || !created.ConfigConfigured {
		t.Fatalf("unexpected created provider: %+v", created)
	}

	listReq := authRequest(http.MethodGet, "/api/v1/credential-providers", nil)
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 list, got %d", listRec.Code)
	}
	if ps.configs[created.ID]["token"] != "hvs.x" {
		t.Fatalf("expected config stored, got %v", ps.configs[created.ID])
	}
}

func TestCreateCredentialProviderInvalidType(t *testing.T) {
	_, mux := newSharedSecretsTestServer(t, &mockCredentialProviderStore{}, &mockSharedSecretStore{}, nil)
	body := bytes.NewBufferString(`{"name":"bad","type":"not-a-type"}`)
	req := authRequest(http.MethodPost, "/api/v1/credential-providers", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid type, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- shared secret admin tests ---

func TestCreateSharedSecretRequiresExistingProvider(t *testing.T) {
	ps := &mockCredentialProviderStore{}
	_, mux := newSharedSecretsTestServer(t, ps, &mockSharedSecretStore{}, nil)

	body := bytes.NewBufferString(`{"provider_id":"` + uuid.New().String() + `","name":"x","secret_id":"x","value":"v"}`)
	req := authRequest(http.MethodPost, "/api/v1/shared-secrets", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when provider does not exist, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateSharedSecretSuccess(t *testing.T) {
	ps := &mockCredentialProviderStore{}
	prov, _ := ps.CreateProvider(context.Background(), &store.CredentialProviderRecord{
		Name: "internal", Type: store.CredentialProviderTypeInternal, Enabled: true,
	}, nil)
	ss := &mockSharedSecretStore{}
	_, mux := newSharedSecretsTestServer(t, ps, ss, nil)

	body := bytes.NewBufferString(`{"provider_id":"` + prov.ID.String() + `","name":"DB Password","value":"s3cr3t","description":"prod"}`)
	req := authRequest(http.MethodPost, "/api/v1/shared-secrets", body)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var created store.SharedSecretRecord
	if err := decodeResponse(t, rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !created.ValueConfigured {
		t.Fatal("expected value_configured=true")
	}
	// secret_id is server-generated (a UUID); any client value is ignored.
	if _, err := uuid.Parse(created.SecretID); err != nil {
		t.Fatalf("expected auto-generated UUID secret_id, got %q", created.SecretID)
	}
}

// --- agent fetch tests ---

func seedInternalSecret(t *testing.T, ps store.CredentialProviderStore, ss store.SharedSecretStore, secretID, value string, allowed []uuid.UUID) *store.SharedSecretRecord {
	t.Helper()
	installAPITestCrypto(t)
	prov, _ := ps.CreateProvider(context.Background(), &store.CredentialProviderRecord{
		Name: "internal", Type: store.CredentialProviderTypeInternal, Enabled: true,
	}, nil)
	enc, err := algacrypto.Default().EncryptString(value)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	rec := &store.SharedSecretRecord{
		ProviderID: prov.ID, Name: "seeded", SecretID: secretID,
		ValueEncrypted: enc, ValueConfigured: true, AllowedAgentIDs: allowed,
	}
	created, err := ss.CreateSecret(context.Background(), rec, value)
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	// Restore the real ciphertext the mock create would have dropped.
	ss.(*mockSharedSecretStore).secrets[created.ID].ValueEncrypted = enc
	return created
}

func TestAgentFetchSecretReturnsValue(t *testing.T) {
	installAPITestCrypto(t)
	ps := &mockCredentialProviderStore{}
	ss := &mockSharedSecretStore{}
	// Restricted by default: the bearer's agent token must be in the allow-list.
	seedInternalSecret(t, ps, ss, "db-password", "super-secret-value",
		[]uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-0000000000a1")})

	_, mux := newSharedSecretsTestServer(t, ps, ss, secretprovider.NewRegistry())
	req := agentAuthRequest(http.MethodGet, "/api/v1/agent/secrets/db-password", nil, "secret-agent-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := decodeResponse(t, rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["value"] != "super-secret-value" {
		t.Fatalf("expected plaintext value, got %v", got["value"])
	}
	if got["secret_id"] != "db-password" {
		t.Fatalf("expected secret_id echoed, got %v", got["secret_id"])
	}
}

func TestAgentFetchSecretNotFound(t *testing.T) {
	_, mux := newSharedSecretsTestServer(t, &mockCredentialProviderStore{}, &mockSharedSecretStore{}, secretprovider.NewRegistry())
	req := agentAuthRequest(http.MethodGet, "/api/v1/agent/secrets/missing", nil, "secret-agent-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentFetchSecretNotAllowedReturnsNotFound(t *testing.T) {
	installAPITestCrypto(t)
	ps := &mockCredentialProviderStore{}
	ss := &mockSharedSecretStore{}
	// Restrict to a different agent than the test bearer identity.
	seedInternalSecret(t, ps, ss, "restricted", "val", []uuid.UUID{uuid.New()})

	_, mux := newSharedSecretsTestServer(t, ps, ss, secretprovider.NewRegistry())
	req := agentAuthRequest(http.MethodGet, "/api/v1/agent/secrets/restricted", nil, "secret-agent-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unauthorized agent (no existence leak), got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentFetchSecretRequiresBearer(t *testing.T) {
	_, mux := newSharedSecretsTestServer(t, &mockCredentialProviderStore{}, &mockSharedSecretStore{}, secretprovider.NewRegistry())
	req := agentAuthRequest(http.MethodGet, "/api/v1/agent/secrets/anything", nil, "")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without bearer, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentFetchSecretRejectsNonGet(t *testing.T) {
	_, mux := newSharedSecretsTestServer(t, &mockCredentialProviderStore{}, &mockSharedSecretStore{}, secretprovider.NewRegistry())
	req := agentAuthRequest(http.MethodPost, "/api/v1/agent/secrets/anything", bytes.NewBufferString(`{}`), "secret-agent-token")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentFetchExternalSecretNotImplemented(t *testing.T) {
	installAPITestCrypto(t)
	ps := &mockCredentialProviderStore{}
	prov, _ := ps.CreateProvider(context.Background(), &store.CredentialProviderRecord{
		Name: "vault", Type: store.CredentialProviderTypeHashiCorpVault, Enabled: true,
	}, map[string]string{"address": "https://vault.example"})
	ss := &mockSharedSecretStore{}
	rec := &store.SharedSecretRecord{
		ProviderID: prov.ID, Name: "ext", SecretID: "ext-secret", RemoteRef: "secret/data/x",
		AllowedAgentIDs: []uuid.UUID{uuid.MustParse("00000000-0000-0000-0000-0000000000a1")},
	}
	created, _ := ss.CreateSecret(context.Background(), rec, "")

	_, mux := newSharedSecretsTestServer(t, ps, ss, secretprovider.NewRegistry())
	req := agentAuthRequest(http.MethodGet, "/api/v1/agent/secrets/"+created.SecretID, nil, "secret-agent-token")
	resp := httptest.NewRecorder()
	mux.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 for stubbed external provider, got %d: %s", resp.Code, resp.Body.String())
	}
}
