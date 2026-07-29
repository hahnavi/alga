package store

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
)

// EscalationLevelRecord and EscalationTargetRecord are aliases for the JSONB
// inner types declared in the models package. The models package owns the
// authoritative definition; the store re-exports them so callers can stay in
// the `store` import path.
type (
	EscalationLevelRecord  = models.EscalationLevelRecord
	EscalationTargetRecord = models.EscalationTargetRecord
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

func newPGEscalationStore(db *bun.DB) EscalationStore {
	return &pgEscalationStore{pgStoreBase{db: db}}
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

	m := &models.EscalationPolicy{
		BaseModel: models.BaseModel{
			ID:        models.NewUUID(),
			CreatedAt: now,
			UpdatedAt: now,
		},
		Name:        record.Name,
		Description: record.Description,
		RepeatCount: record.RepeatCount,
		Levels:      levels,
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create escalation policy: %w", err)
	}

	record.ID = m.ID
	record.Levels = levels
	record.CreatedAt = m.CreatedAt
	record.UpdatedAt = m.UpdatedAt
	return record, nil
}

func (s *pgEscalationStore) GetPolicy(ctx context.Context, id uuid.UUID) (*EscalationPolicyRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var policy models.EscalationPolicy
	err := s.db.NewSelect().Model(&policy).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*EscalationPolicyRecord](err, "escalation policy")
	}
	return escalationPolicyToRecord(&policy), nil
}

func (s *pgEscalationStore) UpdatePolicy(ctx context.Context, id uuid.UUID, record *EscalationPolicyRecord) (*EscalationPolicyRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	if record.RepeatCount < 0 {
		record.RepeatCount = 0
	}
	levels := normalizeLevels(record.Levels)
	now := time.Now().UTC()

	res, err := s.db.NewUpdate().Model((*models.EscalationPolicy)(nil)).
		Set("name = ?", record.Name).
		Set("description = ?", record.Description).
		Set("repeat_count = ?", record.RepeatCount).
		Set("levels = ?", levels).
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update escalation policy: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to update escalation policy: %w", err)
	}
	if n == 0 {
		return nil, fmt.Errorf("escalation policy not found: %w", ErrNotFound)
	}

	// Re-fetch to return the updated record
	var updated models.EscalationPolicy
	if err := s.db.NewSelect().Model(&updated).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to re-fetch updated escalation policy: %w", err)
	}
	return escalationPolicyToRecord(&updated), nil
}

func (s *pgEscalationStore) DeletePolicy(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	res, err := s.db.NewDelete().Model((*models.EscalationPolicy)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete escalation policy: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to delete escalation policy: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("escalation policy not found: %w", ErrNotFound)
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

	total, err := s.db.NewSelect().Model((*models.EscalationPolicy)(nil)).Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count policies: %w", err)
	}

	var policies []models.EscalationPolicy
	err = s.db.NewSelect().Model(&policies).
		Order("created_at DESC").
		Limit(limit).
		Offset(skip).
		Scan(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list policies: %w", err)
	}

	records := make([]EscalationPolicyRecord, 0, len(policies))
	for i := range policies {
		records = append(records, *escalationPolicyToRecord(&policies[i]))
	}
	return records, int64(total), nil
}

// escalationPolicyToRecord copies a models EscalationPolicy into a store record.
// The levels slice is sorted by LevelNumber so callers can rely on ascending
// order without re-sorting; this is the only place we enforce the convention.
func escalationPolicyToRecord(p *models.EscalationPolicy) *EscalationPolicyRecord {
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
