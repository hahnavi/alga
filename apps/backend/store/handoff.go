package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
)

type HandoffRecordRecord struct {
	ID                     uuid.UUID  `json:"id"`
	ScheduleID             uuid.UUID  `json:"schedule_id"`
	OutgoingUserID         *uuid.UUID `json:"outgoing_user_id,omitempty"`
	IncomingUserID         *uuid.UUID `json:"incoming_user_id,omitempty"`
	HandoffAt              time.Time  `json:"handoff_at"`
	Status                 string     `json:"status"`
	OutgoingNotes          string     `json:"outgoing_notes"`
	IncomingNotes          string     `json:"incoming_notes"`
	IncomingAcknowledgedAt *time.Time `json:"incoming_acknowledged_at,omitempty"`
	IncidentSummary        string     `json:"incident_summary"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type HandoffFilter struct {
	ScheduleID *uuid.UUID
	UserID     *uuid.UUID
	Status     string
}

type HandoffStore interface {
	Create(ctx context.Context, r *HandoffRecordRecord) (*HandoffRecordRecord, error)
	Get(ctx context.Context, id uuid.UUID) (*HandoffRecordRecord, error)
	List(ctx context.Context, filter HandoffFilter, limit, skip int) ([]*HandoffRecordRecord, int64, error)
	GetPendingForUser(ctx context.Context, userID uuid.UUID) ([]*HandoffRecordRecord, error)
	GetLatestForSchedule(ctx context.Context, scheduleID uuid.UUID) (*HandoffRecordRecord, error)
	UpdateOutgoingNotes(ctx context.Context, id uuid.UUID, notes string) error
	UpdateIncomingNotes(ctx context.Context, id uuid.UUID, notes string) error
	Acknowledge(ctx context.Context, id uuid.UUID) error
}

type pgHandoffStore struct {
	pgStoreBase
}

func newPGHandoffStore(db *bun.DB) HandoffStore {
	return &pgHandoffStore{pgStoreBase{db: db}}
}

func handoffFromModel(h *models.HandoffRecord) *HandoffRecordRecord {
	return &HandoffRecordRecord{
		ID:                     h.ID,
		ScheduleID:             h.ScheduleID,
		OutgoingUserID:         h.OutgoingUserID,
		IncomingUserID:         h.IncomingUserID,
		HandoffAt:              h.HandoffAt,
		Status:                 h.Status,
		OutgoingNotes:          h.OutgoingNotes,
		IncomingNotes:          h.IncomingNotes,
		IncomingAcknowledgedAt: h.IncomingAcknowledgedAt,
		IncidentSummary:        h.IncidentSummary,
		CreatedAt:              h.CreatedAt,
		UpdatedAt:              h.UpdatedAt,
	}
}

func (s *pgHandoffStore) Create(ctx context.Context, r *HandoffRecordRecord) (*HandoffRecordRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	m := &models.HandoffRecord{
		ScheduleID:             r.ScheduleID,
		OutgoingUserID:         r.OutgoingUserID,
		IncomingUserID:         r.IncomingUserID,
		HandoffAt:              r.HandoffAt,
		OutgoingNotes:          r.OutgoingNotes,
		IncomingNotes:          r.IncomingNotes,
		IncomingAcknowledgedAt: r.IncomingAcknowledgedAt,
		IncidentSummary:        r.IncidentSummary,
	}
	m.ID = models.NewUUID()
	m.CreatedAt = now
	m.UpdatedAt = now

	if r.Status != "" {
		m.Status = r.Status
	} else {
		m.Status = "pending"
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create handoff record: %w", err)
	}

	record := handoffFromModel(m)
	return record, nil
}

func (s *pgHandoffStore) Get(ctx context.Context, id uuid.UUID) (*HandoffRecordRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var h models.HandoffRecord
	err := s.db.NewSelect().Model(&h).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*HandoffRecordRecord](err, "handoff record")
	}

	return handoffFromModel(&h), nil
}

func (s *pgHandoffStore) List(ctx context.Context, filter HandoffFilter, limit, skip int) ([]*HandoffRecordRecord, int64, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	if limit <= 0 {
		limit = 20
	}
	limit = min(limit, 100)

	countQ := s.db.NewSelect().Model((*models.HandoffRecord)(nil))
	listQ := s.db.NewSelect().Model((*models.HandoffRecord)(nil))

	if filter.ScheduleID != nil {
		countQ = countQ.Where("schedule_id = ?", *filter.ScheduleID)
		listQ = listQ.Where("schedule_id = ?", *filter.ScheduleID)
	}
	if filter.UserID != nil {
		countQ = countQ.Where("(outgoing_user_id = ? OR incoming_user_id = ?)", *filter.UserID, *filter.UserID)
		listQ = listQ.Where("(outgoing_user_id = ? OR incoming_user_id = ?)", *filter.UserID, *filter.UserID)
	}
	if filter.Status != "" {
		countQ = countQ.Where("status = ?", filter.Status)
		listQ = listQ.Where("status = ?", filter.Status)
	}

	total, err := countQ.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count handoff records: %w", err)
	}

	var records []models.HandoffRecord
	err = listQ.
		OrderExpr("created_at DESC").
		Limit(limit).
		Offset(skip).
		Scan(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list handoff records: %w", err)
	}

	out := make([]*HandoffRecordRecord, 0, len(records))
	for _, r := range records {
		out = append(out, handoffFromModel(&r))
	}
	return out, int64(total), nil
}

func (s *pgHandoffStore) GetPendingForUser(ctx context.Context, userID uuid.UUID) ([]*HandoffRecordRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var records []models.HandoffRecord
	err := s.db.NewSelect().Model(&records).
		Where("status = ?", "pending").
		Where("(outgoing_user_id = ? OR incoming_user_id = ?)", userID, userID).
		OrderExpr("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending handoffs for user: %w", err)
	}

	out := make([]*HandoffRecordRecord, 0, len(records))
	for _, r := range records {
		out = append(out, handoffFromModel(&r))
	}
	return out, nil
}

func (s *pgHandoffStore) GetLatestForSchedule(ctx context.Context, scheduleID uuid.UUID) (*HandoffRecordRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var h models.HandoffRecord
	err := s.db.NewSelect().Model(&h).
		Where("schedule_id = ?", scheduleID).
		OrderExpr("handoff_at DESC").
		Limit(1).
		Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("handoff record not found: %w", ErrNotFound)
		}
		return nil, fmt.Errorf("failed to get latest handoff for schedule: %w", err)
	}

	return handoffFromModel(&h), nil
}

func (s *pgHandoffStore) UpdateOutgoingNotes(ctx context.Context, id uuid.UUID, notes string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	res, err := s.db.NewUpdate().Model((*models.HandoffRecord)(nil)).
		Set("outgoing_notes = ?", notes).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update outgoing notes: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update outgoing notes: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("handoff record not found: %w", ErrNotFound)
	}
	return nil
}

func (s *pgHandoffStore) UpdateIncomingNotes(ctx context.Context, id uuid.UUID, notes string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	res, err := s.db.NewUpdate().Model((*models.HandoffRecord)(nil)).
		Set("incoming_notes = ?", notes).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update incoming notes: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update incoming notes: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("handoff record not found: %w", ErrNotFound)
	}
	return nil
}

func (s *pgHandoffStore) Acknowledge(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	res, err := s.db.NewUpdate().Model((*models.HandoffRecord)(nil)).
		Set("status = ?", "acknowledged").
		Set("incoming_acknowledged_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to acknowledge handoff record: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to acknowledge handoff record: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("handoff record not found: %w", ErrNotFound)
	}
	return nil
}
