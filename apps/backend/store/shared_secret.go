package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/sharedsecret"
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

func newPGSharedSecretStore(client *ent.Client) SharedSecretStore {
	return &pgSharedSecretStore{pgStoreBase{client: client}}
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

	saved, err := s.client.SharedSecret.Create().
		SetProviderID(record.ProviderID).
		SetName(record.Name).
		SetSecretID(sid).
		SetDescription(record.Description).
		SetRemoteRef(record.RemoteRef).
		SetValueEncrypted(enc).
		SetValueConfigured(value != "").
		SetAllowedAgentIds(allowed).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert shared secret: %w", err)
	}
	out := pgSharedSecretToRecord(saved)
	return out, nil
}

func (s *pgSharedSecretStore) UpdateSecret(ctx context.Context, id uuid.UUID, patch *SharedSecretUpdate, value *string) (*SharedSecretRecord, error) {
	if patch == nil {
		return nil, errors.New("nil patch")
	}
	b := s.client.SharedSecret.UpdateOneID(id).SetUpdatedAt(time.Now().UTC())
	if patch.Name != nil && *patch.Name != "" {
		b.SetName(*patch.Name)
	}
	if patch.Description != nil {
		b.SetDescription(*patch.Description)
	}
	if patch.RemoteRef != nil {
		b.SetRemoteRef(*patch.RemoteRef)
	}
	if patch.AllowedAgentIDs != nil {
		b.SetAllowedAgentIds(*patch.AllowedAgentIDs)
	}
	if value != nil {
		enc, err := encryptSecret(*value)
		if err != nil {
			return nil, err
		}
		b.SetValueEncrypted(enc)
		b.SetValueConfigured(*value != "")
	}

	saved, err := b.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrSharedSecretNotFound
		}
		return nil, fmt.Errorf("failed to update shared secret: %w", err)
	}
	return pgSharedSecretToRecord(saved), nil
}

func (s *pgSharedSecretStore) DeleteSecret(ctx context.Context, id uuid.UUID) error {
	err := s.client.SharedSecret.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrSharedSecretNotFound
		}
		return fmt.Errorf("failed to delete shared secret: %w", err)
	}
	return nil
}

func (s *pgSharedSecretStore) GetSecretByID(ctx context.Context, id uuid.UUID) (*SharedSecretRecord, error) {
	ss, err := s.client.SharedSecret.Get(ctx, id)
	if err != nil {
		return handleQueryErr[*SharedSecretRecord](err, "shared secret")
	}
	return pgSharedSecretToRecord(ss), nil
}

func (s *pgSharedSecretStore) GetSecretBySecretID(ctx context.Context, secretID string) (*SharedSecretRecord, error) {
	sid := normalizeLower(secretID)
	ss, err := s.client.SharedSecret.Query().
		Where(sharedsecret.SecretIDEQ(sid)).
		Only(ctx)
	if err != nil {
		return handleQueryErr[*SharedSecretRecord](err, "shared secret")
	}
	return pgSharedSecretToRecord(ss), nil
}

func (s *pgSharedSecretStore) ListSecrets(ctx context.Context, q SharedSecretQuery) ([]SharedSecretRecord, int64, error) {
	query := s.client.SharedSecret.Query()
	if q.ProviderID != nil {
		query = query.Where(sharedsecret.ProviderIDEQ(*q.ProviderID))
	}
	if q.Search != "" {
		query = query.Where(sharedsecret.Or(
			sharedsecret.NameContains(q.Search),
			sharedsecret.SecretIDContains(q.Search),
		))
	}
	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count shared secrets: %w", err)
	}
	query = query.Order(ent.Desc(sharedsecret.FieldCreatedAt))
	if q.Limit > 0 {
		query = query.Limit(q.Limit)
	}
	if q.Skip > 0 {
		query = query.Offset(q.Skip)
	}
	items, err := query.All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list shared secrets: %w", err)
	}
	out := make([]SharedSecretRecord, 0, len(items))
	for _, ss := range items {
		out = append(out, *pgSharedSecretToRecord(ss))
	}
	return out, int64(total), nil
}

func pgSharedSecretToRecord(ss *ent.SharedSecret) *SharedSecretRecord {
	allowed := ss.AllowedAgentIds
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
