package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	algacrypto "alga/crypto"
	"alga/db/models"
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

func newPGCredentialProviderStore(db *bun.DB) CredentialProviderStore {
	return &pgCredentialProviderStore{pgStoreBase{db: db}}
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

	m := &models.CredentialProvider{
		Name:            record.Name,
		Type:            pt,
		ConfigEncrypted: encConfig,
		Enabled:         record.Enabled,
		System:          record.System,
	}
	m.ID = models.NewUUID()
	m.CreatedAt = now
	m.UpdatedAt = now

	_, err = s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert credential provider: %w", err)
	}
	out := pgCredentialProviderToRecord(m)
	out.ConfigConfigured = record.ConfigConfigured
	return out, nil
}

func (s *pgCredentialProviderStore) UpdateProvider(ctx context.Context, id uuid.UUID, patch *CredentialProviderRecord, config *map[string]string) (*CredentialProviderRecord, error) {
	if patch == nil {
		return nil, errors.New("nil patch")
	}
	// System providers (the seeded default) are fully immutable.
	var existing models.CredentialProvider
	err := s.db.NewSelect().Model(&existing).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrCredentialProviderNotFound
		}
		return nil, fmt.Errorf("failed to load credential provider: %w", err)
	}
	if existing.System {
		return nil, ErrSystemCredentialProvider
	}

	upd := s.db.NewUpdate().Model((*models.CredentialProvider)(nil)).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id)

	if patch.Name != "" {
		upd = upd.Set("name = ?", patch.Name)
	}
	if patch.Type != "" {
		pt, err := normalizeProviderType(patch.Type)
		if err != nil {
			return nil, err
		}
		upd = upd.Set("type = ?", pt)
	}
	if patch.EnabledSet {
		upd = upd.Set("enabled = ?", patch.Enabled)
	}
	configConfigured := patch.ConfigConfigured
	if config != nil {
		enc, err := encryptProviderConfig(*config)
		if err != nil {
			return nil, err
		}
		upd = upd.Set("config_encrypted = ?", enc)
		configConfigured = len(*config) > 0
	}

	res, err := upd.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update credential provider: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to update credential provider: %w", err)
	}
	if n == 0 {
		return nil, ErrCredentialProviderNotFound
	}

	var saved models.CredentialProvider
	err = s.db.NewSelect().Model(&saved).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to reload credential provider: %w", err)
	}
	out := pgCredentialProviderToRecord(&saved)
	if config != nil {
		out.ConfigConfigured = configConfigured
	} else {
		out.ConfigConfigured = saved.ConfigEncrypted != ""
	}
	return out, nil
}

func (s *pgCredentialProviderStore) DeleteProvider(ctx context.Context, id uuid.UUID) error {
	var existing models.CredentialProvider
	err := s.db.NewSelect().Model(&existing).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return ErrCredentialProviderNotFound
		}
		return fmt.Errorf("failed to load credential provider: %w", err)
	}
	if existing.System {
		return ErrSystemCredentialProvider
	}
	res, err := s.db.NewDelete().Model((*models.CredentialProvider)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		if IsForeignKeyViolation(err) {
			// shared_secrets.provider_id is ON DELETE RESTRICT: the provider
			// still owns secrets. Surface as an actionable sentinel (WP-B6).
			return ErrCredentialProviderInUse
		}
		return fmt.Errorf("failed to delete credential provider: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to delete credential provider: %w", err)
	}
	if n == 0 {
		return ErrCredentialProviderNotFound
	}
	return nil
}

func (s *pgCredentialProviderStore) GetProvider(ctx context.Context, id uuid.UUID) (*CredentialProviderRecord, error) {
	var p models.CredentialProvider
	err := s.db.NewSelect().Model(&p).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*CredentialProviderRecord](err, "credential provider")
	}
	out := pgCredentialProviderToRecord(&p)
	out.ConfigConfigured = p.ConfigEncrypted != ""
	return out, nil
}

func (s *pgCredentialProviderStore) GetProviderByName(ctx context.Context, name string) (*CredentialProviderRecord, error) {
	var p models.CredentialProvider
	err := s.db.NewSelect().Model(&p).Where("name = ?", name).Scan(ctx)
	if err != nil {
		return handleQueryErr[*CredentialProviderRecord](err, "credential provider")
	}
	out := pgCredentialProviderToRecord(&p)
	out.ConfigConfigured = p.ConfigEncrypted != ""
	return out, nil
}

func (s *pgCredentialProviderStore) GetProviderWithConfig(ctx context.Context, id uuid.UUID) (*CredentialProviderRecord, error) {
	var p models.CredentialProvider
	err := s.db.NewSelect().Model(&p).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*CredentialProviderRecord](err, "credential provider")
	}
	out := pgCredentialProviderToRecord(&p)
	cfg, err := decryptProviderConfig(p.ConfigEncrypted)
	if err != nil {
		return nil, err
	}
	out.Config = cfg
	out.ConfigConfigured = p.ConfigEncrypted != ""
	return out, nil
}

func (s *pgCredentialProviderStore) ListProviders(ctx context.Context, q CredentialProviderQuery) ([]CredentialProviderRecord, int64, error) {
	countQ := s.db.NewSelect().Model((*models.CredentialProvider)(nil))
	listQ := s.db.NewSelect().Model((*models.CredentialProvider)(nil))

	if t := normalizeLower(q.Type); t != "" {
		if !IsValidCredentialProviderType(t) {
			return nil, 0, fmt.Errorf("invalid provider type %q", t)
		}
		countQ = countQ.Where("type = ?", t)
		listQ = listQ.Where("type = ?", t)
	}
	if q.Enabled != nil {
		countQ = countQ.Where("enabled = ?", *q.Enabled)
		listQ = listQ.Where("enabled = ?", *q.Enabled)
	}
	if q.Search != "" {
		countQ = countQ.Where("name LIKE ?", "%"+q.Search+"%")
		listQ = listQ.Where("name LIKE ?", "%"+q.Search+"%")
	}

	total, err := countQ.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count credential providers: %w", err)
	}

	listQ = listQ.OrderExpr("created_at DESC")
	if q.Limit > 0 {
		listQ = listQ.Limit(q.Limit)
	}
	if q.Skip > 0 {
		listQ = listQ.Offset(q.Skip)
	}

	var items []models.CredentialProvider
	err = listQ.Scan(ctx, &items)
	if err != nil {
		return nil, 0, fmt.Errorf("list credential providers: %w", err)
	}

	out := make([]CredentialProviderRecord, 0, len(items))
	for _, p := range items {
		rec := pgCredentialProviderToRecord(&p)
		rec.ConfigConfigured = p.ConfigEncrypted != ""
		out = append(out, *rec)
	}
	return out, int64(total), nil
}

// DefaultInternalProviderName is the well-known name of the seeded system
// provider that stores secrets encrypted inside Alga.
const DefaultInternalProviderName = "Alga Internal"

// SeedDefaultInternalProvider ensures the built-in "Alga Internal" provider
// exists, is marked system, enabled, and typed internal. It is idempotent.
func (s *pgCredentialProviderStore) SeedDefaultInternalProvider(ctx context.Context) error {
	exists, err := s.db.NewSelect().Model((*models.CredentialProvider)(nil)).
		Where("system = ?", true).
		Where("type = ?", CredentialProviderTypeInternal).
		Exists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check default credential provider: %w", err)
	}
	if exists {
		return nil
	}
	// Name uniqueness: if a non-system provider already occupies the default
	// name, rename it aside so the seed can claim the canonical name.
	if clash, _ := s.GetProviderByName(ctx, DefaultInternalProviderName); clash != nil {
		_, _ = s.db.NewUpdate().Model((*models.CredentialProvider)(nil)).
			Set("name = ?", DefaultInternalProviderName+" (legacy)").
			Set("updated_at = ?", time.Now().UTC()).
			Where("id = ?", clash.ID).
			Exec(ctx)
	}
	now := time.Now().UTC()
	m := &models.CredentialProvider{
		Name:    DefaultInternalProviderName,
		Type:    CredentialProviderTypeInternal,
		Enabled: true,
		System:  true,
	}
	m.ID = models.NewUUID()
	m.CreatedAt = now
	m.UpdatedAt = now

	_, err = s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to seed default credential provider: %w", err)
	}
	return nil
}

// encryptProviderConfig serializes config to JSON and encrypts the blob.
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

func pgCredentialProviderToRecord(p *models.CredentialProvider) *CredentialProviderRecord {
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
