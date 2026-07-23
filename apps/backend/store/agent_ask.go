package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/agentask"
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

func newPGAgentAskStore(client *ent.Client) AgentAskStore {
	return &pgAgentAskStore{pgStoreBase{client: client}}
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
		r.ID = uuid.Must(uuid.NewV7())
	}

	b := s.client.AgentAsk.Create().
		SetFromAgentID(r.FromAgentID).
		SetFromAgentName(r.FromAgentName).
		SetFromAgentType(r.FromAgentType).
		SetInvestigationID(r.InvestigationID).
		SetQuestion(r.Question).
		SetStatus(string(r.Status)).
		SetExpiresAt(r.ExpiresAt).
		SetCreatedAt(now)

	if r.ToAgentID != nil {
		b.SetToAgentID(*r.ToAgentID)
	}
	if r.ToAgentType != "" {
		b.SetToAgentType(r.ToAgentType)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert peer ask: %w", err)
	}

	r.ID = saved.ID
	return r, nil
}

func (s *pgAgentAskStore) Get(ctx context.Context, id string) (*AgentAskRecord, error) {
	uid, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return nil, fmt.Errorf("invalid ask id: %w", err)
	}

	a, err := s.client.AgentAsk.Get(ctx, uid)
	if err != nil {
		return handleQueryErr[*AgentAskRecord](err, "peer ask")
	}
	return pgAgentAskToRecord(a), nil
}

func (s *pgAgentAskStore) List(ctx context.Context, q AgentAskQuery) ([]AgentAskRecord, int64, error) {
	query := s.client.AgentAsk.Query()

	if q.Status != "" {
		query = query.Where(agentask.Status(string(q.Status)))
	}
	if q.FromAgentID != nil {
		query = query.Where(agentask.FromAgentID(*q.FromAgentID))
	}
	if q.ForAgentID != nil {
		uid := *q.ForAgentID
		if q.ForAgentType != "" {
			query = query.Where(agentask.Or(
				agentask.ToAgentID(uid),
				agentask.And(
					agentask.ToAgentIDIsNil(),
					agentask.ToAgentType(q.ForAgentType),
				),
			))
		} else {
			query = query.Where(agentask.ToAgentID(uid))
		}
	} else if q.ForAgentType != "" {
		query = query.Where(agentask.ToAgentType(q.ForAgentType))
	}

	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count peer asks: %w", err)
	}

	query = query.Order(ent.Desc(agentask.FieldCreatedAt))
	if q.Limit > 0 {
		query = query.Limit(q.Limit)
	}
	if q.Skip > 0 {
		query = query.Offset(q.Skip)
	}

	asks, err := query.All(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("find peer asks: %w", err)
	}

	var out []AgentAskRecord
	for _, a := range asks {
		out = append(out, *pgAgentAskToRecord(a))
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

	n, err := s.client.AgentAsk.Update().
		Where(agentask.ID(uid), agentask.Status(AgentAskPending)).
		SetReply(reply).
		SetRepliedByAgentID(repliedBy).
		SetRepliedByAgentName(repliedByName).
		SetStatus(AgentAskAnswered).
		SetAnsweredAt(now).
		Save(ctx)
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

	n, err := s.client.AgentAsk.Update().
		Where(
			agentask.ID(uid),
			agentask.FromAgentID(requesterID),
			agentask.Status(AgentAskPending),
		).
		SetStatus(AgentAskCancelled).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("cancel peer ask: %w", err)
	}
	if n == 0 {
		return errors.New("ask not found, not pending, or not owned by caller")
	}
	return nil
}

func (s *pgAgentAskStore) ExpirePending(ctx context.Context) (int64, error) {
	n, err := s.client.AgentAsk.Update().
		Where(
			agentask.Status(AgentAskPending),
			agentask.ExpiresAtLT(time.Now().UTC()),
		).
		SetStatus(AgentAskExpired).
		Save(ctx)
	if err != nil {
		return 0, fmt.Errorf("expire peer asks: %w", err)
	}
	return int64(n), nil
}

func pgAgentAskToRecord(a *ent.AgentAsk) *AgentAskRecord {
	return &AgentAskRecord{
		ID:                 a.ID,
		FromAgentID:        a.FromAgentID,
		FromAgentName:      a.FromAgentName,
		FromAgentType:      a.FromAgentType,
		InvestigationID:    a.InvestigationID,
		ToAgentID:          a.ToAgentID,
		ToAgentType:        a.ToAgentType,
		Question:           a.Question,
		Reply:              a.Reply,
		RepliedByAgentID:   a.RepliedByAgentID,
		RepliedByAgentName: a.RepliedByAgentName,
		Status:             AgentAskStatus(a.Status),
		ExpiresAt:          a.ExpiresAt,
		CreatedAt:          a.CreatedAt,
		AnsweredAt:         a.AnsweredAt,
	}
}
