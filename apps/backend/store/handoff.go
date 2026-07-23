package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/handoffrecord"
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

func newPGHandoffStore(client *ent.Client) HandoffStore {
	return &pgHandoffStore{pgStoreBase{client: client}}
}

func handoffFromEnt(h *ent.HandoffRecord) *HandoffRecordRecord {
	return &HandoffRecordRecord{
		ID:                     h.ID,
		ScheduleID:             h.ScheduleID,
		OutgoingUserID:         h.OutgoingUserID,
		IncomingUserID:         h.IncomingUserID,
		HandoffAt:              h.HandoffAt,
		Status:                 h.Status.String(),
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

	b := s.client.HandoffRecord.Create().
		SetScheduleID(r.ScheduleID).
		SetHandoffAt(r.HandoffAt).
		SetCreatedAt(time.Now().UTC()).
		SetUpdatedAt(time.Now().UTC())
	if r.Status != "" {
		b.SetStatus(handoffrecord.Status(r.Status))
	}

	if r.OutgoingUserID != nil {
		b.SetOutgoingUserID(*r.OutgoingUserID)
	}
	if r.IncomingUserID != nil {
		b.SetIncomingUserID(*r.IncomingUserID)
	}
	if r.OutgoingNotes != "" {
		b.SetOutgoingNotes(r.OutgoingNotes)
	}
	if r.IncomingNotes != "" {
		b.SetIncomingNotes(r.IncomingNotes)
	}
	if r.IncomingAcknowledgedAt != nil {
		b.SetIncomingAcknowledgedAt(*r.IncomingAcknowledgedAt)
	}
	if r.IncidentSummary != "" {
		b.SetIncidentSummary(r.IncidentSummary)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create handoff record: %w", err)
	}

	record := handoffFromEnt(saved)
	return record, nil
}

func (s *pgHandoffStore) Get(ctx context.Context, id uuid.UUID) (*HandoffRecordRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	h, err := s.client.HandoffRecord.Get(ctx, id)
	if err != nil {
		return handleQueryErr[*HandoffRecordRecord](err, "handoff record")
	}

	return handoffFromEnt(h), nil
}

func (s *pgHandoffStore) List(ctx context.Context, filter HandoffFilter, limit, skip int) ([]*HandoffRecordRecord, int64, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	if limit <= 0 {
		limit = 20
	}
	limit = min(limit, 100)

	q := s.client.HandoffRecord.Query()

	if filter.ScheduleID != nil {
		q.Where(handoffrecord.ScheduleIDEQ(*filter.ScheduleID))
	}
	if filter.UserID != nil {
		q.Where(handoffrecord.Or(
			handoffrecord.OutgoingUserIDEQ(*filter.UserID),
			handoffrecord.IncomingUserIDEQ(*filter.UserID),
		))
	}
	if filter.Status != "" {
		q.Where(handoffrecord.StatusEQ(handoffrecord.Status(filter.Status)))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count handoff records: %w", err)
	}

	records, err := q.
		Order(ent.Desc(handoffrecord.FieldCreatedAt)).
		Limit(limit).
		Offset(skip).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list handoff records: %w", err)
	}

	out := make([]*HandoffRecordRecord, 0, len(records))
	for _, r := range records {
		out = append(out, handoffFromEnt(r))
	}
	return out, int64(total), nil
}

func (s *pgHandoffStore) GetPendingForUser(ctx context.Context, userID uuid.UUID) ([]*HandoffRecordRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	records, err := s.client.HandoffRecord.Query().
		Where(
			handoffrecord.StatusEQ(handoffrecord.StatusPending),
			handoffrecord.Or(
				handoffrecord.OutgoingUserIDEQ(userID),
				handoffrecord.IncomingUserIDEQ(userID),
			),
		).
		Order(ent.Desc(handoffrecord.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending handoffs for user: %w", err)
	}

	out := make([]*HandoffRecordRecord, 0, len(records))
	for _, r := range records {
		out = append(out, handoffFromEnt(r))
	}
	return out, nil
}

func (s *pgHandoffStore) GetLatestForSchedule(ctx context.Context, scheduleID uuid.UUID) (*HandoffRecordRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	h, err := s.client.HandoffRecord.Query().
		Where(handoffrecord.ScheduleIDEQ(scheduleID)).
		Order(ent.Desc(handoffrecord.FieldHandoffAt)).
		Limit(1).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest handoff for schedule: %w", err)
	}

	if len(h) == 0 {
		return nil, fmt.Errorf("handoff record not found: %w", ErrNotFound)
	}

	return handoffFromEnt(h[0]), nil
}

func (s *pgHandoffStore) UpdateOutgoingNotes(ctx context.Context, id uuid.UUID, notes string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	_, err := s.client.HandoffRecord.UpdateOneID(id).
		SetOutgoingNotes(notes).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("handoff record not found: %w", ErrNotFound)
		}
		return fmt.Errorf("failed to update outgoing notes: %w", err)
	}
	return nil
}

func (s *pgHandoffStore) UpdateIncomingNotes(ctx context.Context, id uuid.UUID, notes string) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	_, err := s.client.HandoffRecord.UpdateOneID(id).
		SetIncomingNotes(notes).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("handoff record not found: %w", ErrNotFound)
		}
		return fmt.Errorf("failed to update incoming notes: %w", err)
	}
	return nil
}

func (s *pgHandoffStore) Acknowledge(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	_, err := s.client.HandoffRecord.UpdateOneID(id).
		SetStatus(handoffrecord.StatusAcknowledged).
		SetIncomingAcknowledgedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("handoff record not found: %w", ErrNotFound)
		}
		return fmt.Errorf("failed to acknowledge handoff record: %w", err)
	}
	return nil
}
