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

// SharedSecretRecord is the wire/persistence shape for a shared secret.
// ValueEncrypted holds the internal-provider ciphertext and MUST NOT be
// serialized (json:"-"); load it from getters when resolution is needed.
type SharedSecretRecord struct {
	ID              uuid.UUID   `json:"id"`
	ProviderID      uuid.UUID   `json:"provider_id"`
	Name            string      `json:"name"`
	SecretID        string      `json:"secret_id"`
	Description     string      `json:"description"`
	RemoteRef       string      `json:"remote_ref"`
	ValueConfigured bool        `json:"value_configured"`
	AllowedAgentIDs []uuid.UUID `json:"allowed_agent_ids,omitempty"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`

	ValueEncrypted string                    `json:"-"`
	Provider       *CredentialProviderRecord `json:"provider,omitempty"`
}

// AllowedAgentIDsSet reports whether any agent is permitted to fetch this
// secret. When false, no agent may fetch it until at least one is assigned.
func (r *SharedSecretRecord) AllowedAgentIDsSet() bool {
	return len(r.AllowedAgentIDs) > 0
}

// AgentAllowed reports whether agentID may fetch this secret. A secret is always
// restricted to its allow-list; an empty list denies every agent.
func (r *SharedSecretRecord) AgentAllowed(agentID uuid.UUID) bool {
	for _, id := range r.AllowedAgentIDs {
		if id == agentID {
			return true
		}
	}
	return false
}

type SharedSecretQuery struct {
	ProviderID *uuid.UUID
	Search     string
	Limit      int
	Skip       int
}

type SharedSecretStore interface {
	CreateSecret(ctx context.Context, record *SharedSecretRecord, value string) (*SharedSecretRecord, error)
	UpdateSecret(ctx context.Context, id uuid.UUID, patch *SharedSecretUpdate, value *string) (*SharedSecretRecord, error)
	DeleteSecret(ctx context.Context, id uuid.UUID) error
	GetSecretByID(ctx context.Context, id uuid.UUID) (*SharedSecretRecord, error)
	GetSecretBySecretID(ctx context.Context, secretID string) (*SharedSecretRecord, error)
	ListSecrets(ctx context.Context, q SharedSecretQuery) ([]SharedSecretRecord, int64, error)
}

// SharedSecretUpdate is the patch shape for update. Pointer/nil-sentinel fields
// distinguish "leave unchanged" from "set to empty".
type SharedSecretUpdate struct {
	Name            *string
	Description     *string
	RemoteRef       *string
	AllowedAgentIDs *[]uuid.UUID
}

type pgSharedSecretStore struct {
	pgStoreBase
}

func newPGSharedSecretStore(db *bun.DB) SharedSecretStore {
	return &pgSharedSecretStore{pgStoreBase{db: db}}
}

func (s *pgSharedSecretStore) CreateSecret(ctx context.Context, record *SharedSecretRecord, value string) (*SharedSecretRecord, error) {
	if record == nil {
		return nil, errors.New("nil record")
	}
	if record.ProviderID == uuid.Nil {
		return nil, errors.New("provider_id is required")
	}
	if record.Name == "" {
		return nil, errors.New("name is required")
	}
	sid := normalizeLower(record.SecretID)
	if sid == "" {
		return nil, errors.New("secret_id is required")
	}
	record.SecretID = sid

	enc, err := encryptSecret(value)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now
	record.ValueEncrypted = enc
	record.ValueConfigured = value != ""
	allowed := record.AllowedAgentIDs
	if allowed == nil {
		allowed = []uuid.UUID{}
	}

	m := &models.SharedSecret{
		ID:              models.NewUUID(),
		ProviderID:      record.ProviderID,
		Name:            record.Name,
		SecretID:        sid,
		Description:     record.Description,
		RemoteRef:       record.RemoteRef,
		ValueEncrypted:  enc,
		ValueConfigured: value != "",
		AllowedAgentIDs: allowed,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	_, err = s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert shared secret: %w", err)
	}
	out := pgSharedSecretToRecord(m)
	return out, nil
}

func (s *pgSharedSecretStore) UpdateSecret(ctx context.Context, id uuid.UUID, patch *SharedSecretUpdate, value *string) (*SharedSecretRecord, error) {
	if patch == nil {
		return nil, errors.New("nil patch")
	}
	upd := s.db.NewUpdate().Model((*models.SharedSecret)(nil)).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id)

	if patch.Name != nil && *patch.Name != "" {
		upd = upd.Set("name = ?", *patch.Name)
	}
	if patch.Description != nil {
		upd = upd.Set("description = ?", *patch.Description)
	}
	if patch.RemoteRef != nil {
		upd = upd.Set("remote_ref = ?", *patch.RemoteRef)
	}
	if patch.AllowedAgentIDs != nil {
		upd = upd.Set("allowed_agent_ids = ?", *patch.AllowedAgentIDs)
	}
	if value != nil {
		enc, err := encryptSecret(*value)
		if err != nil {
			return nil, err
		}
		upd = upd.Set("value_encrypted = ?", enc)
		upd = upd.Set("value_configured = ?", *value != "")
	}

	res, err := upd.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update shared secret: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to update shared secret: %w", err)
	}
	if n == 0 {
		return nil, ErrSharedSecretNotFound
	}

	var saved models.SharedSecret
	err = s.db.NewSelect().Model(&saved).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to reload shared secret: %w", err)
	}
	return pgSharedSecretToRecord(&saved), nil
}

func (s *pgSharedSecretStore) DeleteSecret(ctx context.Context, id uuid.UUID) error {
	res, err := s.db.NewDelete().Model((*models.SharedSecret)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete shared secret: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to delete shared secret: %w", err)
	}
	if n == 0 {
		return ErrSharedSecretNotFound
	}
	return nil
}

func (s *pgSharedSecretStore) GetSecretByID(ctx context.Context, id uuid.UUID) (*SharedSecretRecord, error) {
	var ss models.SharedSecret
	err := s.db.NewSelect().Model(&ss).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*SharedSecretRecord](err, "shared secret")
	}
	return pgSharedSecretToRecord(&ss), nil
}

func (s *pgSharedSecretStore) GetSecretBySecretID(ctx context.Context, secretID string) (*SharedSecretRecord, error) {
	sid := normalizeLower(secretID)
	var ss models.SharedSecret
	err := s.db.NewSelect().Model(&ss).Where("secret_id = ?", sid).Scan(ctx)
	if err != nil {
		return handleQueryErr[*SharedSecretRecord](err, "shared secret")
	}
	return pgSharedSecretToRecord(&ss), nil
}

func (s *pgSharedSecretStore) ListSecrets(ctx context.Context, q SharedSecretQuery) ([]SharedSecretRecord, int64, error) {
	countQ := s.db.NewSelect().Model((*models.SharedSecret)(nil))
	listQ := s.db.NewSelect().Model((*models.SharedSecret)(nil))

	if q.ProviderID != nil {
		countQ = countQ.Where("provider_id = ?", *q.ProviderID)
		listQ = listQ.Where("provider_id = ?", *q.ProviderID)
	}
	if q.Search != "" {
		countQ = countQ.Where("(name LIKE ? OR secret_id LIKE ?)", "%"+q.Search+"%", "%"+q.Search+"%")
		listQ = listQ.Where("(name LIKE ? OR secret_id LIKE ?)", "%"+q.Search+"%", "%"+q.Search+"%")
	}

	total, err := countQ.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count shared secrets: %w", err)
	}

	listQ = listQ.OrderExpr("created_at DESC")
	if q.Limit > 0 {
		listQ = listQ.Limit(q.Limit)
	}
	if q.Skip > 0 {
		listQ = listQ.Offset(q.Skip)
	}

	var items []models.SharedSecret
	err = listQ.Scan(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list shared secrets: %w", err)
	}

	out := make([]SharedSecretRecord, 0, len(items))
	for _, ss := range items {
		out = append(out, *pgSharedSecretToRecord(&ss))
	}
	return out, int64(total), nil
}

func pgSharedSecretToRecord(ss *models.SharedSecret) *SharedSecretRecord {
	allowed := ss.AllowedAgentIDs
	if allowed == nil {
		allowed = []uuid.UUID{}
	}
	return &SharedSecretRecord{
		ID:              ss.ID,
		ProviderID:      ss.ProviderID,
		Name:            ss.Name,
		SecretID:        ss.SecretID,
		Description:     ss.Description,
		RemoteRef:       ss.RemoteRef,
		ValueEncrypted:  ss.ValueEncrypted,
		ValueConfigured: ss.ValueConfigured,
		AllowedAgentIDs: allowed,
		CreatedAt:       ss.CreatedAt,
		UpdatedAt:       ss.UpdatedAt,
	}
}
