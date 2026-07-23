package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func newCredentialTestStores(t *testing.T) (CredentialProviderStore, SharedSecretStore) {
	t.Helper()
	installTestKeyring(t)
	client := newTestEntClient(t)
	return newPGCredentialProviderStore(client), newPGSharedSecretStore(client)
}

func TestCredentialProviderCreateGetList(t *testing.T) {
	ps, _ := newCredentialTestStores(t)
	ctx := context.Background()

	out, err := ps.CreateProvider(ctx, &CredentialProviderRecord{
		Name:    "prod-vault",
		Type:    CredentialProviderTypeHashiCorpVault,
		Enabled: true,
	}, map[string]string{"address": "https://vault.example", "token": "hvs.x"})
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if out.ID == uuid.Nil {
		t.Fatal("expected non-nil id")
	}
	if !out.ConfigConfigured {
		t.Fatal("expected config_configured=true")
	}
	if out.Type != CredentialProviderTypeHashiCorpVault {
		t.Fatalf("type = %q", out.Type)
	}

	got, err := ps.GetProvider(ctx, out.ID)
	if err != nil {
		t.Fatalf("GetProvider: %v", err)
	}
	if got == nil || got.Name != "prod-vault" {
		t.Fatalf("unexpected provider: %+v", got)
	}

	items, total, err := ps.ListProviders(ctx, CredentialProviderQuery{Limit: 10})
	if err != nil {
		t.Fatalf("ListProviders: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected 1 provider, got %d (%d)", total, len(items))
	}
}

func TestCredentialProviderGetWithConfigDecrypts(t *testing.T) {
	ps, _ := newCredentialTestStores(t)
	ctx := context.Background()

	created, err := ps.CreateProvider(ctx, &CredentialProviderRecord{
		Name: "aws-prod", Type: CredentialProviderTypeAWSSecretsMgr, Enabled: true,
	}, map[string]string{"region": "us-east-1", "access_key_id": "AKIA"})
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	got, err := ps.GetProviderWithConfig(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetProviderWithConfig: %v", err)
	}
	if got.Config["region"] != "us-east-1" {
		t.Fatalf("expected decrypted region, got %v", got.Config)
	}
	if got.Config["access_key_id"] != "AKIA" {
		t.Fatalf("expected decrypted access_key_id, got %v", got.Config)
	}
}

func TestCredentialProviderUpdateAndDelete(t *testing.T) {
	ps, _ := newCredentialTestStores(t)
	ctx := context.Background()

	created, err := ps.CreateProvider(ctx, &CredentialProviderRecord{
		Name: "internal-default", Type: CredentialProviderTypeInternal, Enabled: true,
	}, nil)
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}

	disabled := false
	updated, err := ps.UpdateProvider(ctx, created.ID, &CredentialProviderRecord{
		Name: "internal-main", EnabledSet: true, Enabled: disabled,
	}, nil)
	if err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	if updated.Name != "internal-main" {
		t.Fatalf("name = %q", updated.Name)
	}
	if updated.Enabled {
		t.Fatal("expected disabled after update")
	}

	if err := ps.DeleteProvider(ctx, created.ID); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
	if got, _ := ps.GetProvider(ctx, created.ID); got != nil {
		t.Fatal("expected provider gone after delete")
	}
}

func TestCredentialProviderInvalidType(t *testing.T) {
	ps, _ := newCredentialTestStores(t)
	ctx := context.Background()
	if _, err := ps.CreateProvider(ctx, &CredentialProviderRecord{
		Name: "bad", Type: "not-a-type", Enabled: true,
	}, nil); err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestSharedSecretInternalRoundTrip(t *testing.T) {
	ps, ss := newCredentialTestStores(t)
	ctx := context.Background()

	prov, err := ps.CreateProvider(ctx, &CredentialProviderRecord{
		Name: "internal", Type: CredentialProviderTypeInternal, Enabled: true,
	}, nil)
	if err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}

	created, err := ss.CreateSecret(ctx, &SharedSecretRecord{
		ProviderID:  prov.ID,
		Name:        "DB Password",
		SecretID:    "db-password",
		Description: "prod db root",
	}, "s3cr3t-value")
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	if !created.ValueConfigured {
		t.Fatal("expected value_configured=true")
	}
	if created.ValueEncrypted == "" || created.ValueEncrypted == "s3cr3t-value" {
		t.Fatalf("expected ciphertext stored, got %q", created.ValueEncrypted)
	}
	if created.SecretID != "db-password" {
		t.Fatalf("secret_id normalized = %q", created.SecretID)
	}

	// Lookup is case-insensitive / trimmed.
	got, err := ss.GetSecretBySecretID(ctx, "  DB-Password  ")
	if err != nil {
		t.Fatalf("GetSecretBySecretID: %v", err)
	}
	if got == nil || got.ID != created.ID {
		t.Fatalf("unexpected secret: %+v", got)
	}
}

func TestSharedSecretAllowedAgentEnforcement(t *testing.T) {
	ps, ss := newCredentialTestStores(t)
	ctx := context.Background()

	prov, _ := ps.CreateProvider(ctx, &CredentialProviderRecord{
		Name: "internal", Type: CredentialProviderTypeInternal, Enabled: true,
	}, nil)

	allowed := []uuid.UUID{uuid.New(), uuid.New()}
	created, err := ss.CreateSecret(ctx, &SharedSecretRecord{
		ProviderID: prov.ID, Name: "restricted", SecretID: "restricted-secret",
		AllowedAgentIDs: allowed,
	}, "val")
	if err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	if !created.AllowedAgentIDsSet() {
		t.Fatal("expected allow-list configured")
	}
	if !created.AgentAllowed(allowed[0]) {
		t.Fatal("expected allowed agent permitted")
	}
	if created.AgentAllowed(uuid.New()) {
		t.Fatal("expected unknown agent rejected")
	}

	// Locked secret: an empty allow-list denies all agents (restricted by default).
	locked, err := ss.CreateSecret(ctx, &SharedSecretRecord{
		ProviderID: prov.ID, Name: "locked", SecretID: "locked-secret",
	}, "val")
	if err != nil {
		t.Fatalf("CreateSecret locked: %v", err)
	}
	if locked.AllowedAgentIDsSet() {
		t.Fatal("expected locked secret to have no allow-list")
	}
	if locked.AgentAllowed(uuid.New()) {
		t.Fatal("expected locked secret to deny all agents")
	}
}

func TestSharedSecretUpdateValueRotation(t *testing.T) {
	ps, ss := newCredentialTestStores(t)
	ctx := context.Background()

	prov, _ := ps.CreateProvider(ctx, &CredentialProviderRecord{
		Name: "internal", Type: CredentialProviderTypeInternal, Enabled: true,
	}, nil)
	created, _ := ss.CreateSecret(ctx, &SharedSecretRecord{
		ProviderID: prov.ID, Name: "rot", SecretID: "rot",
	}, "old")

	newVal := "new-value"
	updated, err := ss.UpdateSecret(ctx, created.ID, &SharedSecretUpdate{}, &newVal)
	if err != nil {
		t.Fatalf("UpdateSecret: %v", err)
	}
	if !updated.ValueConfigured || updated.ValueEncrypted == created.ValueEncrypted {
		t.Fatal("expected ciphertext to change after rotation")
	}

	// Re-resolve via provider to confirm the new value decrypts.
	got, _ := ss.GetSecretBySecretID(ctx, "rot")
	if got.ValueEncrypted != updated.ValueEncrypted {
		t.Fatal("expected fetched record to carry rotated ciphertext")
	}
}

func TestSharedSecretDelete(t *testing.T) {
	ps, ss := newCredentialTestStores(t)
	ctx := context.Background()
	prov, _ := ps.CreateProvider(ctx, &CredentialProviderRecord{
		Name: "internal", Type: CredentialProviderTypeInternal, Enabled: true,
	}, nil)
	created, _ := ss.CreateSecret(ctx, &SharedSecretRecord{
		ProviderID: prov.ID, Name: "tmp", SecretID: "tmp",
	}, "v")

	if err := ss.DeleteSecret(ctx, created.ID); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	if got, _ := ss.GetSecretByID(ctx, created.ID); got != nil {
		t.Fatal("expected secret gone after delete")
	}
}

func TestSeedDefaultInternalProviderIdempotent(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	ps := newPGCredentialProviderStore(client)
	ctx := context.Background()

	if err := ps.SeedDefaultInternalProvider(ctx); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	got, err := ps.GetProviderByName(ctx, DefaultInternalProviderName)
	if err != nil || got == nil {
		t.Fatalf("expected seeded provider, got %v %v", got, err)
	}
	if !got.System || got.Type != CredentialProviderTypeInternal || !got.Enabled {
		t.Fatalf("seeded provider wrong shape: %+v", got)
	}

	// Second seed must be a no-op (not create a duplicate, not error).
	if err := ps.SeedDefaultInternalProvider(ctx); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	items, total, _ := ps.ListProviders(ctx, CredentialProviderQuery{Limit: 50})
	_ = items
	if total != 1 {
		t.Fatalf("expected exactly 1 provider after reseed, got %d", total)
	}
}

func TestSystemProviderIsNonRemovable(t *testing.T) {
	installTestKeyring(t)
	client := newTestEntClient(t)
	ps := newPGCredentialProviderStore(client)
	ctx := context.Background()
	if err := ps.SeedDefaultInternalProvider(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}
	def, _ := ps.GetProviderByName(ctx, DefaultInternalProviderName)
	if def == nil {
		t.Fatal("expected seeded default provider")
	}

	// Delete is refused.
	if err := ps.DeleteProvider(ctx, def.ID); !errors.Is(err, ErrSystemCredentialProvider) {
		t.Fatalf("expected ErrSystemCredentialProvider on delete, got %v", err)
	}
	// Type change is refused.
	if _, err := ps.UpdateProvider(ctx, def.ID, &CredentialProviderRecord{Type: CredentialProviderTypeHashiCorpVault}, nil); !errors.Is(err, ErrSystemCredentialProvider) {
		t.Fatalf("expected ErrSystemCredentialProvider on type change, got %v", err)
	}
	// Disable is refused.
	if _, err := ps.UpdateProvider(ctx, def.ID, &CredentialProviderRecord{EnabledSet: true, Enabled: false}, nil); !errors.Is(err, ErrSystemCredentialProvider) {
		t.Fatalf("expected ErrSystemCredentialProvider on disable, got %v", err)
	}
	// Rename is refused (system providers are fully immutable).
	if _, err := ps.UpdateProvider(ctx, def.ID, &CredentialProviderRecord{Name: "Alga Internal"}, nil); !errors.Is(err, ErrSystemCredentialProvider) {
		t.Fatalf("expected ErrSystemCredentialProvider on rename, got %v", err)
	}
}
