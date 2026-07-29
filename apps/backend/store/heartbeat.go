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

const (
	HeartbeatStatusHealthy  = "healthy"
	HeartbeatStatusExpired  = "expired"
	heartbeatTokenPrefix    = "alga_hb_"
	heartbeatTokenRandBytes = 32
)

type HeartbeatRecord struct {
	ID              uuid.UUID         `json:"id"`
	Name            string            `json:"name"`
	Description     string            `json:"description"`
	IntervalSeconds int               `json:"interval_seconds"`
	GraceSeconds    int               `json:"grace_seconds"`
	Enabled         bool              `json:"enabled"`
	EnabledSet      bool              `json:"-"`
	OwnerTeamID     *uuid.UUID        `json:"owner_team_id,omitempty"`
	Status          string            `json:"status"`
	Severity        string            `json:"severity"`
	Labels          map[string]string `json:"labels"`
	PingTokenHash   string            `json:"-"`
	LookupPrefix    string            `json:"-"`
	PingToken       string            `json:"ping_token,omitempty"`
	LastPingAt      *time.Time        `json:"last_ping_at,omitempty"`
	ExpiresAt       *time.Time        `json:"expires_at,omitempty"`
	LastBreachAt    *time.Time        `json:"last_breach_at,omitempty"`
	CreatedBy       string            `json:"created_by"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

type HeartbeatQuery struct {
	Enabled     *bool
	Status      string
	OwnerTeamID *uuid.UUID
	Search      string
	Limit       int
	Skip        int
}

type HeartbeatStore interface {
	Create(ctx context.Context, record *HeartbeatRecord) (*HeartbeatRecord, error)
	Update(ctx context.Context, id uuid.UUID, patch *HeartbeatRecord) (*HeartbeatRecord, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Get(ctx context.Context, id uuid.UUID) (*HeartbeatRecord, error)
	List(ctx context.Context, q HeartbeatQuery) ([]HeartbeatRecord, int64, error)
	RegenerateToken(ctx context.Context, id uuid.UUID) (*HeartbeatRecord, error)
	GetByPingToken(ctx context.Context, token string) (*HeartbeatRecord, error)
	RecordPing(ctx context.Context, id uuid.UUID, now time.Time) (*HeartbeatRecord, error)
	ListExpired(ctx context.Context, now time.Time) ([]HeartbeatRecord, error)
	MarkExpired(ctx context.Context, id uuid.UUID, now time.Time) (*HeartbeatRecord, error)
}

type pgHeartbeatStore struct {
	pgStoreBase
}

func newPGHeartbeatStore(db *bun.DB) HeartbeatStore {
	return &pgHeartbeatStore{pgStoreBase{db: db}}
}

func generateHeartbeatToken() (string, error) {
	return generateTokenBase64(heartbeatTokenPrefix, heartbeatTokenRandBytes)
}

func (s *pgHeartbeatStore) Create(ctx context.Context, record *HeartbeatRecord) (*HeartbeatRecord, error) {
	if record == nil {
		return nil, errors.New("nil record")
	}
	if record.IntervalSeconds <= 0 {
		return nil, errors.New("interval_seconds must be positive")
	}
	if record.GraceSeconds < 0 {
		record.GraceSeconds = 0
	}
	if record.Status == "" {
		record.Status = HeartbeatStatusHealthy
	}
	if record.Severity == "" {
		record.Severity = "warning"
	}
	if record.Labels == nil {
		record.Labels = map[string]string{}
	}

	tokenStr, err := generateHeartbeatToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate heartbeat token: %w", err)
	}

	now := time.Now().UTC()
	record.CreatedAt = now
	record.UpdatedAt = now
	record.PingTokenHash = hashToken(tokenStr)
	record.LookupPrefix = lookupPrefix(tokenStr)

	expiresAt := now.Add(time.Duration(record.IntervalSeconds+record.GraceSeconds) * time.Second)
	record.ExpiresAt = &expiresAt
	record.LastPingAt = nil

	m := &models.Heartbeat{
		BaseModel:       models.BaseModel{ID: models.NewUUID(), CreatedAt: now, UpdatedAt: now},
		Name:            record.Name,
		Description:     record.Description,
		IntervalSeconds: record.IntervalSeconds,
		GraceSeconds:    record.GraceSeconds,
		Enabled:         record.Enabled,
		OwnerTeamID:     record.OwnerTeamID,
		Status:          record.Status,
		Severity:        record.Severity,
		Labels:          record.Labels,
		PingTokenHash:   record.PingTokenHash,
		LookupPrefix:    record.LookupPrefix,
		ExpiresAt:       &expiresAt,
		CreatedBy:       record.CreatedBy,
	}

	_, err = s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert heartbeat: %w", err)
	}

	out := pgHeartbeatToRecord(m)
	out.PingToken = tokenStr
	return out, nil
}

func (s *pgHeartbeatStore) Update(ctx context.Context, id uuid.UUID, patch *HeartbeatRecord) (*HeartbeatRecord, error) {
	if patch == nil {
		return nil, errors.New("nil patch")
	}

	now := time.Now().UTC()
	q := s.db.NewUpdate().Model((*models.Heartbeat)(nil)).
		Set("updated_at = ?", now).
		Where("id = ?", id)

	if patch.Name != "" {
		q = q.Set("name = ?", patch.Name)
	}
	if patch.Description != "" {
		q = q.Set("description = ?", patch.Description)
	}
	if patch.IntervalSeconds > 0 {
		q = q.Set("interval_seconds = ?", patch.IntervalSeconds)
	}
	if patch.GraceSeconds >= 0 {
		q = q.Set("grace_seconds = ?", patch.GraceSeconds)
	}
	if patch.Severity != "" {
		q = q.Set("severity = ?", patch.Severity)
	}
	if patch.Labels != nil {
		q = q.Set("labels = ?", patch.Labels)
	}
	if patch.OwnerTeamID != nil {
		q = q.Set("owner_team_id = ?", *patch.OwnerTeamID)
	}
	if patch.EnabledSet {
		q = q.Set("enabled = ?", patch.Enabled)
	}

	// Changing interval/grace re-arms the deadline from the last ping (or now).
	if patch.IntervalSeconds > 0 || patch.GraceSeconds >= 0 {
		existing := new(models.Heartbeat)
		err := s.db.NewSelect().Model(existing).Where("id = ?", id).Scan(ctx)
		if err != nil {
			if isNotFound(err) {
				return nil, ErrHeartbeatNotFound
			}
			return nil, fmt.Errorf("failed to load heartbeat: %w", err)
		}
		interval := patch.IntervalSeconds
		if interval <= 0 {
			interval = existing.IntervalSeconds
		}
		grace := existing.GraceSeconds
		if patch.GraceSeconds >= 0 {
			grace = patch.GraceSeconds
		}
		base := time.Now().UTC()
		if existing.LastPingAt != nil {
			base = *existing.LastPingAt
		}
		expiresAt := base.Add(time.Duration(interval+grace) * time.Second)
		q = q.Set("expires_at = ?", expiresAt)
		q = q.Set("status = ?", HeartbeatStatusHealthy)
	}

	res, err := q.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to update heartbeat: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to update heartbeat: %w", err)
	}
	if n == 0 {
		return nil, ErrHeartbeatNotFound
	}

	// Reload the updated record.
	updated := new(models.Heartbeat)
	if err := s.db.NewSelect().Model(updated).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to reload heartbeat: %w", err)
	}
	return pgHeartbeatToRecord(updated), nil
}

func (s *pgHeartbeatStore) Delete(ctx context.Context, id uuid.UUID) error {
	res, err := s.db.NewDelete().Model((*models.Heartbeat)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete heartbeat: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to delete heartbeat: %w", err)
	}
	if n == 0 {
		return ErrHeartbeatNotFound
	}
	return nil
}

func (s *pgHeartbeatStore) Get(ctx context.Context, id uuid.UUID) (*HeartbeatRecord, error) {
	hb := new(models.Heartbeat)
	err := s.db.NewSelect().Model(hb).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return handleQueryErr[*HeartbeatRecord](err, "heartbeat")
	}
	return pgHeartbeatToRecord(hb), nil
}

func (s *pgHeartbeatStore) List(ctx context.Context, q HeartbeatQuery) ([]HeartbeatRecord, int64, error) {
	countQ := s.db.NewSelect().Model((*models.Heartbeat)(nil))
	listQ := s.db.NewSelect().Model((*models.Heartbeat)(nil))

	if q.Enabled != nil {
		countQ = countQ.Where("enabled = ?", *q.Enabled)
		listQ = listQ.Where("enabled = ?", *q.Enabled)
	}
	if q.Status != "" {
		countQ = countQ.Where("status = ?", q.Status)
		listQ = listQ.Where("status = ?", q.Status)
	}
	if q.OwnerTeamID != nil {
		countQ = countQ.Where("owner_team_id = ?", *q.OwnerTeamID)
		listQ = listQ.Where("owner_team_id = ?", *q.OwnerTeamID)
	}
	if q.Search != "" {
		countQ = countQ.Where("name LIKE ?", "%"+q.Search+"%")
		listQ = listQ.Where("name LIKE ?", "%"+q.Search+"%")
	}

	total, err := countQ.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count heartbeats: %w", err)
	}

	listQ = listQ.Order("created_at DESC")
	if q.Limit > 0 {
		listQ = listQ.Limit(q.Limit)
	}
	if q.Skip > 0 {
		listQ = listQ.Offset(q.Skip)
	}

	var items []models.Heartbeat
	err = listQ.Scan(ctx, &items)
	if err != nil {
		return nil, 0, fmt.Errorf("list heartbeats: %w", err)
	}

	out := make([]HeartbeatRecord, 0, len(items))
	for i := range items {
		out = append(out, *pgHeartbeatToRecord(&items[i]))
	}
	return out, int64(total), nil
}

func (s *pgHeartbeatStore) RegenerateToken(ctx context.Context, id uuid.UUID) (*HeartbeatRecord, error) {
	tokenStr, err := generateHeartbeatToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate heartbeat token: %w", err)
	}

	res, err := s.db.NewUpdate().Model((*models.Heartbeat)(nil)).
		Set("ping_token_hash = ?", hashToken(tokenStr)).
		Set("lookup_prefix = ?", lookupPrefix(tokenStr)).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to regenerate heartbeat token: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to regenerate heartbeat token: %w", err)
	}
	if n == 0 {
		return nil, ErrHeartbeatNotFound
	}

	// Reload the updated record.
	updated := new(models.Heartbeat)
	if err := s.db.NewSelect().Model(updated).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to reload heartbeat: %w", err)
	}
	out := pgHeartbeatToRecord(updated)
	out.PingToken = tokenStr
	return out, nil
}

func (s *pgHeartbeatStore) GetByPingToken(ctx context.Context, token string) (*HeartbeatRecord, error) {
	if token == "" {
		return nil, nil
	}
	prefix := lookupPrefix(token)
	hash := hashToken(token)

	var items []models.Heartbeat
	err := s.db.NewSelect().Model(&items).
		Where("lookup_prefix = ?", prefix).
		Where("ping_token_hash = ?", hash).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to look up heartbeat by token: %w", err)
	}
	for i := range items {
		return pgHeartbeatToRecord(&items[i]), nil
	}
	return nil, nil
}

func (s *pgHeartbeatStore) RecordPing(ctx context.Context, id uuid.UUID, now time.Time) (*HeartbeatRecord, error) {
	// Re-load to compute the deadline from the configured interval/grace.
	existing := new(models.Heartbeat)
	err := s.db.NewSelect().Model(existing).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil, ErrHeartbeatNotFound
		}
		return nil, fmt.Errorf("failed to load heartbeat: %w", err)
	}
	expiresAt := now.Add(time.Duration(existing.IntervalSeconds+existing.GraceSeconds) * time.Second)

	res, err := s.db.NewUpdate().Model((*models.Heartbeat)(nil)).
		Set("last_ping_at = ?", now).
		Set("expires_at = ?", expiresAt).
		Set("status = ?", HeartbeatStatusHealthy).
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to record heartbeat ping: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to record heartbeat ping: %w", err)
	}
	if n == 0 {
		return nil, ErrHeartbeatNotFound
	}

	// Reload the updated record.
	updated := new(models.Heartbeat)
	if err := s.db.NewSelect().Model(updated).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to reload heartbeat: %w", err)
	}
	return pgHeartbeatToRecord(updated), nil
}

func (s *pgHeartbeatStore) ListExpired(ctx context.Context, now time.Time) ([]HeartbeatRecord, error) {
	var items []models.Heartbeat
	err := s.db.NewSelect().Model(&items).
		Where("enabled = ?", true).
		Where("status = ?", HeartbeatStatusHealthy).
		Where("expires_at IS NOT NULL").
		Where("expires_at < ?", now).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list expired heartbeats: %w", err)
	}
	out := make([]HeartbeatRecord, 0, len(items))
	for i := range items {
		out = append(out, *pgHeartbeatToRecord(&items[i]))
	}
	return out, nil
}

func (s *pgHeartbeatStore) MarkExpired(ctx context.Context, id uuid.UUID, now time.Time) (*HeartbeatRecord, error) {
	res, err := s.db.NewUpdate().Model((*models.Heartbeat)(nil)).
		Set("status = ?", HeartbeatStatusExpired).
		Set("last_breach_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to mark heartbeat expired: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to mark heartbeat expired: %w", err)
	}
	if n == 0 {
		return nil, ErrHeartbeatNotFound
	}

	// Reload the updated record.
	updated := new(models.Heartbeat)
	if err := s.db.NewSelect().Model(updated).Where("id = ?", id).Scan(ctx); err != nil {
		return nil, fmt.Errorf("failed to reload heartbeat: %w", err)
	}
	return pgHeartbeatToRecord(updated), nil
}

func pgHeartbeatToRecord(hb *models.Heartbeat) *HeartbeatRecord {
	var labels map[string]string
	if hb.Labels != nil {
		labels = hb.Labels
	} else {
		labels = map[string]string{}
	}
	return &HeartbeatRecord{
		ID:              hb.ID,
		Name:            hb.Name,
		Description:     hb.Description,
		IntervalSeconds: hb.IntervalSeconds,
		GraceSeconds:    hb.GraceSeconds,
		Enabled:         hb.Enabled,
		OwnerTeamID:     hb.OwnerTeamID,
		Status:          hb.Status,
		Severity:        hb.Severity,
		Labels:          labels,
		PingTokenHash:   hb.PingTokenHash,
		LookupPrefix:    hb.LookupPrefix,
		LastPingAt:      hb.LastPingAt,
		ExpiresAt:       hb.ExpiresAt,
		LastBreachAt:    hb.LastBreachAt,
		CreatedBy:       hb.CreatedBy,
		CreatedAt:       hb.CreatedAt,
		UpdatedAt:       hb.UpdatedAt,
	}
}
