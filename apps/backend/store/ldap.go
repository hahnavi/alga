package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
)

type LDAPProviderRecord struct {
	ID                    uuid.UUID `json:"id"`
	Name                  string    `json:"name"`
	URL                   string    `json:"url"`
	BaseDN                string    `json:"base_dn"`
	BindDN                string    `json:"bind_dn"`
	BindPasswordEncrypted string    `json:"-"`
	LoginAttribute        string    `json:"login_attribute"`
	IDAttribute           string    `json:"id_attribute"`
	EmailAttribute        string    `json:"email_attribute"`
	StartTLS              bool      `json:"start_tls"`
	Enabled               bool      `json:"enabled"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type LDAPProviderStore interface {
	Create(ctx context.Context, p *LDAPProviderRecord) (*LDAPProviderRecord, error)
	Update(ctx context.Context, p *LDAPProviderRecord) (*LDAPProviderRecord, error)
	Get(ctx context.Context, id uuid.UUID) (*LDAPProviderRecord, error)
	List(ctx context.Context, enabledOnly bool) ([]LDAPProviderRecord, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type LDAPIdentityStore interface {
	Get(ctx context.Context, providerID uuid.UUID, subject string) (*LDAPIdentityRecord, error)
	Create(ctx context.Context, providerID, userID uuid.UUID, subject string) error
}

type LDAPIdentityRecord struct {
	UserID uuid.UUID
}

type pgLDAPProviderStore struct{ pgStoreBase }
type pgLDAPIdentityStore struct{ pgStoreBase }

func newPGLDAPProviderStore(db *bun.DB) LDAPProviderStore {
	return &pgLDAPProviderStore{pgStoreBase{db: db}}
}
func newPGLDAPIdentityStore(db *bun.DB) LDAPIdentityStore {
	return &pgLDAPIdentityStore{pgStoreBase{db: db}}
}

func ldapRecord(m *models.LDAPProvider) *LDAPProviderRecord {
	return &LDAPProviderRecord{ID: m.ID, Name: m.Name, URL: m.URL, BaseDN: m.BaseDN, BindDN: m.BindDN,
		BindPasswordEncrypted: m.BindPasswordEncrypted, LoginAttribute: m.LoginAttribute, IDAttribute: m.IDAttribute,
		EmailAttribute: m.EmailAttribute, StartTLS: m.StartTLS, Enabled: m.Enabled, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt}
}

func (s *pgLDAPProviderStore) Create(ctx context.Context, p *LDAPProviderRecord) (*LDAPProviderRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()
	now := time.Now().UTC()
	m := &models.LDAPProvider{BaseModel: models.BaseModel{ID: models.NewUUID(), CreatedAt: now, UpdatedAt: now},
		Name: p.Name, URL: p.URL, BaseDN: p.BaseDN, BindDN: p.BindDN, BindPasswordEncrypted: p.BindPasswordEncrypted,
		LoginAttribute: p.LoginAttribute, IDAttribute: p.IDAttribute, EmailAttribute: p.EmailAttribute,
		StartTLS: p.StartTLS, Enabled: p.Enabled}
	if _, err := s.db.NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, fmt.Errorf("create ldap provider: %w", err)
	}
	return ldapRecord(m), nil
}

func (s *pgLDAPProviderStore) Update(ctx context.Context, p *LDAPProviderRecord) (*LDAPProviderRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()
	res, err := s.db.NewUpdate().Model((*models.LDAPProvider)(nil)).
		Set("name = ?", p.Name).Set("url = ?", p.URL).Set("base_dn = ?", p.BaseDN).
		Set("bind_dn = ?", p.BindDN).Set("bind_password_encrypted = ?", p.BindPasswordEncrypted).
		Set("login_attribute = ?", p.LoginAttribute).Set("id_attribute = ?", p.IDAttribute).
		Set("email_attribute = ?", p.EmailAttribute).Set("start_tls = ?", p.StartTLS).
		Set("enabled = ?", p.Enabled).Set("updated_at = ?", time.Now().UTC()).Where("id = ?", p.ID).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("update ldap provider: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("ldap update count: %w", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("ldap provider %w", ErrNotFound)
	}
	return s.Get(ctx, p.ID)
}

func (s *pgLDAPProviderStore) Get(ctx context.Context, id uuid.UUID) (*LDAPProviderRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()
	m := new(models.LDAPProvider)
	if err := s.db.NewSelect().Model(m).Where("id = ?", id).Scan(ctx); err != nil {
		return handleQueryErr[*LDAPProviderRecord](err, "ldap provider")
	}
	return ldapRecord(m), nil
}

func (s *pgLDAPProviderStore) List(ctx context.Context, enabledOnly bool) ([]LDAPProviderRecord, error) {
	ctx, cancel := pgctxLong(ctx)
	defer cancel()
	var rows []models.LDAPProvider
	q := s.db.NewSelect().Model(&rows).Order("name ASC")
	if enabledOnly {
		q = q.Where("enabled = ?", true)
	}
	if err := q.Scan(ctx); err != nil {
		return nil, fmt.Errorf("list ldap providers: %w", err)
	}
	out := make([]LDAPProviderRecord, 0, len(rows))
	for i := range rows {
		out = append(out, *ldapRecord(&rows[i]))
	}
	return out, nil
}

func (s *pgLDAPProviderStore) Delete(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()
	res, err := s.db.NewDelete().Model((*models.LDAPProvider)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete ldap provider: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("ldap delete count: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("ldap provider %w", ErrNotFound)
	}
	return nil
}

func (s *pgLDAPIdentityStore) Get(ctx context.Context, providerID uuid.UUID, subject string) (*LDAPIdentityRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()
	m := new(models.LDAPIdentity)
	if err := s.db.NewSelect().Model(m).Where("provider_id = ?", providerID).Where("subject = ?", subject).Scan(ctx); err != nil {
		return handleQueryErr[*LDAPIdentityRecord](err, "ldap identity")
	}
	return &LDAPIdentityRecord{UserID: m.UserID}, nil
}

func (s *pgLDAPIdentityStore) Create(ctx context.Context, providerID, userID uuid.UUID, subject string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()
	now := time.Now().UTC()
	m := &models.LDAPIdentity{BaseModel: models.BaseModel{ID: models.NewUUID(), CreatedAt: now, UpdatedAt: now},
		ProviderID: providerID, UserID: userID, Subject: subject}
	if _, err := s.db.NewInsert().Model(m).Exec(ctx); err != nil {
		return fmt.Errorf("link ldap identity: %w", err)
	}
	return nil
}
