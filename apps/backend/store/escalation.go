package store

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/escalationpolicy"
	entschema "alga/ent/schema"
)

// EscalationLevelRecord and EscalationTargetRecord are aliases for the JSONB
// inner types declared on the ent schema. The schema package owns the
// authoritative definition so Ent can generate the marshalling code; the store
// re-exports them so callers can stay in the `store` import path.
type (
	EscalationLevelRecord  = entschema.EscalationLevelRecord
	EscalationTargetRecord = entschema.EscalationTargetRecord
)

// EscalationPolicyRecord is the on-disk and wire-format representation of an
// escalation policy. The Levels slice is the single source of truth for the
// policy's escalation schedule; there is no separate levels/targets table.
//
// The store returns levels sorted by LevelNumber so callers can rely on
// ascending order without re-sorting. Insert/update paths accept any slice
// order and the next read normalizes it.
type EscalationPolicyRecord struct {
	ID          uuid.UUID               `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	RepeatCount int                     `json:"repeat_count"`
	Levels      []EscalationLevelRecord `json:"levels,omitempty"`
	CreatedAt   time.Time               `json:"created_at"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

type EscalationStore interface {
	CreatePolicy(ctx context.Context, record *EscalationPolicyRecord) (*EscalationPolicyRecord, error)
	GetPolicy(ctx context.Context, id uuid.UUID) (*EscalationPolicyRecord, error)
	UpdatePolicy(ctx context.Context, id uuid.UUID, record *EscalationPolicyRecord) (*EscalationPolicyRecord, error)
	DeletePolicy(ctx context.Context, id uuid.UUID) error
	ListPolicies(ctx context.Context, limit, skip int) ([]EscalationPolicyRecord, int64, error)
}

type pgEscalationStore struct {
	pgStoreBase
}

func newPGEscalationStore(client *ent.Client) EscalationStore {
	return &pgEscalationStore{pgStoreBase{client: client}}
}

// normalizeLevels clamps negative repeat counts to zero and returns a
// defensively-copied slice so a caller mutating the input cannot race a
// concurrent write that already captured it.
func normalizeLevels(in []EscalationLevelRecord) []EscalationLevelRecord {
	if in == nil {
		return []EscalationLevelRecord{}
	}
	out := make([]EscalationLevelRecord, len(in))
	copy(out, in)
	return out
}

func (s *pgEscalationStore) CreatePolicy(ctx context.Context, record *EscalationPolicyRecord) (*EscalationPolicyRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	if record.RepeatCount < 0 {
		record.RepeatCount = 0
	}
	levels := normalizeLevels(record.Levels)
	now := time.Now().UTC()

	saved, err := s.client.EscalationPolicy.Create().
		SetName(record.Name).
		SetDescription(record.Description).
		SetRepeatCount(record.RepeatCount).
		SetLevels(levels).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create escalation policy: %w", err)
	}

	record.ID = saved.ID
	record.Levels = levels
	record.CreatedAt = saved.CreatedAt
	record.UpdatedAt = saved.UpdatedAt
	return record, nil
}

func (s *pgEscalationStore) GetPolicy(ctx context.Context, id uuid.UUID) (*EscalationPolicyRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	policy, err := s.client.EscalationPolicy.Get(ctx, id)
	if err != nil {
		return handleQueryErr[*EscalationPolicyRecord](err, "escalation policy")
	}
	return escalationPolicyToRecord(policy), nil
}

func (s *pgEscalationStore) UpdatePolicy(ctx context.Context, id uuid.UUID, record *EscalationPolicyRecord) (*EscalationPolicyRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	if record.RepeatCount < 0 {
		record.RepeatCount = 0
	}
	levels := normalizeLevels(record.Levels)
	now := time.Now().UTC()

	updated, err := s.client.EscalationPolicy.UpdateOneID(id).
		SetName(record.Name).
		SetDescription(record.Description).
		SetRepeatCount(record.RepeatCount).
		SetLevels(levels).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("escalation policy not found: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("failed to update escalation policy: %w", err)
	}
	return escalationPolicyToRecord(updated), nil
}

func (s *pgEscalationStore) DeletePolicy(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	if err := s.client.EscalationPolicy.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("escalation policy not found: %w", ErrNotFound)
		}
		return fmt.Errorf("failed to delete escalation policy: %w", err)
	}
	return nil
}

func (s *pgEscalationStore) ListPolicies(ctx context.Context, limit, skip int) ([]EscalationPolicyRecord, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 20
	}
	limit = min(limit, 100)

	total, err := s.client.EscalationPolicy.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count policies: %w", err)
	}

	policies, err := s.client.EscalationPolicy.Query().
		Order(ent.Desc(escalationpolicy.FieldCreatedAt)).
		Limit(limit).
		Offset(skip).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list policies: %w", err)
	}

	records := make([]EscalationPolicyRecord, 0, len(policies))
	for _, p := range policies {
		records = append(records, *escalationPolicyToRecord(p))
	}
	return records, int64(total), nil
}

// escalationPolicyToRecord copies an ent EscalationPolicy into a store record.
// The levels slice is sorted by LevelNumber so callers can rely on ascending
// order without re-sorting; this is the only place we enforce the convention.
func escalationPolicyToRecord(p *ent.EscalationPolicy) *EscalationPolicyRecord {
	rec := &EscalationPolicyRecord{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		RepeatCount: p.RepeatCount,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
	if len(p.Levels) == 0 {
		rec.Levels = []EscalationLevelRecord{}
		return rec
	}
	levels := make([]EscalationLevelRecord, len(p.Levels))
	copy(levels, p.Levels)
	slices.SortFunc(levels, func(a, b EscalationLevelRecord) int { return cmp.Compare(a.LevelNumber, b.LevelNumber) })
	rec.Levels = levels
	return rec
}
