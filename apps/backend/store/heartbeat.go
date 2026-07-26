package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/heartbeat"
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

func newPGHeartbeatStore(client *ent.Client) HeartbeatStore {
	return &pgHeartbeatStore{pgStoreBase{client: client}}
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

	saved, err := s.client.Heartbeat.Create().
		SetName(record.Name).
		SetDescription(record.Description).
		SetIntervalSeconds(record.IntervalSeconds).
		SetGraceSeconds(record.GraceSeconds).
		SetEnabled(record.Enabled).
		SetNillableOwnerTeamID(record.OwnerTeamID).
		SetStatus(heartbeat.Status(record.Status)).
		SetSeverity(heartbeat.Severity(record.Severity)).
		SetLabels(record.Labels).
		SetPingTokenHash(record.PingTokenHash).
		SetLookupPrefix(record.LookupPrefix).
		SetExpiresAt(expiresAt).
		SetCreatedBy(record.CreatedBy).
		SetCreatedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert heartbeat: %w", err)
	}

	out := pgHeartbeatToRecord(saved)
	out.PingToken = tokenStr
	return out, nil
}

func (s *pgHeartbeatStore) Update(ctx context.Context, id uuid.UUID, patch *HeartbeatRecord) (*HeartbeatRecord, error) {
	if patch == nil {
		return nil, errors.New("nil patch")
	}

	b := s.client.Heartbeat.UpdateOneID(id).SetUpdatedAt(time.Now().UTC())

	if patch.Name != "" {
		b.SetName(patch.Name)
	}
	if patch.Description != "" {
		b.SetDescription(patch.Description)
	}
	if patch.IntervalSeconds > 0 {
		b.SetIntervalSeconds(patch.IntervalSeconds)
	}
	if patch.GraceSeconds >= 0 {
		b.SetGraceSeconds(patch.GraceSeconds)
	}
	if patch.Severity != "" {
		b.SetSeverity(heartbeat.Severity(patch.Severity))
	}
	if patch.Labels != nil {
		b.SetLabels(patch.Labels)
	}
	if patch.OwnerTeamID != nil {
		b.SetOwnerTeamID(*patch.OwnerTeamID)
	}
	if patch.EnabledSet {
		b.SetEnabled(patch.Enabled)
	}

	// Changing interval/grace re-arms the deadline from the last ping (or now).
	if patch.IntervalSeconds > 0 || patch.GraceSeconds >= 0 {
		existing, err := s.client.Heartbeat.Get(ctx, id)
		if err != nil {
			if ent.IsNotFound(err) {
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
		b.SetExpiresAt(expiresAt)
		b.SetStatus(heartbeat.Status(HeartbeatStatusHealthy))
	}

	saved, err := b.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrHeartbeatNotFound
		}
		return nil, fmt.Errorf("failed to update heartbeat: %w", err)
	}
	return pgHeartbeatToRecord(saved), nil
}

func (s *pgHeartbeatStore) Delete(ctx context.Context, id uuid.UUID) error {
	err := s.client.Heartbeat.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrHeartbeatNotFound
		}
		return fmt.Errorf("failed to delete heartbeat: %w", err)
	}
	return nil
}

func (s *pgHeartbeatStore) Get(ctx context.Context, id uuid.UUID) (*HeartbeatRecord, error) {
	hb, err := s.client.Heartbeat.Get(ctx, id)
	if err != nil {
		return handleQueryErr[*HeartbeatRecord](err, "heartbeat")
	}
	return pgHeartbeatToRecord(hb), nil
}

func (s *pgHeartbeatStore) List(ctx context.Context, q HeartbeatQuery) ([]HeartbeatRecord, int64, error) {
	query := s.client.Heartbeat.Query()

	if q.Enabled != nil {
		query = query.Where(heartbeat.EnabledEQ(*q.Enabled))
	}
	if q.Status != "" {
		query = query.Where(heartbeat.StatusEQ(heartbeat.Status(q.Status)))
	}
	if q.OwnerTeamID != nil {
		query = query.Where(heartbeat.OwnerTeamIDEQ(*q.OwnerTeamID))
	}
	if q.Search != "" {
		query = query.Where(heartbeat.NameContains(q.Search))
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count heartbeats: %w", err)
	}

	query = query.Order(ent.Desc(heartbeat.FieldCreatedAt))
	if q.Limit > 0 {
		query = query.Limit(q.Limit)
	}
	if q.Skip > 0 {
		query = query.Offset(q.Skip)
	}

	items, err := query.All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("list heartbeats: %w", err)
	}

	out := make([]HeartbeatRecord, 0, len(items))
	for _, hb := range items {
		out = append(out, *pgHeartbeatToRecord(hb))
	}
	return out, int64(total), nil
}

func (s *pgHeartbeatStore) RegenerateToken(ctx context.Context, id uuid.UUID) (*HeartbeatRecord, error) {
	tokenStr, err := generateHeartbeatToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate heartbeat token: %w", err)
	}

	saved, err := s.client.Heartbeat.UpdateOneID(id).
		SetPingTokenHash(hashToken(tokenStr)).
		SetLookupPrefix(lookupPrefix(tokenStr)).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrHeartbeatNotFound
		}
		return nil, fmt.Errorf("failed to regenerate heartbeat token: %w", err)
	}

	out := pgHeartbeatToRecord(saved)
	out.PingToken = tokenStr
	return out, nil
}

func (s *pgHeartbeatStore) GetByPingToken(ctx context.Context, token string) (*HeartbeatRecord, error) {
	if token == "" {
		return nil, nil
	}
	prefix := lookupPrefix(token)
	hash := hashToken(token)

	items, err := s.client.Heartbeat.Query().
		Where(heartbeat.LookupPrefix(prefix), heartbeat.PingTokenHash(hash)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to look up heartbeat by token: %w", err)
	}
	for _, hb := range items {
		return pgHeartbeatToRecord(hb), nil
	}
	return nil, nil
}

func (s *pgHeartbeatStore) RecordPing(ctx context.Context, id uuid.UUID, now time.Time) (*HeartbeatRecord, error) {
	// Re-load to compute the deadline from the configured interval/grace.
	existing, err := s.client.Heartbeat.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrHeartbeatNotFound
		}
		return nil, fmt.Errorf("failed to load heartbeat: %w", err)
	}
	expiresAt := now.Add(time.Duration(existing.IntervalSeconds+existing.GraceSeconds) * time.Second)

	saved, err := s.client.Heartbeat.UpdateOneID(id).
		SetLastPingAt(now).
		SetExpiresAt(expiresAt).
		SetStatus(heartbeat.Status(HeartbeatStatusHealthy)).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrHeartbeatNotFound
		}
		return nil, fmt.Errorf("failed to record heartbeat ping: %w", err)
	}
	return pgHeartbeatToRecord(saved), nil
}

func (s *pgHeartbeatStore) ListExpired(ctx context.Context, now time.Time) ([]HeartbeatRecord, error) {
	items, err := s.client.Heartbeat.Query().
		Where(
			heartbeat.EnabledEQ(true),
			heartbeat.StatusEQ(heartbeat.Status(HeartbeatStatusHealthy)),
			heartbeat.ExpiresAtNotNil(),
			heartbeat.ExpiresAtLT(now),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list expired heartbeats: %w", err)
	}
	out := make([]HeartbeatRecord, 0, len(items))
	for _, hb := range items {
		out = append(out, *pgHeartbeatToRecord(hb))
	}
	return out, nil
}

func (s *pgHeartbeatStore) MarkExpired(ctx context.Context, id uuid.UUID, now time.Time) (*HeartbeatRecord, error) {
	saved, err := s.client.Heartbeat.UpdateOneID(id).
		SetStatus(heartbeat.Status(HeartbeatStatusExpired)).
		SetLastBreachAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, ErrHeartbeatNotFound
		}
		return nil, fmt.Errorf("failed to mark heartbeat expired: %w", err)
	}
	return pgHeartbeatToRecord(saved), nil
}

func pgHeartbeatToRecord(hb *ent.Heartbeat) *HeartbeatRecord {
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
		Status:          string(hb.Status),
		Severity:        string(hb.Severity),
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
