package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/capability"
	"alga/config"
	algacrypto "alga/crypto"
	"alga/ent"
	"alga/ent/agenttoken"
	"alga/logger"
)

const (
	AgentTypeHermes   = "hermes"
	AgentTypeOpenClaw = "openclaw"
	AgentTypeOther    = "other"
)

func NormalizeAgentType(s string) string {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case AgentTypeOpenClaw:
		return AgentTypeOpenClaw
	case AgentTypeOther:
		return AgentTypeOther
	default:
		return AgentTypeHermes
	}
}

func ParseAgentType(s string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "":
		return AgentTypeHermes, nil
	case AgentTypeHermes:
		return AgentTypeHermes, nil
	case AgentTypeOpenClaw:
		return AgentTypeOpenClaw, nil
	case AgentTypeOther:
		return AgentTypeOther, nil
	default:
		return "", fmt.Errorf("invalid agent_type (use %q, %q, or %q): %w", AgentTypeHermes, AgentTypeOpenClaw, AgentTypeOther, ErrInvalidAgentType)
	}
}

type AgentTokenRecord struct {
	ID             uuid.UUID               `json:"id"`
	Name           string                  `json:"name"`
	AgentType      string                  `json:"agent_type"`
	TokenHash      string                  `json:"-"`
	LookupPrefix   string                  `json:"-"`
	Token          string                  `json:"token,omitempty"`
	CreatedAt      time.Time               `json:"created_at"`
	LastUsedAt     *time.Time              `json:"last_used_at,omitempty"`
	ExpiresAt      *time.Time              `json:"expires_at,omitempty"`
	Revoked        bool                    `json:"revoked"`
	Enabled        bool                    `json:"enabled"`
	Scope          string                  `json:"scope,omitempty"`
	Capabilities   []string                `json:"capabilities,omitempty"`
	LabelSelectors []config.RouteCondition `json:"label_selectors,omitempty"`
}

type AgentTokenStore interface {
	CreateToken(name string, expiresAt *time.Time, agentType string, capabilities []string) (*AgentTokenRecord, error)
	ListTokens() ([]AgentTokenRecord, error)
	RevokeToken(id uuid.UUID) error
	RegenerateToken(id uuid.UUID) (*AgentTokenRecord, error)
	ValidateToken(token string) (*AgentTokenRecord, error)
	GetActiveAgentTokenByID(id uuid.UUID) (*AgentTokenRecord, error)
	UpdateAgentConfig(id uuid.UUID, scope string, selectors []config.RouteCondition, capabilities []string) error
	SetAgentEnabled(id uuid.UUID, enabled bool) error
	ListActiveAgents() ([]AgentTokenRecord, error)
	Close()
}

func generateAgentToken() (string, error) {
	return generateTokenBase64("alga_agent_", 48)
}

type pgAgentTokenStore struct {
	pgStoreBase
}

func newPGAgentTokenStore(client *ent.Client) AgentTokenStore {
	return &pgAgentTokenStore{pgStoreBase{client: client}}
}

func (s *pgAgentTokenStore) CreateToken(name string, expiresAt *time.Time, agentType string, capabilities []string) (*AgentTokenRecord, error) {
	at, err := ParseAgentType(agentType)
	if err != nil {
		return nil, err
	}
	caps := capability.Normalize(capabilities)
	tokenStr, err := generateAgentToken()
	if err != nil {
		return nil, err
	}
	tokenHash := hashToken(tokenStr)
	prefix := lookupPrefix(tokenStr)

	ctx, cancel := pgctx(context.Background())
	defer cancel()

	b := s.client.AgentToken.Create().
		SetName(name).
		SetAgentType(at).
		SetTokenHash(tokenHash).
		SetLookupPrefix(prefix).
		SetCreatedAt(time.Now().UTC()).
		SetRevoked(false).
		SetEnabled(true).
		SetCapabilities(caps)

	if expiresAt != nil {
		b.SetExpiresAt(*expiresAt)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent token: %w", err)
	}

	return &AgentTokenRecord{
		ID:           saved.ID,
		Name:         saved.Name,
		AgentType:    at,
		TokenHash:    tokenHash,
		LookupPrefix: prefix,
		Token:        tokenStr,
		CreatedAt:    saved.CreatedAt,
		Revoked:      false,
		Enabled:      true,
		ExpiresAt:    expiresAt,
		Capabilities: caps,
	}, nil
}

func (s *pgAgentTokenStore) ListTokens() ([]AgentTokenRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tokens, err := s.client.AgentToken.Query().Where(agenttoken.Revoked(false)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list agent tokens: %w", err)
	}

	records := make([]AgentTokenRecord, 0, len(tokens))
	for _, t := range tokens {
		records = append(records, AgentTokenRecord{
			ID:           t.ID,
			Name:         t.Name,
			AgentType:    NormalizeAgentType(t.AgentType),
			TokenHash:    t.TokenHash,
			LookupPrefix: t.LookupPrefix,
			Token:        maskSuffix(t.TokenHash),
			CreatedAt:    t.CreatedAt,
			LastUsedAt:   t.LastUsedAt,
			ExpiresAt:    t.ExpiresAt,
			Revoked:      t.Revoked,
			Enabled:      t.Enabled,
			Scope:        t.Scope,
			Capabilities: t.Capabilities,
		})
	}
	return records, nil
}

func (s *pgAgentTokenStore) RevokeToken(id uuid.UUID) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	_, err := s.client.AgentToken.UpdateOneID(id).
		SetRevoked(true).
		SetEnabled(false).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrTokenNotFound
		}
		return fmt.Errorf("failed to revoke agent token: %w", err)
	}
	return nil
}

func (s *pgAgentTokenStore) RegenerateToken(id uuid.UUID) (*AgentTokenRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rec, err := s.client.AgentToken.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("agent not found or inactive: %w", ErrAgentNotFoundInactive)
		}
		return nil, fmt.Errorf("failed to load agent token: %w", err)
	}
	if rec.Revoked {
		return nil, fmt.Errorf("agent not found or inactive: %w", ErrAgentNotFoundInactive)
	}
	if rec.ExpiresAt != nil && time.Now().After(*rec.ExpiresAt) {
		return nil, fmt.Errorf("agent not found or inactive: %w", ErrAgentNotFoundInactive)
	}

	tokenStr, err := generateAgentToken()
	if err != nil {
		return nil, err
	}
	tokenHash := hashToken(tokenStr)
	prefix := lookupPrefix(tokenStr)

	_, err = s.client.AgentToken.UpdateOneID(id).
		SetTokenHash(tokenHash).
		SetLookupPrefix(prefix).
		ClearLastUsedAt().
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to regenerate agent token: %w", err)
	}

	return &AgentTokenRecord{
		ID:           rec.ID,
		Name:         rec.Name,
		AgentType:    NormalizeAgentType(rec.AgentType),
		TokenHash:    tokenHash,
		LookupPrefix: prefix,
		Token:        tokenStr,
		CreatedAt:    rec.CreatedAt,
		Revoked:      rec.Revoked,
		Enabled:      rec.Enabled,
		Scope:        rec.Scope,
		Capabilities: rec.Capabilities,
	}, nil
}
func (s *pgAgentTokenStore) ValidateToken(token string) (*AgentTokenRecord, error) {
	if token == "" {
		return nil, nil
	}
	prefix := lookupPrefix(token)
	tokenHash := hashToken(token)

	ctx, cancel := pgctx(context.Background())
	defer cancel()

	tokens, err := s.client.AgentToken.Query().
		Where(agenttoken.LookupPrefix(prefix), agenttoken.Revoked(false), agenttoken.Enabled(true)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to validate agent token: %w", err)
	}

	for _, t := range tokens {
		if !algacrypto.ConstantTimeEqualString(tokenHash, t.TokenHash) {
			continue
		}
		if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
			return nil, nil
		}
		go s.updateAgentTokenLastUsed(t.ID, t.LastUsedAt)
		return &AgentTokenRecord{
			ID:           t.ID,
			Name:         t.Name,
			AgentType:    NormalizeAgentType(t.AgentType),
			TokenHash:    t.TokenHash,
			LookupPrefix: t.LookupPrefix,
			CreatedAt:    t.CreatedAt,
			Revoked:      t.Revoked,
			Enabled:      t.Enabled,
			Scope:        t.Scope,
			Capabilities: t.Capabilities,
		}, nil
	}
	return nil, nil
}

func (s *pgAgentTokenStore) updateAgentTokenLastUsed(id uuid.UUID, lastUsedAt *time.Time) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("goroutine panic recovered", "panic", r, "location", "agent-token-updateLastUsed")
		}
	}()
	if lastUsedAt != nil && time.Since(*lastUsedAt) < 24*time.Hour {
		return
	}
	ctx, cancel := pgctx(context.Background())
	defer cancel()
	_ = s.client.AgentToken.UpdateOneID(id).
		SetLastUsedAt(time.Now().UTC()).
		Exec(ctx)
}

func (s *pgAgentTokenStore) GetActiveAgentTokenByID(id uuid.UUID) (*AgentTokenRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	rec, err := s.client.AgentToken.Get(ctx, id)
	if err != nil {
		return handleQueryErr[*AgentTokenRecord](err, "agent token")
	}
	if rec.Revoked {
		return nil, nil
	}
	if rec.ExpiresAt != nil && time.Now().After(*rec.ExpiresAt) {
		return nil, nil
	}
	return &AgentTokenRecord{
		ID:           rec.ID,
		Name:         rec.Name,
		AgentType:    NormalizeAgentType(rec.AgentType),
		TokenHash:    rec.TokenHash,
		LookupPrefix: rec.LookupPrefix,
		Token:        maskSuffix(rec.TokenHash),
		CreatedAt:    rec.CreatedAt,
		Revoked:      rec.Revoked,
		Enabled:      rec.Enabled,
		Scope:        rec.Scope,
		Capabilities: rec.Capabilities,
	}, nil
}

func (s *pgAgentTokenStore) UpdateAgentConfig(id uuid.UUID, scope string, selectors []config.RouteCondition, capabilities []string) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	if selectors == nil {
		selectors = []config.RouteCondition{}
	}
	caps := capability.Normalize(capabilities)

	_, err := s.client.AgentToken.UpdateOneID(id).
		Where(agenttoken.Revoked(false)).
		SetScope(scope).
		SetLabelSelectors(routeConditionsToSchema(selectors)).
		SetCapabilities(caps).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("agent not found or inactive: %w", ErrAgentNotFoundInactive)
		}
		return fmt.Errorf("failed to update agent config: %w", err)
	}
	return nil
}

func (s *pgAgentTokenStore) SetAgentEnabled(id uuid.UUID, enabled bool) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	_, err := s.client.AgentToken.UpdateOneID(id).
		Where(agenttoken.Revoked(false)).
		SetEnabled(enabled).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("agent not found or inactive: %w", ErrAgentNotFoundInactive)
		}
		return fmt.Errorf("failed to update agent enabled state: %w", err)
	}
	return nil
}

func (s *pgAgentTokenStore) ListActiveAgents() ([]AgentTokenRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tokens, err := s.client.AgentToken.Query().
		Where(agenttoken.Revoked(false), agenttoken.Enabled(true)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list active agents: %w", err)
	}

	records := make([]AgentTokenRecord, 0, len(tokens))
	for _, t := range tokens {
		if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
			continue
		}
		scope := t.Scope
		if scope == "" {
			scope = "all"
		}
		var labelSelectors []config.RouteCondition
		if t.LabelSelectors != nil {
			labelSelectors = routeConditionsFromSchema(t.LabelSelectors)
		}
		records = append(records, AgentTokenRecord{
			ID:             t.ID,
			Name:           t.Name,
			AgentType:      NormalizeAgentType(t.AgentType),
			CreatedAt:      t.CreatedAt,
			Enabled:        t.Enabled,
			Scope:          scope,
			Capabilities:   t.Capabilities,
			LabelSelectors: labelSelectors,
		})
	}
	return records, nil
}

func (s *pgAgentTokenStore) Close() {}
