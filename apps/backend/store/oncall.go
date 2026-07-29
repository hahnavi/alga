package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
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

func newPGOnCallStore(db *bun.DB) OnCallStore {
	return &pgOnCallStore{pgStoreBase{db: db}}
}

func createScheduleLayers(ctx context.Context, tx bun.Tx, scheduleID uuid.UUID, layers []ScheduleLayerRecord) error {
	for _, layer := range layers {
		now := time.Now().UTC()
		m := &models.ScheduleLayer{
			ID:               models.NewUUID(),
			ScheduleID:       scheduleID,
			Name:             layer.Name,
			RotationType:     layer.RotationType,
			RotationInterval: layer.RotationInterval,
			StartDate:        layer.StartDate,
			EndDate:          layer.EndDate,
			Timezone:         layer.Timezone,
			StartTime:        layer.StartTime,
			EndTime:          layer.EndTime,
			DaysOfWeek:       layer.DaysOfWeek,
			Priority:         layer.Priority,
			UserIDs:          layer.UserIds,
			CreatedAt:        now,
			UpdatedAt:        now,
		}

		if _, err := tx.NewInsert().Model(m).Exec(ctx); err != nil {
			return fmt.Errorf("failed to create layer: %w", err)
		}
	}
	return nil
}

func (s *pgOnCallStore) CreateSchedule(ctx context.Context, record *OnCallScheduleRecord) (*OnCallScheduleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	now := time.Now().UTC()
	schedID := models.NewUUID()

	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		m := &models.OnCallSchedule{
			BaseModel: models.BaseModel{
				ID:        schedID,
				CreatedAt: now,
				UpdatedAt: now,
			},
			TeamID: record.TeamID,
		}

		if _, err := tx.NewInsert().Model(m).Exec(ctx); err != nil {
			return fmt.Errorf("failed to create schedule: %w", err)
		}

		return createScheduleLayers(ctx, tx, schedID, record.Layers)
	})
	if err != nil {
		return nil, err
	}

	record.ID = schedID
	record.CreatedAt = now
	record.UpdatedAt = now
	return record, nil
}

func (s *pgOnCallStore) GetSchedule(ctx context.Context, id uuid.UUID) (*OnCallScheduleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var sched models.OnCallSchedule
	err := s.db.NewSelect().Model(&sched).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*OnCallScheduleRecord](err, "schedule")
	}

	rec := &OnCallScheduleRecord{
		ID:        sched.ID,
		TeamID:    sched.TeamID,
		CreatedAt: sched.CreatedAt,
		UpdatedAt: sched.UpdatedAt,
	}

	var layers []models.ScheduleLayer
	err = s.db.NewSelect().Model(&layers).
		Where("schedule_id = ?", id).
		Order("created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get layers: %w", err)
	}

	rec.Layers = make([]ScheduleLayerRecord, 0, len(layers))
	for i := range layers {
		l := &layers[i]
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
			UserIds:          l.UserIDs,
			CreatedAt:        l.CreatedAt,
			UpdatedAt:        l.UpdatedAt,
		})
	}

	return rec, nil
}

// GetScheduleByTeam returns the schedule auto-provisioned for a team, with its
// layers loaded. There is one schedule per team, so this uses a single-row
// scan; a missing schedule surfaces as ErrNotFound via handleQueryErr.
func (s *pgOnCallStore) GetScheduleByTeam(ctx context.Context, teamID uuid.UUID) (*OnCallScheduleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var sched models.OnCallSchedule
	err := s.db.NewSelect().Model(&sched).Where("team_id = ?", teamID).Scan(ctx)
	if err != nil {
		return handleQueryErr[*OnCallScheduleRecord](err, "schedule")
	}

	rec := &OnCallScheduleRecord{
		ID:        sched.ID,
		TeamID:    sched.TeamID,
		CreatedAt: sched.CreatedAt,
		UpdatedAt: sched.UpdatedAt,
	}

	var layers []models.ScheduleLayer
	err = s.db.NewSelect().Model(&layers).
		Where("schedule_id = ?", sched.ID).
		Order("created_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get layers: %w", err)
	}

	rec.Layers = make([]ScheduleLayerRecord, 0, len(layers))
	for i := range layers {
		l := &layers[i]
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
			UserIds:          l.UserIDs,
			CreatedAt:        l.CreatedAt,
			UpdatedAt:        l.UpdatedAt,
		})
	}

	return rec, nil
}

func (s *pgOnCallStore) UpdateSchedule(ctx context.Context, id uuid.UUID, record *OnCallScheduleRecord) (*OnCallScheduleRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		q := tx.NewUpdate().Model((*models.OnCallSchedule)(nil)).
			Set("updated_at = ?", time.Now().UTC()).
			Where("id = ?", id)

		if record.TeamID != nil {
			q = q.Set("team_id = ?", *record.TeamID)
		} else {
			q = q.Set("team_id = NULL")
		}

		if _, err := q.Exec(ctx); err != nil {
			return fmt.Errorf("failed to update schedule: %w", err)
		}

		_, err := tx.NewDelete().Model((*models.ScheduleLayer)(nil)).
			Where("schedule_id = ?", id).
			Exec(ctx)
		if err != nil {
			return fmt.Errorf("failed to delete old layers: %w", err)
		}

		return createScheduleLayers(ctx, tx, id, record.Layers)
	})
	if err != nil {
		return nil, err
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

	total, err := s.db.NewSelect().Model((*models.OnCallSchedule)(nil)).Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count schedules: %w", err)
	}

	var schedules []models.OnCallSchedule
	err = s.db.NewSelect().Model(&schedules).
		Order("created_at DESC").
		Limit(limit).
		Offset(skip).
		Scan(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list schedules: %w", err)
	}

	records := make([]OnCallScheduleRecord, 0, len(schedules))
	for i := range schedules {
		sched := &schedules[i]
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

	now := time.Now().UTC()
	m := &models.ScheduleOverride{
		ID:         models.NewUUID(),
		ScheduleID: record.ScheduleID,
		UserID:     record.UserID,
		StartAt:    record.StartAt,
		EndAt:      record.EndAt,
		CreatedBy:  record.CreatedBy,
		CreatedAt:  now,
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create override: %w", err)
	}

	record.ID = m.ID
	record.CreatedAt = m.CreatedAt
	return record, nil
}

func (s *pgOnCallStore) DeleteOverride(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	_, err := s.db.NewDelete().Model((*models.ScheduleOverride)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete override: %w", err)
	}
	return nil
}

func (s *pgOnCallStore) ListOverrides(ctx context.Context, scheduleID uuid.UUID) ([]ScheduleOverrideRecord, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	var overrides []models.ScheduleOverride
	err := s.db.NewSelect().Model(&overrides).
		Where("schedule_id = ?", scheduleID).
		Order("start_at ASC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list overrides: %w", err)
	}

	records := make([]ScheduleOverrideRecord, 0, len(overrides))
	for i := range overrides {
		o := &overrides[i]
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
