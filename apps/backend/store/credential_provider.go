package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	algacrypto "alga/crypto"
	"alga/ent"
	"alga/ent/credentialprovider"
	"alga/secretprovider"
)

// Credential provider type identifiers. These match secretprovider.Type* and
// are duplicated here so callers can validate without importing the resolver.
const (
	CredentialProviderTypeInternal       = "internal"
	CredentialProviderTypeHashiCorpVault = "hashicorp_vault"
	CredentialProviderTypeAWSSecretsMgr  = "aws_secrets_manager"
	CredentialProviderTypeGCPSecretMgr   = "gcp_secret_manager"
	CredentialProviderTypeAzureKeyVault  = "azure_key_vault"
)

// IsValidCredentialProviderType reports whether t is a selectable provider type.
func IsValidCredentialProviderType(t string) bool {
	return secretprovider.IsValidType(t)
}

// CredentialProviderRecord is the wire/persistence shape for a credential
// provider. Config carries decrypted connection details and MUST NOT be
// serialized in list responses; use ConfigConfigured there. Load decrypted
// config only via GetProviderWithConfig.
type CredentialProviderRecord struct {
	ID               uuid.UUID         `json:"id"`
	Name             string            `json:"name"`
	Type             string            `json:"type"`
	Enabled          bool              `json:"enabled"`
	System           bool              `json:"system"`
	ConfigConfigured bool              `json:"config_configured"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	EnabledSet       bool              `json:"-"`
	Config           map[string]string `json:"-"`
	ProviderTypeName string            `json:"provider_type_name,omitempty"`
}

type CredentialProviderQuery struct {
	Type    string
	Search  string
	Enabled *bool
	Limit   int
	Skip    int
}

type CredentialProviderStore interface {
	CreateProvider(ctx context.Context, record *CredentialProviderRecord, config map[string]string) (*CredentialProviderRecord, error)
	UpdateProvider(ctx context.Context, id uuid.UUID, patch *CredentialProviderRecord, config *map[string]string) (*CredentialProviderRecord, error)
	DeleteProvider(ctx context.Context, id uuid.UUID) error
	GetProvider(ctx context.Context, id uuid.UUID) (*CredentialProviderRecord, error)
	GetProviderByName(ctx context.Context, name string) (*CredentialProviderRecord, error)
	GetProviderWithConfig(ctx context.Context, id uuid.UUID) (*CredentialProviderRecord, error)
	ListProviders(ctx context.Context, q CredentialProviderQuery) ([]CredentialProviderRecord, int64, error)
	SeedDefaultInternalProvider(ctx context.Context) error
}

type pgCredentialProviderStore struct {
	pgStoreBase
}

func newPGCredentialProviderStore(client *ent.Client) CredentialProviderStore {
	return &pgCredentialProviderStore{pgStoreBase{client: client}}
}

func normalizeProviderType(t string) (string, error) {
	t = normalizeLower(t)
	if !IsValidCredentialProviderType(t) {
		return "", fmt.Errorf("invalid provider type %q", t)
	}
	return t, nil
}

func (s *pgCredentialProviderStore) CreateProvider(ctx context.Context, record *CredentialProviderRecord, config map[string]string) (*CredentialProviderRecord, error) {
	if record == nil {
		return nil, errors.New("nil record")
	}
	pt, err := normalizeProviderType(record.Type)
	if err != nil {
		return nil, err
	}
	record.Type = pt
	if record.Name == "" {
		return nil, errors.New("name is required")
	}
	encConfig, err := encryptProviderConfig(config)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now
	record.ConfigConfigured = len(config) > 0

	saved, err := s.client.CredentialProvider.Create().
		SetName(record.Name).
		SetType(pt).
		SetConfigEncrypted(encConfig).
		SetEnabled(record.Enabled).
		SetSystem(record.System).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert credential provider: %w", err)
	}
	out := pgCredentialProviderToRecord(saved)
	out.ConfigConfigured = record.ConfigConfigured
	return out, nil
}

func (s *pgCredentialProviderStore) UpdateProvider(ctx context.Context, id uuid.UUID, patch *CredentialProviderRecord, config *map[string]string) (*CredentialProviderRecord, error) {
	if patch == nil {
		return nil, errors.New("nil patch")
	}
	// System providers (the seeded default) are fully immutable: they cannot be
	// renamed, retyped, reconfigured, enabled, or disabled through the API.
	existing, err := s.client.CredentialProvider.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrCredentialProviderNotFound
		}
		return nil, fmt.Errorf("failed to load credential provider: %w", err)
	}
	if existing.System {
		return nil, ErrSystemCredentialProvider
	}
	b := s.client.CredentialProvider.UpdateOneID(id).SetUpdatedAt(time.Now().UTC())
	if patch.Name != "" {
		b.SetName(patch.Name)
	}
	if patch.Type != "" {
		pt, err := normalizeProviderType(patch.Type)
		if err != nil {
			return nil, err
		}
		b.SetType(pt)
	}
	if patch.EnabledSet {
		b.SetEnabled(patch.Enabled)
	}
	configConfigured := patch.ConfigConfigured
	if config != nil {
		enc, err := encryptProviderConfig(*config)
		if err != nil {
			return nil, err
		}
		b.SetConfigEncrypted(enc)
		configConfigured = len(*config) > 0
	}

	saved, err := b.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrCredentialProviderNotFound
		}
		return nil, fmt.Errorf("failed to update credential provider: %w", err)
	}
	out := pgCredentialProviderToRecord(saved)
	if config != nil {
		out.ConfigConfigured = configConfigured
	} else {
		existing, _ := s.client.CredentialProvider.Get(ctx, id)
		out.ConfigConfigured = existing != nil && existing.ConfigEncrypted != ""
	}
	return out, nil
}

func (s *pgCredentialProviderStore) DeleteProvider(ctx context.Context, id uuid.UUID) error {
	existing, err := s.client.CredentialProvider.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrCredentialProviderNotFound
		}
		return fmt.Errorf("failed to load credential provider: %w", err)
	}
	if existing.System {
		return ErrSystemCredentialProvider
	}
	err = s.client.CredentialProvider.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrCredentialProviderNotFound
		}
		return fmt.Errorf("failed to delete credential provider: %w", err)
	}
	return nil
}

func (s *pgCredentialProviderStore) GetProvider(ctx context.Context, id uuid.UUID) (*CredentialProviderRecord, error) {
	p, err := s.client.CredentialProvider.Get(ctx, id)
	if err != nil {
		return handleQueryErr[*CredentialProviderRecord](err, "credential provider")
	}
	out := pgCredentialProviderToRecord(p)
	out.ConfigConfigured = p.ConfigEncrypted != ""
	return out, nil
}

func (s *pgCredentialProviderStore) GetProviderByName(ctx context.Context, name string) (*CredentialProviderRecord, error) {
	p, err := s.client.CredentialProvider.Query().
		Where(credentialprovider.NameEQ(name)).
		Only(ctx)
	if err != nil {
		return handleQueryErr[*CredentialProviderRecord](err, "credential provider")
	}
	out := pgCredentialProviderToRecord(p)
	out.ConfigConfigured = p.ConfigEncrypted != ""
	return out, nil
}

func (s *pgCredentialProviderStore) GetProviderWithConfig(ctx context.Context, id uuid.UUID) (*CredentialProviderRecord, error) {
	p, err := s.client.CredentialProvider.Get(ctx, id)
	if err != nil {
		return handleQueryErr[*CredentialProviderRecord](err, "credential provider")
	}
	out := pgCredentialProviderToRecord(p)
	cfg, err := decryptProviderConfig(p.ConfigEncrypted)
	if err != nil {
		return nil, err
	}
	out.Config = cfg
	out.ConfigConfigured = p.ConfigEncrypted != ""
	return out, nil
}

func (s *pgCredentialProviderStore) ListProviders(ctx context.Context, q CredentialProviderQuery) ([]CredentialProviderRecord, int64, error) {
	query := s.client.CredentialProvider.Query()
	if t := normalizeLower(q.Type); t != "" {
		if !IsValidCredentialProviderType(t) {
			return nil, 0, fmt.Errorf("invalid provider type %q", t)
		}
		query = query.Where(credentialprovider.TypeEQ(t))
	}
	if q.Enabled != nil {
		query = query.Where(credentialprovider.EnabledEQ(*q.Enabled))
	}
	if q.Search != "" {
		query = query.Where(credentialprovider.NameContains(q.Search))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count credential providers: %w", err)
	}
	query = query.Order(ent.Desc(credentialprovider.FieldCreatedAt))
	if q.Limit > 0 {
		query = query.Limit(q.Limit)
	}
	if q.Skip > 0 {
		query = query.Offset(q.Skip)
	}
	items, err := query.All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list credential providers: %w", err)
	}
	out := make([]CredentialProviderRecord, 0, len(items))
	for _, p := range items {
		rec := pgCredentialProviderToRecord(p)
		rec.ConfigConfigured = p.ConfigEncrypted != ""
		out = append(out, *rec)
	}
	return out, int64(total), nil
}

// DefaultInternalProviderName is the well-known name of the seeded system
// provider that stores secrets encrypted inside Alga.
const DefaultInternalProviderName = "Alga Internal"

// SeedDefaultInternalProvider ensures the built-in "Alga Internal" provider
// exists, is marked system, enabled, and typed internal. It is idempotent: if a
// system internal provider already exists it is left alone. It runs at startup
// so the secret store is usable without any admin setup.
func (s *pgCredentialProviderStore) SeedDefaultInternalProvider(ctx context.Context) error {
	existing, err := s.client.CredentialProvider.Query().
		Where(credentialprovider.SystemEQ(true), credentialprovider.TypeEQ(CredentialProviderTypeInternal)).
		First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("failed to check default credential provider: %w", err)
	}
	if existing != nil {
		return nil
	}
	// Name uniqueness: if a non-system provider already occupies the default
	// name (e.g. created manually before seeding), rename it aside so the seed
	// can claim the canonical name.
	if clash, _ := s.GetProviderByName(ctx, DefaultInternalProviderName); clash != nil {
		_, _ = s.client.CredentialProvider.UpdateOneID(clash.ID).
			SetName(DefaultInternalProviderName + " (legacy)").
			Save(ctx)
	}
	now := time.Now().UTC()
	_, err = s.client.CredentialProvider.Create().
		SetName(DefaultInternalProviderName).
		SetType(CredentialProviderTypeInternal).
		SetEnabled(true).
		SetSystem(true).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to seed default credential provider: %w", err)
	}
	return nil
}

// encryptProviderConfig serializes config to JSON and encrypts the blob. An
// empty/nil config encrypts to the empty string so internal providers store no
// ciphertext.
func encryptProviderConfig(config map[string]string) (string, error) {
	if len(config) == 0 {
		return "", nil
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal provider config: %w", err)
	}
	enc, err := algacrypto.Default().EncryptString(string(raw))
	if err != nil {
		return "", fmt.Errorf("failed to encrypt provider config: %w", err)
	}
	return enc, nil
}

// decryptProviderConfig reverses encryptProviderConfig. An empty blob yields nil.
func decryptProviderConfig(enc string) (map[string]string, error) {
	if enc == "" {
		return nil, nil
	}
	pt, err := algacrypto.Default().DecryptString(enc)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt provider config: %w", err)
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(pt), &out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal provider config: %w", err)
	}
	return out, nil
}

func providerTypeDisplayName(t string) string {
	switch t {
	case CredentialProviderTypeInternal:
		return "Alga Internal"
	case CredentialProviderTypeHashiCorpVault:
		return "HashiCorp Vault"
	case CredentialProviderTypeAWSSecretsMgr:
		return "AWS Secrets Manager"
	case CredentialProviderTypeGCPSecretMgr:
		return "GCP Secret Manager"
	case CredentialProviderTypeAzureKeyVault:
		return "Azure Key Vault"
	default:
		return t
	}
}

func pgCredentialProviderToRecord(p *ent.CredentialProvider) *CredentialProviderRecord {
	return &CredentialProviderRecord{
		ID:               p.ID,
		Name:             p.Name,
		Type:             p.Type,
		Enabled:          p.Enabled,
		System:           p.System,
		ProviderTypeName: providerTypeDisplayName(p.Type),
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        p.UpdatedAt,
	}
}
