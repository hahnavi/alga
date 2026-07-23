package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/ent"
	"alga/ent/agentdmmessage"
	"alga/ent/agenttoken"
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

func newPGAgentDMStore(client *ent.Client) AgentDMStore {
	return &pgAgentDMStore{pgStoreBase{client: client}}
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

	b := s.client.AgentDMMessage.Create().
		SetAgentTokenID(agentTokenID).
		SetChatID(AlgaAgentDMChatID()).
		SetRole(string(role)).
		SetBody(body).
		SetCreatedAt(now).
		SetUpdatedAt(now)

	if userID != nil {
		b.SetUserID(*userID)
	}
	if username != nil {
		b.SetUsername(*username)
	}

	ctx, cancel := pgctx(context.Background())
	defer cancel()

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert agent dm message: %w", err)
	}

	return &AgentDMMessage{
		ID:           saved.ID,
		AgentTokenID: hex,
		ChatID:       saved.ChatID,
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

	query := s.client.AgentDMMessage.Query().
		Where(agentdmmessage.HasAgentTokenWith(agenttoken.ID(agentTokenID))).
		Order(ent.Desc(agentdmmessage.FieldCreatedAt)).
		Limit(limit + 1)

	if beforeID != nil {
		query = query.Where(agentdmmessage.IDLT(*beforeID))
	}

	msgs, err := query.All(ctx)
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

	n, err := s.client.AgentDMMessage.Update().
		Where(
			agentdmmessage.ID(id),
			agentdmmessage.HasAgentTokenWith(agenttoken.ID(agentTokenID)),
		).
		SetBody(body).
		SetEdited(markEdited).
		SetUpdatedAt(time.Now().UTC()).
		Save(ctx)
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

	n, err := s.client.AgentDMMessage.Delete().
		Where(
			agentdmmessage.ID(id),
			agentdmmessage.HasAgentTokenWith(agenttoken.ID(agentTokenID)),
		).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete agent dm message: %w", err)
	}
	if n == 0 {
		return errors.New("message not found")
	}
	return nil
}
