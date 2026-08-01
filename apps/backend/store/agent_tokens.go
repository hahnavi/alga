package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/capability"
	"alga/config"
	algacrypto "alga/crypto"
	"alga/db/models"
	"alga/logger"
)

const (
	AgentTypeAlga     = "alga"
	AgentTypeHermes   = "hermes"
	AgentTypeOpenClaw = "openclaw"
	AgentTypeOther    = "other"
)

func NormalizeAgentType(s string) string {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case AgentTypeAlga:
		return AgentTypeAlga
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
	case AgentTypeAlga:
		return AgentTypeAlga, nil
	case AgentTypeHermes:
		return AgentTypeHermes, nil
	case AgentTypeOpenClaw:
		return AgentTypeOpenClaw, nil
	case AgentTypeOther:
		return AgentTypeOther, nil
	default:
		return "", fmt.Errorf("invalid agent_type (use %q, %q, %q, or %q): %w", AgentTypeAlga, AgentTypeHermes, AgentTypeOpenClaw, AgentTypeOther, ErrInvalidAgentType)
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

func newPGAgentTokenStore(db *bun.DB) AgentTokenStore {
	return &pgAgentTokenStore{pgStoreBase{db: db}}
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

	now := time.Now().UTC()
	m := &models.AgentToken{
		Name:         name,
		AgentType:    at,
		TokenHash:    tokenHash,
		LookupPrefix: prefix,
		Revoked:      false,
		Enabled:      true,
		Capabilities: caps,
		ExpiresAt:    expiresAt,
	}
	m.ID = models.NewUUID()
	m.CreatedAt = now
	m.UpdatedAt = now

	_, err = s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create agent token: %w", err)
	}

	return &AgentTokenRecord{
		ID:           m.ID,
		Name:         m.Name,
		AgentType:    at,
		TokenHash:    tokenHash,
		LookupPrefix: prefix,
		Token:        tokenStr,
		CreatedAt:    m.CreatedAt,
		Revoked:      false,
		Enabled:      true,
		ExpiresAt:    expiresAt,
		Capabilities: caps,
	}, nil
}

func (s *pgAgentTokenStore) ListTokens() ([]AgentTokenRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var tokens []models.AgentToken
	err := s.db.NewSelect().Model(&tokens).Where("revoked = ?", false).Scan(ctx)
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

	res, err := s.db.NewUpdate().Model((*models.AgentToken)(nil)).
		Set("revoked = ?", true).
		Set("enabled = ?", false).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to revoke agent token: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to revoke agent token: %w", err)
	}
	if n == 0 {
		return ErrTokenNotFound
	}
	return nil
}

func (s *pgAgentTokenStore) RegenerateToken(id uuid.UUID) (*AgentTokenRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var rec models.AgentToken
	err := s.db.NewSelect().Model(&rec).Where("id = ?", id).Scan(ctx)
	if err != nil {
		if isNotFound(err) {
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

	_, err = s.db.NewUpdate().Model((*models.AgentToken)(nil)).
		Set("token_hash = ?", tokenHash).
		Set("lookup_prefix = ?", prefix).
		Set("last_used_at = NULL").
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Exec(ctx)
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

	var tokens []models.AgentToken
	err := s.db.NewSelect().Model(&tokens).
		Where("lookup_prefix = ?", prefix).
		Where("revoked = ?", false).
		Where("enabled = ?", true).
		Scan(ctx)
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
	_, _ = s.db.NewUpdate().Model((*models.AgentToken)(nil)).
		Set("last_used_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Exec(ctx)
}

func (s *pgAgentTokenStore) GetActiveAgentTokenByID(id uuid.UUID) (*AgentTokenRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var rec models.AgentToken
	err := s.db.NewSelect().Model(&rec).Where("id = ?", id).Scan(ctx)
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

	res, err := s.db.NewUpdate().Model((*models.AgentToken)(nil)).
		Set("scope = ?", scope).
		Set("label_selectors = ?", routeConditionsToModels(selectors)).
		Set("capabilities = ?", caps).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Where("revoked = ?", false).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update agent config: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update agent config: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("agent not found or inactive: %w", ErrAgentNotFoundInactive)
	}
	return nil
}

func (s *pgAgentTokenStore) SetAgentEnabled(id uuid.UUID, enabled bool) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	res, err := s.db.NewUpdate().Model((*models.AgentToken)(nil)).
		Set("enabled = ?", enabled).
		Set("updated_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Where("revoked = ?", false).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to update agent enabled state: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to update agent enabled state: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("agent not found or inactive: %w", ErrAgentNotFoundInactive)
	}
	return nil
}

func (s *pgAgentTokenStore) ListActiveAgents() ([]AgentTokenRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var tokens []models.AgentToken
	err := s.db.NewSelect().Model(&tokens).
		Where("revoked = ?", false).
		Where("enabled = ?", true).
		Scan(ctx)
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
			labelSelectors = routeConditionsFromModels(t.LabelSelectors)
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
