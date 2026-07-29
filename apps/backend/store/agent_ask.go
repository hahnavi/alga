package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
)

type AgentAskStatus = string

const (
	AgentAskPending   AgentAskStatus = "pending"
	AgentAskAnswered  AgentAskStatus = "answered"
	AgentAskExpired   AgentAskStatus = "expired"
	AgentAskCancelled AgentAskStatus = "cancelled"
)

type AgentAskRecord struct {
	ID                 uuid.UUID      `json:"id"`
	FromAgentID        uuid.UUID      `json:"from_agent_id"`
	FromAgentName      string         `json:"from_agent_name"`
	FromAgentType      string         `json:"from_agent_type"`
	InvestigationID    string         `json:"investigation_id,omitempty"`
	ToAgentID          *uuid.UUID     `json:"to_agent_id,omitempty"`
	ToAgentType        string         `json:"to_agent_type,omitempty"`
	Question           string         `json:"question"`
	Reply              string         `json:"reply,omitempty"`
	RepliedByAgentID   *uuid.UUID     `json:"replied_by_agent_id,omitempty"`
	RepliedByAgentName string         `json:"replied_by_agent_name,omitempty"`
	Status             AgentAskStatus `json:"status"`
	ExpiresAt          time.Time      `json:"expires_at"`
	CreatedAt          time.Time      `json:"created_at"`
	AnsweredAt         *time.Time     `json:"answered_at,omitempty"`
}

type AgentAskQuery struct {
	ForAgentID   *uuid.UUID
	ForAgentType string
	FromAgentID  *uuid.UUID
	Status       AgentAskStatus
	Limit        int
	Skip         int
}

type AgentAskStore interface {
	Create(ctx context.Context, r *AgentAskRecord) (*AgentAskRecord, error)
	Get(ctx context.Context, id string) (*AgentAskRecord, error)
	List(ctx context.Context, q AgentAskQuery) ([]AgentAskRecord, int64, error)
	Reply(ctx context.Context, id string, reply string, repliedBy uuid.UUID, repliedByName string) (*AgentAskRecord, error)
	Cancel(ctx context.Context, id string, requesterID uuid.UUID) error
	ExpirePending(ctx context.Context) (int64, error)
}

type pgAgentAskStore struct {
	pgStoreBase
}

func newPGAgentAskStore(db *bun.DB) AgentAskStore {
	return &pgAgentAskStore{pgStoreBase{db: db}}
}

func (s *pgAgentAskStore) Create(ctx context.Context, r *AgentAskRecord) (*AgentAskRecord, error) {
	if r == nil {
		return nil, errors.New("nil ask record")
	}
	if strings.TrimSpace(r.Question) == "" {
		return nil, errors.New("question is required")
	}
	if r.FromAgentID == uuid.Nil {
		return nil, errors.New("from_agent_id is required")
	}

	now := time.Now().UTC()
	r.CreatedAt = now
	if r.Status == "" {
		r.Status = AgentAskPending
	}
	if r.ExpiresAt.IsZero() {
		r.ExpiresAt = now.Add(10 * time.Minute)
	}
	if r.ID == uuid.Nil {
		r.ID = models.NewUUID()
	}

	m := &models.AgentAsk{
		FromAgentID:     r.FromAgentID,
		FromAgentName:   r.FromAgentName,
		FromAgentType:   r.FromAgentType,
		InvestigationID: r.InvestigationID,
		ToAgentID:       r.ToAgentID,
		Question:        r.Question,
		Status:          r.Status,
		ExpiresAt:       r.ExpiresAt,
	}
	m.ID = r.ID
	m.CreatedAt = now
	m.UpdatedAt = now

	if r.ToAgentType != "" {
		m.ToAgentType = &r.ToAgentType
	}

	_, err := s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert peer ask: %w", err)
	}

	r.ID = m.ID
	return r, nil
}

func (s *pgAgentAskStore) Get(ctx context.Context, id string) (*AgentAskRecord, error) {
	uid, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return nil, fmt.Errorf("invalid ask id: %w", err)
	}

	var a models.AgentAsk
	err = s.db.NewSelect().Model(&a).Where("id = ?", uid).Scan(ctx)
	if err != nil {
		return handleQueryErr[*AgentAskRecord](err, "peer ask")
	}
	return pgAgentAskToRecord(&a), nil
}

func (s *pgAgentAskStore) List(ctx context.Context, q AgentAskQuery) ([]AgentAskRecord, int64, error) {
	applyAskFilters := func(sq *bun.SelectQuery) *bun.SelectQuery {
		if q.Status != "" {
			sq = sq.Where("status = ?", q.Status)
		}
		if q.FromAgentID != nil {
			sq = sq.Where("from_agent_id = ?", *q.FromAgentID)
		}
		if q.ForAgentID != nil {
			uid := *q.ForAgentID
			if q.ForAgentType != "" {
				sq = sq.Where("(to_agent_id = ? OR (to_agent_id IS NULL AND to_agent_type = ?))", uid, q.ForAgentType)
			} else {
				sq = sq.Where("to_agent_id = ?", uid)
			}
		} else if q.ForAgentType != "" {
			sq = sq.Where("to_agent_type = ?", q.ForAgentType)
		}
		return sq
	}

	total, err := applyAskFilters(s.db.NewSelect().Model((*models.AgentAsk)(nil))).Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count peer asks: %w", err)
	}

	listQ := applyAskFilters(s.db.NewSelect().Model((*models.AgentAsk)(nil)))
	listQ = listQ.OrderExpr("created_at DESC")
	if q.Limit > 0 {
		listQ = listQ.Limit(q.Limit)
	}
	if q.Skip > 0 {
		listQ = listQ.Offset(q.Skip)
	}

	var asks []models.AgentAsk
	err = listQ.Scan(ctx, &asks)
	if err != nil {
		return nil, 0, fmt.Errorf("find peer asks: %w", err)
	}

	var out []AgentAskRecord
	for _, a := range asks {
		out = append(out, *pgAgentAskToRecord(&a))
	}
	if out == nil {
		out = []AgentAskRecord{}
	}
	return out, int64(total), nil
}

func (s *pgAgentAskStore) Reply(ctx context.Context, id string, reply string, repliedBy uuid.UUID, repliedByName string) (*AgentAskRecord, error) {
	uid, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return nil, fmt.Errorf("invalid ask id: %w", err)
	}

	reply = strings.TrimSpace(reply)
	if reply == "" {
		return nil, errors.New("reply is required")
	}

	now := time.Now().UTC()

	res, err := s.db.NewUpdate().Model((*models.AgentAsk)(nil)).
		Set("reply = ?", reply).
		Set("replied_by_agent_id = ?", repliedBy).
		Set("replied_by_agent_name = ?", repliedByName).
		Set("status = ?", AgentAskAnswered).
		Set("answered_at = ?", now).
		Set("updated_at = ?", now).
		Where("id = ?", uid).
		Where("status = ?", AgentAskPending).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("reply peer ask: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("reply peer ask: %w", err)
	}
	if n == 0 {
		return nil, errors.New("ask is not pending (already answered, cancelled, or expired)")
	}

	return s.Get(ctx, id)
}

func (s *pgAgentAskStore) Cancel(ctx context.Context, id string, requesterID uuid.UUID) error {
	uid, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("invalid ask id: %w", err)
	}

	res, err := s.db.NewUpdate().Model((*models.AgentAsk)(nil)).
		Set("status = ?", AgentAskCancelled).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", uid).
		Where("from_agent_id = ?", requesterID).
		Where("status = ?", AgentAskPending).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("cancel peer ask: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("cancel peer ask: %w", err)
	}
	if n == 0 {
		return errors.New("ask not found, not pending, or not owned by caller")
	}
	return nil
}

func (s *pgAgentAskStore) ExpirePending(ctx context.Context) (int64, error) {
	res, err := s.db.NewUpdate().Model((*models.AgentAsk)(nil)).
		Set("status = ?", AgentAskExpired).
		Set("updated_at = ?", time.Now().UTC()).
		Where("status = ?", AgentAskPending).
		Where("expires_at < ?", time.Now().UTC()).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("expire peer asks: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("expire peer asks: %w", err)
	}
	return n, nil
}

func pgAgentAskToRecord(a *models.AgentAsk) *AgentAskRecord {
	r := &AgentAskRecord{
		ID:                 a.ID,
		FromAgentID:        a.FromAgentID,
		FromAgentName:      a.FromAgentName,
		FromAgentType:      a.FromAgentType,
		InvestigationID:    a.InvestigationID,
		ToAgentID:          a.ToAgentID,
		Question:           a.Question,
		Reply:              a.Reply,
		RepliedByAgentID:   a.RepliedByAgentID,
		RepliedByAgentName: a.RepliedByAgentName,
		Status:             a.Status,
		ExpiresAt:          a.ExpiresAt,
		CreatedAt:          a.CreatedAt,
		AnsweredAt:         a.AnsweredAt,
	}
	if a.ToAgentType != nil {
		r.ToAgentType = *a.ToAgentType
	}
	return r
}
