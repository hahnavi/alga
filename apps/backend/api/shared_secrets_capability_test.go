package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"alga/api/platform"
	"alga/capability"
	"alga/config"
	algacrypto "alga/crypto"
	"alga/logger"
	"alga/secretprovider"
	"alga/store"
)

// ---- stubs -----------------------------------------------------------------

type b6SecretStore struct {
	store.SharedSecretStore
	rec      *store.SharedSecretRecord
	queries  int
	bySecret map[string]*store.SharedSecretRecord
}

func (s *b6SecretStore) GetSecretBySecretID(ctx context.Context, secretID string) (*store.SharedSecretRecord, error) {
	s.queries++
	if s.bySecret != nil {
		return s.bySecret[secretID], nil
	}
	return s.rec, nil
}

type b6ProviderStore struct {
	store.CredentialProviderStore
	rec *store.CredentialProviderRecord
}

func (s *b6ProviderStore) GetProviderWithConfig(ctx context.Context, id uuid.UUID) (*store.CredentialProviderRecord, error) {
	return s.rec, nil
}

// agentContextB6 injects an agent identity as AgentBearerMiddleware would.
func agentContextB6(id uuid.UUID, caps []string) context.Context {
	return platform.WithAgent(context.Background(), &platform.AgentTokenContext{
		ID:           id,
		Name:         "probe-agent",
		Capabilities: caps,
	})
}

// mustEncryptB6 installs the test keyring and returns a ciphertext the
// internal provider can decrypt.
func mustEncryptB6(t *testing.T, plaintext string) string {
	t.Helper()
	t.Setenv("SECRET_PEPPER", "MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=")
	t.Setenv("ENCRYPTION_KEYS", "1:MTIzNDU2Nzg5MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTI=")
	k, err := algacrypto.LoadFromEnv()
	if err != nil {
		t.Fatalf("crypto.LoadFromEnv: %v", err)
	}
	algacrypto.SetDefault(k)
	enc, err := algacrypto.Default().EncryptString(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	return enc
}

// ---- tests -----------------------------------------------------------------

// TestCapabilityCatalogIncludesSecrets asserts the fixed catalog grew to
// four entries and normalization still dedupes/sorts (GET /agent/capabilities
// iterates capability.All and picks the new entry up automatically).
func TestCapabilityCatalogIncludesSecrets(t *testing.T) {
	t.Parallel()

	if !capability.Has(capability.All, capability.Secrets) {
		t.Fatal("capability.All missing secrets")
	}
	if len(capability.All) != 4 {
		t.Fatalf("capability.All = %v, want 4 entries", capability.All)
	}
	got := capability.Normalize([]string{"secrets", "investigate", "secrets"})
	want := []string{"investigate", "secrets"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("Normalize = %v, want %v", got, want)
	}
	if err := capability.Validate([]string{capability.Secrets}); err != nil {
		t.Fatalf("Validate should accept secrets: %v", err)
	}
}

// TestAgentSecretRequiresCapability covers acceptance:
//
//   - token WITHOUT `secrets` never reaches the store; denial body is byte-
//     identical to an unknown-secret denial (no existence oracle)
//   - token WITH `secrets` + allow-listed fetches plaintext + audit row
func TestAgentSecretRequiresCapability(t *testing.T) {
	logger.Init("error", "")

	agentAllowed := uuid.New()
	enc := mustEncryptB6(t, "super-secret-value")

	rec := &store.SharedSecretRecord{
		ID:             uuid.New(),
		SecretID:       "sec_demo_00000000",
		Name:           "api-key",
		ProviderID:     uuid.New(),
		ValueEncrypted: enc,
	}
	rec.AllowedAgentIDs = []uuid.UUID{agentAllowed}

	secretStore := &b6SecretStore{rec: rec}
	s := &Server{
		sharedSecretStore: secretStore,
		credentialProviderStore: &b6ProviderStore{rec: &store.CredentialProviderRecord{
			ID: rec.ProviderID, Type: "internal", Enabled: true,
		}},
		secretProviderRegistry: secretprovider.NewRegistry(),
		ipExtractor:            newIPExtractor(&config.Config{}),
	}

	fetch := func(caps []string, agentID uuid.UUID) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/secrets/sec_demo_00000000", nil)
		req.SetPathValue("secret_id", "sec_demo_00000000")
		req = req.WithContext(agentContextB6(agentID, caps))
		w := httptest.NewRecorder()
		s.handleAgentSecretByID(w, req)
		return w
	}

	denied := fetch([]string{"investigate"}, agentAllowed)
	if denied.Code != http.StatusNotFound {
		t.Fatalf("capability denial status = %d, want 404", denied.Code)
	}

	unknown := &Server{sharedSecretStore: secretStore}
	reqUnknown := httptest.NewRequest(http.MethodGet, "/api/v1/agent/secrets/nope", nil)
	reqUnknown.SetPathValue("secret_id", "does-not-exist")
	reqUnknown = reqUnknown.WithContext(agentContextB6(uuid.New(), []string{"investigate"}))
	recUnknown := httptest.NewRecorder()
	unknown.handleAgentSecretByID(recUnknown, reqUnknown)

	if recUnknown.Code != http.StatusNotFound {
		t.Fatalf("unknown-secret status = %d, want 404", recUnknown.Code)
	}
	// Byte-identical bodies between capability denial and unknown-secret
	// denial (anti-enumeration R8).
	if denied.Body.String() != recUnknown.Body.String() {
		t.Fatalf("denial bodies differ:\n capability: %s\n unknown:    %s",
			denied.Body.String(), recUnknown.Body.String())
	}
	// The gate fires BEFORE any store lookup.
	if secretStore.queries != 0 {
		t.Fatalf("store queried %d times under capability denial, want 0", secretStore.queries)
	}

	allowed := fetch([]string{"investigate", capability.Secrets}, agentAllowed)
	if allowed.Code != http.StatusOK {
		t.Fatalf("allow-listed fetch with secrets cap status = %d, want 200; body: %s",
			allowed.Code, allowed.Body.String())
	}
	var out struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(allowed.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, allowed.Body.String())
	}
	if out.Value != "super-secret-value" {
		t.Fatalf("fetched value = %q", out.Value)
	}
	if secretStore.queries != 1 {
		t.Fatalf("store queries = %d, want 1 for the authorized fetch", secretStore.queries)
	}
}

// TestAllowListStillDeniesUnlistedAgent: capability alone is not enough —
// the per-secret allow-list remains authoritative.
func TestAllowListStillDeniesUnlistedAgent(t *testing.T) {
	t.Parallel()

	logger.Init("error", "")
	rec := &store.SharedSecretRecord{
		ID:         uuid.New(),
		SecretID:   "sec_other_0000000",
		Name:       "api-key",
		ProviderID: uuid.New(),
		AllowedAgentIDs: []uuid.UUID{
			uuid.New(), // some other agent
		},
	}
	secretStore := &b6SecretStore{rec: rec}
	s := &Server{
		sharedSecretStore: secretStore,
		ipExtractor:       newIPExtractor(&config.Config{}),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/secrets/sec_other_0000000", nil)
	req.SetPathValue("secret_id", "sec_other_0000000")
	req = req.WithContext(agentContextB6(uuid.New(), []string{capability.Secrets}))
	w := httptest.NewRecorder()
	s.handleAgentSecretByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if !strings.Contains(w.Body.String(), "secret not found") {
		t.Fatalf("body = %s, want generic not-found", w.Body.String())
	}
}
