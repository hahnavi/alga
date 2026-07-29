package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	algacrypto "alga/crypto"
	"alga/db/models"
)

type OIDCProviderRecord struct {
	ID                     uuid.UUID `json:"id"`
	Name                   string    `json:"name"`
	Issuer                 string    `json:"issuer"`
	ClientID               string    `json:"client_id"`
	ClientSecretConfigured bool      `json:"client_secret_configured"`
	Scopes                 []string  `json:"scopes"`
	Enabled                bool      `json:"enabled"`
	EnabledSet             bool      `json:"-"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`

	// clientSecret holds the decrypted plaintext for the authorize/exchange flow.
	// Never serialized; only populated by GetProviderWithSecret.
	clientSecret string `json:"-"`
}

// ClientSecret returns the decrypted secret. Only valid for records loaded via
// GetProviderWithSecret.
func (r *OIDCProviderRecord) ClientSecret() string { return r.clientSecret }

// SetClientSecret sets the decrypted client secret (used by mocks/tests).
func (r *OIDCProviderRecord) SetClientSecret(s string) { r.clientSecret = s }

type OIDCProviderQuery struct {
	Enabled *bool
	Search  string
	Limit   int
	Skip    int
}

type OIDCProviderStore interface {
	CreateProvider(ctx context.Context, record *OIDCProviderRecord, clientSecret string) (*OIDCProviderRecord, error)
	UpdateProvider(ctx context.Context, id uuid.UUID, patch *OIDCProviderRecord, clientSecret *string) (*OIDCProviderRecord, error)
	DeleteProvider(ctx context.Context, id uuid.UUID) error
	GetProvider(ctx context.Context, id uuid.UUID) (*OIDCProviderRecord, error)
	GetProviderWithSecret(ctx context.Context, id uuid.UUID) (*OIDCProviderRecord, error)
	ListProviders(ctx context.Context, q OIDCProviderQuery) ([]OIDCProviderRecord, int64, error)
	ListEnabledProviders(ctx context.Context) ([]OIDCProviderRecord, error)
}

type OIDCIdentityRecord struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	ProviderID uuid.UUID `json:"provider_id"`
	Subject    string    `json:"subject"`
	Issuer     string    `json:"issuer"`
	Email      string    `json:"email"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type OIDCIdentityStore interface {
	CreateLink(ctx context.Context, record *OIDCIdentityRecord) (*OIDCIdentityRecord, error)
	GetByProviderSubject(ctx context.Context, providerID uuid.UUID, subject string) (*OIDCIdentityRecord, error)
	ListByUser(ctx context.Context, userID uuid.UUID) ([]OIDCIdentityRecord, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type pgOIDCProviderStore struct {
	pgStoreBase
}

func newPGOIDCProviderStore(db *bun.DB) OIDCProviderStore {
	return &pgOIDCProviderStore{pgStoreBase{db: db}}
}

func (s *pgOIDCProviderStore) CreateProvider(ctx context.Context, record *OIDCProviderRecord, clientSecret string) (*OIDCProviderRecord, error) {
	if record == nil {
		return nil, errors.New("nil record")
	}
	if record.Scopes == nil {
		record.Scopes = []string{"openid", "email", "profile"}
	}
	encSecret, err := encryptSecret(clientSecret)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now

	m := &models.OIDCProvider{
		BaseModel:             models.BaseModel{ID: models.NewUUID(), CreatedAt: now, UpdatedAt: now},
		Name:                  record.Name,
		Issuer:                record.Issuer,
		ClientID:              record.ClientID,
		ClientSecretEncrypted: encSecret,
		Scopes:                record.Scopes,
		Enabled:               record.Enabled,
	}

	_, err = s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert oidc provider: %w", err)
	}
	out := pgOIDCProviderToRecord(m)
	out.ClientSecretConfigured = clientSecret != ""
	return out, nil
}

func (s *pgOIDCProviderStore) UpdateProvider(ctx context.Context, id uuid.UUID, patch *OIDCProviderRecord, clientSecret *string) (*OIDCProviderRecord, error) {
	if patch == nil {
		return nil, errors.New("nil patch")
	}
	now := time.Now().UTC()
	q := s.db.NewUpdate().Model((*models.OIDCProvider)(nil)).
		Set("updated_at = ?", now).
		Where("id = ?", id)

	if patch.Name != "" {
		q = q.Set("name = ?", patch.Name)
	}
	if patch.Issuer != "" {
		q = q.Set("issuer = ?", patch.Issuer)
	}
	if patch.ClientID != "" {
		q = q.Set("client_id = ?", patch.ClientID)
	}
	if patch.Scopes != nil {
		q = q.Set("scopes = ?", patch.Scopes)
	}
	if patch.EnabledSet {
		q = q.Set("enabled = ?", patch.Enabled)
	}
	if clientSecret != nil {
		enc, err := encryptSecret(*clientSecret)
		if err != nil {
			return nil, err
		}
		q = q.Set("client_secret_encrypted = ?", enc)
	}

	res, err := q.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update oidc provider: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to update oidc provider: %w", err)
	}
	if n == 0 {
		return nil, errors.New("oidc provider not found")
	}

	// Reload the updated record.
	updated := new(models.OIDCProvider)
	if err := s.db.NewSelect().Model(updated).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to reload oidc provider: %w", err)
	}
	out := pgOIDCProviderToRecord(updated)
	if clientSecret != nil {
		out.ClientSecretConfigured = *clientSecret != ""
	} else {
		out.ClientSecretConfigured = updated.ClientSecretEncrypted != ""
	}
	return out, nil
}

func (s *pgOIDCProviderStore) DeleteProvider(ctx context.Context, id uuid.UUID) error {
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewDelete().Model((*models.OIDCIdentity)(nil)).
			Where("provider_id = ?", id).Exec(ctx); err != nil {
			return fmt.Errorf("failed to delete linked oidc identities: %w", err)
		}
		res, err := tx.NewDelete().Model((*models.OIDCProvider)(nil)).
			Where("id = ?", id).Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to delete oidc provider: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to delete oidc provider: %w", err)
		}
		if n == 0 {
			return fmt.Errorf("oidc provider not found: %w", ErrNotFound)
		}
		return nil
	})
}

func (s *pgOIDCProviderStore) GetProvider(ctx context.Context, id uuid.UUID) (*OIDCProviderRecord, error) {
	p := new(models.OIDCProvider)
	err := s.db.NewSelect().Model(p).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*OIDCProviderRecord](err, "oidc provider")
	}
	out := pgOIDCProviderToRecord(p)
	out.ClientSecretConfigured = p.ClientSecretEncrypted != ""
	return out, nil
}

func (s *pgOIDCProviderStore) GetProviderWithSecret(ctx context.Context, id uuid.UUID) (*OIDCProviderRecord, error) {
	p := new(models.OIDCProvider)
	err := s.db.NewSelect().Model(p).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*OIDCProviderRecord](err, "oidc provider")
	}
	out := pgOIDCProviderToRecord(p)
	plaintext, err := algacrypto.Default().DecryptString(p.ClientSecretEncrypted)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt oidc client secret: %w", err)
	}
	out.clientSecret = plaintext
	out.ClientSecretConfigured = p.ClientSecretEncrypted != ""
	return out, nil
}

func (s *pgOIDCProviderStore) ListProviders(ctx context.Context, q OIDCProviderQuery) ([]OIDCProviderRecord, int64, error) {
	countQ := s.db.NewSelect().Model((*models.OIDCProvider)(nil))
	listQ := s.db.NewSelect().Model((*models.OIDCProvider)(nil))

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
		return nil, 0, fmt.Errorf("count oidc providers: %w", err)
	}

	listQ = listQ.Order("created_at DESC")
	if q.Limit > 0 {
		listQ = listQ.Limit(q.Limit)
	}
	if q.Skip > 0 {
		listQ = listQ.Offset(q.Skip)
	}

	var items []models.OIDCProvider
	err = listQ.Scan(ctx, &items)
	if err != nil {
		return nil, 0, fmt.Errorf("list oidc providers: %w", err)
	}

	out := make([]OIDCProviderRecord, 0, len(items))
	for i := range items {
		rec := pgOIDCProviderToRecord(&items[i])
		rec.ClientSecretConfigured = items[i].ClientSecretEncrypted != ""
		out = append(out, *rec)
	}
	return out, int64(total), nil
}

func (s *pgOIDCProviderStore) ListEnabledProviders(ctx context.Context) ([]OIDCProviderRecord, error) {
	var items []models.OIDCProvider
	err := s.db.NewSelect().Model(&items).
		Where("enabled = ?", true).
		Order("name ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled oidc providers: %w", err)
	}
	out := make([]OIDCProviderRecord, 0, len(items))
	for i := range items {
		rec := pgOIDCProviderToRecord(&items[i])
		rec.ClientSecretConfigured = items[i].ClientSecretEncrypted != ""
		out = append(out, *rec)
	}
	return out, nil
}

func encryptSecret(secret string) (string, error) {
	if secret == "" {
		return "", nil
	}
	enc, err := algacrypto.Default().EncryptString(secret)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt oidc client secret: %w", err)
	}
	return enc, nil
}

func pgOIDCProviderToRecord(p *models.OIDCProvider) *OIDCProviderRecord {
	scopes := p.Scopes
	if scopes == nil {
		scopes = []string{"openid", "email", "profile"}
	}
	return &OIDCProviderRecord{
		ID:        p.ID,
		Name:      p.Name,
		Issuer:    p.Issuer,
		ClientID:  p.ClientID,
		Scopes:    scopes,
		Enabled:   p.Enabled,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

type pgOIDCIdentityStore struct {
	pgStoreBase
}

func newPGOIDCIdentityStore(db *bun.DB) OIDCIdentityStore {
	return &pgOIDCIdentityStore{pgStoreBase{db: db}}
}

func (s *pgOIDCIdentityStore) CreateLink(ctx context.Context, record *OIDCIdentityRecord) (*OIDCIdentityRecord, error) {
	if record == nil {
		return nil, errors.New("nil record")
	}
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now

	m := &models.OIDCIdentity{
		BaseModel:  models.BaseModel{ID: models.NewUUID(), CreatedAt: now, UpdatedAt: now},
		UserID:     record.UserID,
		ProviderID: record.ProviderID,
		Subject:    record.Subject,
		Issuer:     record.Issuer,
		Email:      record.Email,
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert oidc identity: %w", err)
	}
	return pgOIDCIdentityToRecord(m), nil
}

func (s *pgOIDCIdentityStore) GetByProviderSubject(ctx context.Context, providerID uuid.UUID, subject string) (*OIDCIdentityRecord, error) {
	item := new(models.OIDCIdentity)
	err := s.db.NewSelect().Model(item).
		Where("provider_id = ?", providerID).
		Where("subject = ?", subject).
		Scan(ctx)
	if err != nil {
		return handleQueryErr[*OIDCIdentityRecord](err, "oidc identity")
	}
	return pgOIDCIdentityToRecord(item), nil
}

func (s *pgOIDCIdentityStore) ListByUser(ctx context.Context, userID uuid.UUID) ([]OIDCIdentityRecord, error) {
	var items []models.OIDCIdentity
	err := s.db.NewSelect().Model(&items).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("list oidc identities: %w", err)
	}
	out := make([]OIDCIdentityRecord, 0, len(items))
	for i := range items {
		out = append(out, *pgOIDCIdentityToRecord(&items[i]))
	}
	return out, nil
}

func (s *pgOIDCIdentityStore) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := s.db.NewDelete().Model((*models.OIDCIdentity)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete oidc identity: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to delete oidc identity: %w", err)
	}
	if n == 0 {
		return errors.New("oidc identity not found")
	}
	return nil
}

func pgOIDCIdentityToRecord(i *models.OIDCIdentity) *OIDCIdentityRecord {
	return &OIDCIdentityRecord{
		ID:         i.ID,
		UserID:     i.UserID,
		ProviderID: i.ProviderID,
		Subject:    i.Subject,
		Issuer:     i.Issuer,
		Email:      i.Email,
		CreatedAt:  i.CreatedAt,
		UpdatedAt:  i.UpdatedAt,
	}
}
