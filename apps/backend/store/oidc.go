package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	algacrypto "alga/crypto"
	"alga/ent"
	"alga/ent/oidcidentity"
	"alga/ent/oidcprovider"
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

func newPGOIDCProviderStore(client *ent.Client) OIDCProviderStore {
	return &pgOIDCProviderStore{pgStoreBase{client: client}}
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

	saved, err := s.client.OIDCProvider.Create().
		SetName(record.Name).
		SetIssuer(record.Issuer).
		SetClientID(record.ClientID).
		SetClientSecretEncrypted(encSecret).
		SetScopes(record.Scopes).
		SetEnabled(record.Enabled).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert oidc provider: %w", err)
	}
	out := pgOIDCProviderToRecord(saved)
	out.ClientSecretConfigured = clientSecret != ""
	return out, nil
}

func (s *pgOIDCProviderStore) UpdateProvider(ctx context.Context, id uuid.UUID, patch *OIDCProviderRecord, clientSecret *string) (*OIDCProviderRecord, error) {
	if patch == nil {
		return nil, errors.New("nil patch")
	}
	b := s.client.OIDCProvider.UpdateOneID(id).SetUpdatedAt(time.Now().UTC())
	if patch.Name != "" {
		b.SetName(patch.Name)
	}
	if patch.Issuer != "" {
		b.SetIssuer(patch.Issuer)
	}
	if patch.ClientID != "" {
		b.SetClientID(patch.ClientID)
	}
	if patch.Scopes != nil {
		b.SetScopes(patch.Scopes)
	}
	if patch.EnabledSet {
		b.SetEnabled(patch.Enabled)
	}
	if clientSecret != nil {
		enc, err := encryptSecret(*clientSecret)
		if err != nil {
			return nil, err
		}
		b.SetClientSecretEncrypted(enc)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errors.New("oidc provider not found")
		}
		return nil, fmt.Errorf("failed to update oidc provider: %w", err)
	}
	out := pgOIDCProviderToRecord(saved)
	if clientSecret != nil {
		out.ClientSecretConfigured = *clientSecret != ""
	} else {
		// Preserve the configured flag based on the stored value.
		existing, _ := s.client.OIDCProvider.Get(ctx, id)
		out.ClientSecretConfigured = existing != nil && existing.ClientSecretEncrypted != ""
	}
	return out, nil
}

func (s *pgOIDCProviderStore) DeleteProvider(ctx context.Context, id uuid.UUID) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer rollbackTx(tx)

	if _, err := tx.OIDCIdentity.Delete().Where(oidcidentity.ProviderIDEQ(id)).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete linked oidc identities: %w", err)
	}
	if err := tx.OIDCProvider.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("oidc provider not found: %w", ErrNotFound)
		}
		return fmt.Errorf("failed to delete oidc provider: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (s *pgOIDCProviderStore) GetProvider(ctx context.Context, id uuid.UUID) (*OIDCProviderRecord, error) {
	p, err := s.client.OIDCProvider.Get(ctx, id)
	if err != nil {
		return handleQueryErr[*OIDCProviderRecord](err, "oidc provider")
	}
	out := pgOIDCProviderToRecord(p)
	out.ClientSecretConfigured = p.ClientSecretEncrypted != ""
	return out, nil
}

func (s *pgOIDCProviderStore) GetProviderWithSecret(ctx context.Context, id uuid.UUID) (*OIDCProviderRecord, error) {
	p, err := s.client.OIDCProvider.Get(ctx, id)
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
	query := s.client.OIDCProvider.Query()
	if q.Enabled != nil {
		query = query.Where(oidcprovider.EnabledEQ(*q.Enabled))
	}
	if q.Search != "" {
		query = query.Where(oidcprovider.NameContains(q.Search))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count oidc providers: %w", err)
	}
	query = query.Order(ent.Desc(oidcprovider.FieldCreatedAt))
	if q.Limit > 0 {
		query = query.Limit(q.Limit)
	}
	if q.Skip > 0 {
		query = query.Offset(q.Skip)
	}
	items, err := query.All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list oidc providers: %w", err)
	}
	out := make([]OIDCProviderRecord, 0, len(items))
	for _, p := range items {
		rec := pgOIDCProviderToRecord(p)
		rec.ClientSecretConfigured = p.ClientSecretEncrypted != ""
		out = append(out, *rec)
	}
	return out, int64(total), nil
}

func (s *pgOIDCProviderStore) ListEnabledProviders(ctx context.Context) ([]OIDCProviderRecord, error) {
	items, err := s.client.OIDCProvider.Query().
		Where(oidcprovider.EnabledEQ(true)).
		Order(ent.Asc(oidcprovider.FieldName)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled oidc providers: %w", err)
	}
	out := make([]OIDCProviderRecord, 0, len(items))
	for _, p := range items {
		rec := pgOIDCProviderToRecord(p)
		rec.ClientSecretConfigured = p.ClientSecretEncrypted != ""
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

func pgOIDCProviderToRecord(p *ent.OIDCProvider) *OIDCProviderRecord {
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

func newPGOIDCIdentityStore(client *ent.Client) OIDCIdentityStore {
	return &pgOIDCIdentityStore{pgStoreBase{client: client}}
}

func (s *pgOIDCIdentityStore) CreateLink(ctx context.Context, record *OIDCIdentityRecord) (*OIDCIdentityRecord, error) {
	if record == nil {
		return nil, errors.New("nil record")
	}
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now

	saved, err := s.client.OIDCIdentity.Create().
		SetUserID(record.UserID).
		SetProviderID(record.ProviderID).
		SetSubject(record.Subject).
		SetIssuer(record.Issuer).
		SetEmail(record.Email).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert oidc identity: %w", err)
	}
	return pgOIDCIdentityToRecord(saved), nil
}

func (s *pgOIDCIdentityStore) GetByProviderSubject(ctx context.Context, providerID uuid.UUID, subject string) (*OIDCIdentityRecord, error) {
	item, err := s.client.OIDCIdentity.Query().
		Where(oidcidentity.ProviderIDEQ(providerID), oidcidentity.SubjectEQ(subject)).
		Only(ctx)
	if err != nil {
		return handleQueryErr[*OIDCIdentityRecord](err, "oidc identity")
	}
	return pgOIDCIdentityToRecord(item), nil
}

func (s *pgOIDCIdentityStore) ListByUser(ctx context.Context, userID uuid.UUID) ([]OIDCIdentityRecord, error) {
	items, err := s.client.OIDCIdentity.Query().
		Where(oidcidentity.UserIDEQ(userID)).
		Order(ent.Desc(oidcidentity.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list oidc identities: %w", err)
	}
	out := make([]OIDCIdentityRecord, 0, len(items))
	for _, i := range items {
		out = append(out, *pgOIDCIdentityToRecord(i))
	}
	return out, nil
}

func (s *pgOIDCIdentityStore) Delete(ctx context.Context, id uuid.UUID) error {
	err := s.client.OIDCIdentity.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return errors.New("oidc identity not found")
		}
		return fmt.Errorf("failed to delete oidc identity: %w", err)
	}
	return nil
}

func pgOIDCIdentityToRecord(i *ent.OIDCIdentity) *OIDCIdentityRecord {
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
