package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
)

type SAMLProviderRecord struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	IDPEntityID    string    `json:"idp_entity_id"`
	MetadataXML    string    `json:"-"`
	MetadataXMLSet bool      `json:"-"`
	EmailAttribute string    `json:"email_attribute"`
	Enabled        bool      `json:"enabled"`
	EnabledSet     bool      `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type SAMLProviderQuery struct {
	Enabled *bool
	Search  string
	Limit   int
	Skip    int
}

type SAMLProviderStore interface {
	CreateProvider(ctx context.Context, record *SAMLProviderRecord) (*SAMLProviderRecord, error)
	UpdateProvider(ctx context.Context, id uuid.UUID, patch *SAMLProviderRecord) (*SAMLProviderRecord, error)
	DeleteProvider(ctx context.Context, id uuid.UUID) error
	GetProvider(ctx context.Context, id uuid.UUID) (*SAMLProviderRecord, error)
	ListProviders(ctx context.Context, q SAMLProviderQuery) ([]SAMLProviderRecord, int64, error)
	ListEnabledProviders(ctx context.Context) ([]SAMLProviderRecord, error)
}

type SAMLIdentityRecord struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	ProviderID uuid.UUID `json:"provider_id"`
	Subject    string    `json:"subject"`
	Email      string    `json:"email"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type SAMLIdentityStore interface {
	CreateLink(ctx context.Context, record *SAMLIdentityRecord) (*SAMLIdentityRecord, error)
	GetByProviderSubject(ctx context.Context, providerID uuid.UUID, subject string) (*SAMLIdentityRecord, error)
}

type pgSAMLProviderStore struct {
	pgStoreBase
}

func newPGSAMLProviderStore(db *bun.DB) SAMLProviderStore {
	return &pgSAMLProviderStore{pgStoreBase{db: db}}
}

func (s *pgSAMLProviderStore) CreateProvider(ctx context.Context, record *SAMLProviderRecord) (*SAMLProviderRecord, error) {
	if record == nil {
		return nil, errors.New("nil record")
	}
	ctx, cancel := pgctx(ctx)
	defer cancel()
	now := time.Now().UTC()
	item := &models.SAMLProvider{
		BaseModel:      models.BaseModel{ID: models.NewUUID(), CreatedAt: now, UpdatedAt: now},
		Name:           record.Name,
		IDPEntityID:    record.IDPEntityID,
		MetadataXML:    record.MetadataXML,
		EmailAttribute: record.EmailAttribute,
		Enabled:        record.Enabled,
	}
	if _, err := s.db.NewInsert().Model(item).Exec(ctx); err != nil {
		return nil, fmt.Errorf("insert saml provider: %w", err)
	}
	return pgSAMLProviderToRecord(item), nil
}

func (s *pgSAMLProviderStore) UpdateProvider(ctx context.Context, id uuid.UUID, patch *SAMLProviderRecord) (*SAMLProviderRecord, error) {
	if patch == nil {
		return nil, errors.New("nil patch")
	}
	ctx, cancel := pgctx(ctx)
	defer cancel()
	q := s.db.NewUpdate().Model((*models.SAMLProvider)(nil)).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id)
	if patch.Name != "" {
		q = q.Set("name = ?", patch.Name)
	}
	if patch.MetadataXMLSet {
		q = q.Set("metadata_xml = ?", patch.MetadataXML).
			Set("idp_entity_id = ?", patch.IDPEntityID)
	}
	if patch.EmailAttribute != "" {
		q = q.Set("email_attribute = ?", patch.EmailAttribute)
	}
	if patch.EnabledSet {
		q = q.Set("enabled = ?", patch.Enabled)
	}
	res, err := q.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("update saml provider: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("read saml provider update count: %w", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("saml provider %w", ErrNotFound)
	}
	item := new(models.SAMLProvider)
	if err := s.db.NewSelect().Model(item).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, fmt.Errorf("reload saml provider: %w", err)
	}
	return pgSAMLProviderToRecord(item), nil
}

func (s *pgSAMLProviderStore) DeleteProvider(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()
	res, err := s.db.NewDelete().Model((*models.SAMLProvider)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete saml provider: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("read saml provider delete count: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("saml provider %w", ErrNotFound)
	}
	return nil
}

func (s *pgSAMLProviderStore) GetProvider(ctx context.Context, id uuid.UUID) (*SAMLProviderRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()
	item := new(models.SAMLProvider)
	if err := s.db.NewSelect().Model(item).Where("id = ?", id).Scan(ctx); err != nil {
		return handleQueryErr[*SAMLProviderRecord](err, "saml provider")
	}
	return pgSAMLProviderToRecord(item), nil
}

func (s *pgSAMLProviderStore) ListProviders(ctx context.Context, q SAMLProviderQuery) ([]SAMLProviderRecord, int64, error) {
	ctx, cancel := pgctxLong(ctx)
	defer cancel()
	countQ := s.db.NewSelect().Model((*models.SAMLProvider)(nil))
	listQ := s.db.NewSelect().Model((*models.SAMLProvider)(nil))
	if q.Enabled != nil {
		countQ = countQ.Where("enabled = ?", *q.Enabled)
		listQ = listQ.Where("enabled = ?", *q.Enabled)
	}
	if q.Search != "" {
		pattern := "%" + q.Search + "%"
		countQ = countQ.Where("name ILIKE ?", pattern)
		listQ = listQ.Where("name ILIKE ?", pattern)
	}
	total, err := countQ.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count saml providers: %w", err)
	}
	listQ = listQ.Order("created_at DESC")
	if q.Limit > 0 {
		listQ = listQ.Limit(q.Limit)
	}
	if q.Skip > 0 {
		listQ = listQ.Offset(q.Skip)
	}
	var items []models.SAMLProvider
	if err := listQ.Scan(ctx, &items); err != nil {
		return nil, 0, fmt.Errorf("list saml providers: %w", err)
	}
	out := make([]SAMLProviderRecord, 0, len(items))
	for i := range items {
		out = append(out, *pgSAMLProviderToRecord(&items[i]))
	}
	return out, int64(total), nil
}

func (s *pgSAMLProviderStore) ListEnabledProviders(ctx context.Context) ([]SAMLProviderRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()
	var items []models.SAMLProvider
	if err := s.db.NewSelect().Model(&items).
		Where("enabled = ?", true).
		Where("metadata_xml <> ''").
		Order("name ASC").
		Scan(ctx); err != nil {
		return nil, fmt.Errorf("list enabled saml providers: %w", err)
	}
	out := make([]SAMLProviderRecord, 0, len(items))
	for i := range items {
		out = append(out, *pgSAMLProviderToRecord(&items[i]))
	}
	return out, nil
}

func pgSAMLProviderToRecord(item *models.SAMLProvider) *SAMLProviderRecord {
	return &SAMLProviderRecord{
		ID:             item.ID,
		Name:           item.Name,
		IDPEntityID:    item.IDPEntityID,
		MetadataXML:    item.MetadataXML,
		EmailAttribute: item.EmailAttribute,
		Enabled:        item.Enabled,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

type pgSAMLIdentityStore struct {
	pgStoreBase
}

func newPGSAMLIdentityStore(db *bun.DB) SAMLIdentityStore {
	return &pgSAMLIdentityStore{pgStoreBase{db: db}}
}

func (s *pgSAMLIdentityStore) CreateLink(ctx context.Context, record *SAMLIdentityRecord) (*SAMLIdentityRecord, error) {
	if record == nil {
		return nil, errors.New("nil record")
	}
	ctx, cancel := pgctx(ctx)
	defer cancel()
	now := time.Now().UTC()
	item := &models.SAMLIdentity{
		BaseModel:  models.BaseModel{ID: models.NewUUID(), CreatedAt: now, UpdatedAt: now},
		UserID:     record.UserID,
		ProviderID: record.ProviderID,
		Subject:    record.Subject,
		Email:      record.Email,
	}
	if _, err := s.db.NewInsert().Model(item).Exec(ctx); err != nil {
		return nil, fmt.Errorf("insert saml identity: %w", err)
	}
	return pgSAMLIdentityToRecord(item), nil
}

func (s *pgSAMLIdentityStore) GetByProviderSubject(ctx context.Context, providerID uuid.UUID, subject string) (*SAMLIdentityRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()
	item := new(models.SAMLIdentity)
	err := s.db.NewSelect().Model(item).
		Where("provider_id = ?", providerID).
		Where("subject = ?", subject).
		Scan(ctx)
	if err != nil {
		return handleQueryErr[*SAMLIdentityRecord](err, "saml identity")
	}
	return pgSAMLIdentityToRecord(item), nil
}

func pgSAMLIdentityToRecord(item *models.SAMLIdentity) *SAMLIdentityRecord {
	return &SAMLIdentityRecord{
		ID:         item.ID,
		UserID:     item.UserID,
		ProviderID: item.ProviderID,
		Subject:    item.Subject,
		Email:      item.Email,
		CreatedAt:  item.CreatedAt,
		UpdatedAt:  item.UpdatedAt,
	}
}
