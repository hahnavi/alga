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

const AlgaAgentDMChatIDLiteral = "alga_dm"

func AlgaAgentDMChatID() string {
	return AlgaAgentDMChatIDLiteral
}

func IsAlgaAgentDMChatID(chatID string) bool {
	return strings.EqualFold(strings.TrimSpace(chatID), AlgaAgentDMChatIDLiteral)
}

type AgentDMMessageRole string

const (
	AgentDMRoleUser  AgentDMMessageRole = "user"
	AgentDMRoleAgent AgentDMMessageRole = "agent"
)

type AgentDMMessage struct {
	ID           uuid.UUID          `json:"id"`
	AgentTokenID string             `json:"agent_token_id"`
	ChatID       string             `json:"chat_id"`
	Role         AgentDMMessageRole `json:"role"`
	Body         string             `json:"body"`
	UserID       *string            `json:"user_id,omitempty"`
	Username     *string            `json:"username,omitempty"`
	Edited       bool               `json:"edited,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

type AgentDMStore interface {
	AddMessage(agentTokenHex string, role AgentDMMessageRole, body string, userID, username *string) (*AgentDMMessage, error)
	ListMessages(agentTokenHex string, beforeID *uuid.UUID, limit int) ([]AgentDMMessage, bool, error)
	UpdateMessageBody(agentTokenHex, messageIDHex, body string, markEdited bool) error
	DeleteMessage(agentTokenHex, messageIDHex string) error
}

type pgAgentDMStore struct {
	pgStoreBase
}

func newPGAgentDMStore(db *bun.DB) AgentDMStore {
	return &pgAgentDMStore{pgStoreBase{db: db}}
}

func (s *pgAgentDMStore) AddMessage(agentTokenHex string, role AgentDMMessageRole, body string, userID, username *string) (*AgentDMMessage, error) {
	hex := strings.ToLower(strings.TrimSpace(agentTokenHex))
	if hex == "" {
		return nil, errors.New("missing agent_token_id")
	}
	now := time.Now().UTC()

	agentTokenID, err := uuid.Parse(hex)
	if err != nil {
		return nil, fmt.Errorf("invalid agent_token_id: %w", err)
	}

	m := &models.AgentDMMessage{
		AgentTokenID: agentTokenID,
		ChatID:       AlgaAgentDMChatID(),
		Role:         string(role),
		Body:         body,
		UserID:       userID,
		Username:     username,
	}
	m.ID = models.NewUUID()
	m.CreatedAt = now
	m.UpdatedAt = now

	ctx, cancel := pgctx(context.Background())
	defer cancel()

	_, err = s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert agent dm message: %w", err)
	}

	return &AgentDMMessage{
		ID:           m.ID,
		AgentTokenID: hex,
		ChatID:       m.ChatID,
		Role:         role,
		Body:         body,
		UserID:       userID,
		Username:     username,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (s *pgAgentDMStore) ListMessages(agentTokenHex string, beforeID *uuid.UUID, limit int) ([]AgentDMMessage, bool, error) {
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 200)

	hex := strings.ToLower(strings.TrimSpace(agentTokenHex))
	if hex == "" {
		return nil, false, errors.New("missing agent_token_id")
	}

	agentTokenID, err := uuid.Parse(hex)
	if err != nil {
		return nil, false, fmt.Errorf("invalid agent_token_id: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var msgs []models.AgentDMMessage
	q := s.db.NewSelect().Model(&msgs).
		Where("agent_token_id = ?", agentTokenID).
		OrderExpr("created_at DESC").
		Limit(limit + 1)

	if beforeID != nil {
		q = q.Where("id < ?", *beforeID)
	}

	err = q.Scan(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("find agent dm messages: %w", err)
	}

	hasMore := len(msgs) > limit
	if hasMore {
		msgs = msgs[:limit]
	}

	var batch []AgentDMMessage
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		batch = append(batch, AgentDMMessage{
			ID:           m.ID,
			AgentTokenID: hex,
			ChatID:       m.ChatID,
			Role:         AgentDMMessageRole(m.Role),
			Body:         m.Body,
			UserID:       m.UserID,
			Username:     m.Username,
			Edited:       m.Edited,
			CreatedAt:    m.CreatedAt,
			UpdatedAt:    m.UpdatedAt,
		})
	}
	return batch, hasMore, nil
}

func (s *pgAgentDMStore) UpdateMessageBody(agentTokenHex, messageIDHex, body string, markEdited bool) error {
	agentTokenID, err := uuid.Parse(strings.ToLower(strings.TrimSpace(agentTokenHex)))
	if err != nil {
		return fmt.Errorf("invalid agent_token_id: %w", err)
	}
	id, err := uuid.Parse(strings.TrimSpace(messageIDHex))
	if err != nil {
		return fmt.Errorf("invalid message id: %w", err)
	}

	ctx, cancel := pgctx(context.Background())
	defer cancel()

	res, err := s.db.NewUpdate().Model((*models.AgentDMMessage)(nil)).
		Set("body = ?", body).
		Set("edited = ?", markEdited).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Where("agent_token_id = ?", agentTokenID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update agent dm message: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update agent dm message: %w", err)
	}
	if n == 0 {
		return errors.New("message not found")
	}
	return nil
}

func (s *pgAgentDMStore) DeleteMessage(agentTokenHex, messageIDHex string) error {
	agentTokenID, err := uuid.Parse(strings.ToLower(strings.TrimSpace(agentTokenHex)))
	if err != nil {
		return fmt.Errorf("invalid agent_token_id: %w", err)
	}
	id, err := uuid.Parse(strings.TrimSpace(messageIDHex))
	if err != nil {
		return fmt.Errorf("invalid message id: %w", err)
	}

	ctx, cancel := pgctx(context.Background())
	defer cancel()

	res, err := s.db.NewDelete().Model((*models.AgentDMMessage)(nil)).
		Where("id = ?", id).
		Where("agent_token_id = ?", agentTokenID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete agent dm message: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to delete agent dm message: %w", err)
	}
	if n == 0 {
		return errors.New("message not found")
	}
	return nil
}
