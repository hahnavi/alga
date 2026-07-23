package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	entoncallschedule "alga/ent/oncallschedule"
	entschedulelayer "alga/ent/schedulelayer"
	entscheduleoverride "alga/ent/scheduleoverride"
)

type OnCallScheduleRecord struct {
	ID        uuid.UUID             `json:"id"`
	TeamID    *uuid.UUID            `json:"team_id,omitempty"`
	TeamName  string                `json:"team_name"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
	Layers    []ScheduleLayerRecord `json:"layers,omitempty"`
}

type ScheduleLayerRecord struct {
	ID               uuid.UUID  `json:"id"`
	ScheduleID       uuid.UUID  `json:"schedule_id"`
	Name             string     `json:"name"`
	RotationType     string     `json:"rotation_type"`
	RotationInterval int        `json:"rotation_interval"`
	StartDate        time.Time  `json:"start_date"`
	EndDate          *time.Time `json:"end_date,omitempty"`
	// Timezone is the IANA timezone in which this layer's daily-active window
	// and days_of_week are interpreted. The shift resolver applies it per layer.
	Timezone   string    `json:"timezone"`
	StartTime  string    `json:"start_time"`
	EndTime    string    `json:"end_time,omitempty"`
	DaysOfWeek []string  `json:"days_of_week,omitempty"`
	Priority   int       `json:"priority"`
	UserIds    []string  `json:"user_ids,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ScheduleOverrideRecord struct {
	ID         uuid.UUID  `json:"id"`
	ScheduleID uuid.UUID  `json:"schedule_id"`
	UserID     uuid.UUID  `json:"user_id"`
	StartAt    time.Time  `json:"start_at"`
	EndAt      time.Time  `json:"end_at"`
	CreatedBy  *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type OnCallStore interface {
	CreateSchedule(ctx context.Context, record *OnCallScheduleRecord) (*OnCallScheduleRecord, error)
	GetSchedule(ctx context.Context, id uuid.UUID) (*OnCallScheduleRecord, error)
	// GetScheduleByTeam returns the auto-provisioned schedule for a team, with
	// its layers loaded. Returns ErrNotFound when the team has no schedule
	// (e.g. legacy teams pre-auto-provisioning).
	GetScheduleByTeam(ctx context.Context, teamID uuid.UUID) (*OnCallScheduleRecord, error)
	UpdateSchedule(ctx context.Context, id uuid.UUID, record *OnCallScheduleRecord) (*OnCallScheduleRecord, error)
	ListSchedules(ctx context.Context, limit, skip int) ([]OnCallScheduleRecord, int64, error)
	CreateOverride(ctx context.Context, record *ScheduleOverrideRecord) (*ScheduleOverrideRecord, error)
	DeleteOverride(ctx context.Context, id uuid.UUID) error
	ListOverrides(ctx context.Context, scheduleID uuid.UUID) ([]ScheduleOverrideRecord, error)
}

type pgOnCallStore struct {
	pgStoreBase
}

func newPGOnCallStore(client *ent.Client) OnCallStore {
	return &pgOnCallStore{pgStoreBase{client: client}}
}

func createScheduleLayers(ctx context.Context, tx *ent.Tx, scheduleID uuid.UUID, layers []ScheduleLayerRecord) error {
	for _, layer := range layers {
		lb := tx.ScheduleLayer.Create().
			SetScheduleID(scheduleID).
			SetName(layer.Name).
			SetRotationType(layer.RotationType).
			SetRotationInterval(layer.RotationInterval).
			SetStartDate(layer.StartDate).
			SetTimezone(layer.Timezone).
			SetStartTime(layer.StartTime).
			SetEndTime(layer.EndTime).
			SetDaysOfWeek(layer.DaysOfWeek).
			SetPriority(layer.Priority).
			SetUserIds(layer.UserIds).
			SetCreatedAt(time.Now().UTC()).
			SetUpdatedAt(time.Now().UTC())

		if layer.EndDate != nil {
			lb.SetEndDate(*layer.EndDate)
		}

		if _, err := lb.Save(ctx); err != nil {
			return fmt.Errorf("failed to create layer: %w", err)
		}
	}
	return nil
}

func (s *pgOnCallStore) CreateSchedule(ctx context.Context, record *OnCallScheduleRecord) (*OnCallScheduleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer rollbackTx(tx)

	b := tx.OnCallSchedule.Create().
		SetCreatedAt(time.Now().UTC()).
		SetUpdatedAt(time.Now().UTC())

	if record.TeamID != nil {
		b.SetTeamID(*record.TeamID)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create schedule: %w", err)
	}

	if err := createScheduleLayers(ctx, tx, saved.ID, record.Layers); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	record.ID = saved.ID
	record.CreatedAt = saved.CreatedAt
	record.UpdatedAt = saved.UpdatedAt
	return record, nil
}

func (s *pgOnCallStore) GetSchedule(ctx context.Context, id uuid.UUID) (*OnCallScheduleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	sched, err := s.client.OnCallSchedule.Get(ctx, id)
	if err != nil {
		return handleQueryErr[*OnCallScheduleRecord](err, "schedule")
	}

	rec := &OnCallScheduleRecord{
		ID:        sched.ID,
		TeamID:    sched.TeamID,
		CreatedAt: sched.CreatedAt,
		UpdatedAt: sched.UpdatedAt,
	}

	layers, err := s.client.ScheduleLayer.Query().
		Where(entschedulelayer.ScheduleIDEQ(id)).
		Order(ent.Asc(entschedulelayer.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get layers: %w", err)
	}

	rec.Layers = make([]ScheduleLayerRecord, 0, len(layers))
	for _, l := range layers {
		rec.Layers = append(rec.Layers, ScheduleLayerRecord{
			ID:               l.ID,
			ScheduleID:       l.ScheduleID,
			Name:             l.Name,
			RotationType:     l.RotationType,
			RotationInterval: l.RotationInterval,
			StartDate:        l.StartDate,
			EndDate:          l.EndDate,
			Timezone:         l.Timezone,
			StartTime:        l.StartTime,
			EndTime:          l.EndTime,
			DaysOfWeek:       l.DaysOfWeek,
			Priority:         l.Priority,
			UserIds:          l.UserIds,
			CreatedAt:        l.CreatedAt,
			UpdatedAt:        l.UpdatedAt,
		})
	}

	return rec, nil
}

// GetScheduleByTeam returns the schedule auto-provisioned for a team, with its
// layers loaded. There is one schedule per team, so this uses Only(); a missing
// schedule surfaces as ErrNotFound via handleQueryErr.
func (s *pgOnCallStore) GetScheduleByTeam(ctx context.Context, teamID uuid.UUID) (*OnCallScheduleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	sched, err := s.client.OnCallSchedule.Query().
		Where(entoncallschedule.TeamIDEQ(teamID)).
		Only(ctx)
	if err != nil {
		return handleQueryErr[*OnCallScheduleRecord](err, "schedule")
	}

	rec := &OnCallScheduleRecord{
		ID:        sched.ID,
		TeamID:    sched.TeamID,
		CreatedAt: sched.CreatedAt,
		UpdatedAt: sched.UpdatedAt,
	}

	layers, err := s.client.ScheduleLayer.Query().
		Where(entschedulelayer.ScheduleIDEQ(sched.ID)).
		Order(ent.Asc(entschedulelayer.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get layers: %w", err)
	}

	rec.Layers = make([]ScheduleLayerRecord, 0, len(layers))
	for _, l := range layers {
		rec.Layers = append(rec.Layers, ScheduleLayerRecord{
			ID:               l.ID,
			ScheduleID:       l.ScheduleID,
			Name:             l.Name,
			RotationType:     l.RotationType,
			RotationInterval: l.RotationInterval,
			StartDate:        l.StartDate,
			EndDate:          l.EndDate,
			Timezone:         l.Timezone,
			StartTime:        l.StartTime,
			EndTime:          l.EndTime,
			DaysOfWeek:       l.DaysOfWeek,
			Priority:         l.Priority,
			UserIds:          l.UserIds,
			CreatedAt:        l.CreatedAt,
			UpdatedAt:        l.UpdatedAt,
		})
	}

	return rec, nil
}

func (s *pgOnCallStore) UpdateSchedule(ctx context.Context, id uuid.UUID, record *OnCallScheduleRecord) (*OnCallScheduleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer rollbackTx(tx)

	ub := tx.OnCallSchedule.UpdateOneID(id).
		SetUpdatedAt(time.Now().UTC())

	if record.TeamID != nil {
		ub.SetTeamID(*record.TeamID)
	} else {
		ub.ClearTeamID()
	}

	_, err = ub.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update schedule: %w", err)
	}

	_, err = tx.ScheduleLayer.Delete().
		Where(entschedulelayer.ScheduleIDEQ(id)).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to delete old layers: %w", err)
	}

	if err := createScheduleLayers(ctx, tx, id, record.Layers); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return s.GetSchedule(ctx, id)
}

func (s *pgOnCallStore) ListSchedules(ctx context.Context, limit, skip int) ([]OnCallScheduleRecord, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 20
	}
	limit = min(limit, 100)

	total, err := s.client.OnCallSchedule.Query().Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count schedules: %w", err)
	}

	schedules, err := s.client.OnCallSchedule.Query().
		Order(ent.Desc(entoncallschedule.FieldCreatedAt)).
		Limit(limit).
		Offset(skip).
		All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list schedules: %w", err)
	}

	records := make([]OnCallScheduleRecord, 0, len(schedules))
	for _, sched := range schedules {
		records = append(records, OnCallScheduleRecord{
			ID:        sched.ID,
			TeamID:    sched.TeamID,
			CreatedAt: sched.CreatedAt,
			UpdatedAt: sched.UpdatedAt,
		})
	}
	return records, int64(total), nil
}

func (s *pgOnCallStore) CreateOverride(ctx context.Context, record *ScheduleOverrideRecord) (*ScheduleOverrideRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	b := s.client.ScheduleOverride.Create().
		SetScheduleID(record.ScheduleID).
		SetUserID(record.UserID).
		SetStartAt(record.StartAt).
		SetEndAt(record.EndAt).
		SetCreatedAt(time.Now().UTC())

	if record.CreatedBy != nil {
		b.SetCreatedBy(*record.CreatedBy)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create override: %w", err)
	}

	record.ID = saved.ID
	record.CreatedAt = saved.CreatedAt
	return record, nil
}

func (s *pgOnCallStore) DeleteOverride(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	err := s.client.ScheduleOverride.DeleteOneID(id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete override: %w", err)
	}
	return nil
}

func (s *pgOnCallStore) ListOverrides(ctx context.Context, scheduleID uuid.UUID) ([]ScheduleOverrideRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	overrides, err := s.client.ScheduleOverride.Query().
		Where(entscheduleoverride.ScheduleIDEQ(scheduleID)).
		Order(ent.Asc(entscheduleoverride.FieldStartAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list overrides: %w", err)
	}

	records := make([]ScheduleOverrideRecord, 0, len(overrides))
	for _, o := range overrides {
		records = append(records, ScheduleOverrideRecord{
			ID:         o.ID,
			ScheduleID: o.ScheduleID,
			UserID:     o.UserID,
			StartAt:    o.StartAt,
			EndAt:      o.EndAt,
			CreatedBy:  o.CreatedBy,
			CreatedAt:  o.CreatedAt,
		})
	}
	return records, nil
}
