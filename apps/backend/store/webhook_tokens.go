package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	algacrypto "alga/crypto"
	"alga/db/models"
	"alga/logger"
)

type WebhookTokenRecord struct {
	ID           uuid.UUID  `json:"id"`
	Name         string     `json:"name"`
	TokenHash    string     `json:"-"`
	LookupPrefix string     `json:"-"`
	Token        string     `json:"token,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

func hashToken(token string) string {
	return algacrypto.Default().HMACString(token)
}

func lookupPrefix(token string) string {
	full := algacrypto.PlainSHA256Hex(token)
	if len(full) < 12 {
		return full
	}
	return full[:12]
}

type WebhookTokenStore interface {
	CreateToken(name string, expiresAt *time.Time) (*WebhookTokenRecord, error)
	ListTokens() ([]WebhookTokenRecord, error)
	RevokeToken(id uuid.UUID) error
	ValidateToken(token string) (bool, error)
	Close()
}

func generateWebhookToken() (string, error) {
	return generateTokenBase64("alga_", 48)
}

type pgWebhookTokenStore struct {
	pgStoreBase
}

func newPGWebhookTokenStore(db *bun.DB) WebhookTokenStore {
	return &pgWebhookTokenStore{pgStoreBase{db: db}}
}

func (s *pgWebhookTokenStore) CreateToken(name string, expiresAt *time.Time) (*WebhookTokenRecord, error) {
	tokenStr, err := generateWebhookToken()
	if err != nil {
		return nil, err
	}

	ctx, cancel := pgctx(context.Background())
	defer cancel()

	m := &models.WebhookToken{
		ID:           models.NewUUID(),
		Name:         name,
		TokenHash:    hashToken(tokenStr),
		LookupPrefix: lookupPrefix(tokenStr),
		CreatedAt:    time.Now().UTC(),
		ExpiresAt:    expiresAt,
	}

	_, err = s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	record := &WebhookTokenRecord{
		ID:           m.ID,
		Name:         name,
		TokenHash:    m.TokenHash,
		LookupPrefix: m.LookupPrefix,
		Token:        tokenStr,
		CreatedAt:    m.CreatedAt,
		ExpiresAt:    expiresAt,
	}
	return record, nil
}

func (s *pgWebhookTokenStore) ListTokens() ([]WebhookTokenRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var tokens []models.WebhookToken
	err := s.db.NewSelect().Model(&tokens).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}

	records := make([]WebhookTokenRecord, 0, len(tokens))
	for i := range tokens {
		t := &tokens[i]
		records = append(records, WebhookTokenRecord{
			ID:           t.ID,
			Name:         t.Name,
			TokenHash:    t.TokenHash,
			LookupPrefix: t.LookupPrefix,
			Token:        maskSuffix(t.LookupPrefix),
			CreatedAt:    t.CreatedAt,
			LastUsedAt:   t.LastUsedAt,
			ExpiresAt:    t.ExpiresAt,
		})
	}
	return records, nil
}

func (s *pgWebhookTokenStore) RevokeToken(id uuid.UUID) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	res, err := s.db.NewDelete().Model((*models.WebhookToken)(nil)).Where("id = ?", id).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("token not found: %w", ErrTokenNotFound)
	}
	return nil
}

func (s *pgWebhookTokenStore) ValidateToken(token string) (bool, error) {
	if token == "" {
		return false, nil
	}
	prefix := lookupPrefix(token)
	hash := hashToken(token)

	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var tokens []models.WebhookToken
	err := s.db.NewSelect().Model(&tokens).
		Where("lookup_prefix = ?", prefix).
		Scan(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to validate token: %w", err)
	}

	for i := range tokens {
		t := &tokens[i]
		if !algacrypto.ConstantTimeEqualString(hash, t.TokenHash) {
			continue
		}
		if t.ExpiresAt != nil && time.Now().After(*t.ExpiresAt) {
			return false, nil
		}
		go s.updateLastUsed(t.ID, t.LastUsedAt)
		return true, nil
	}
	return false, nil
}

func (s *pgWebhookTokenStore) updateLastUsed(id uuid.UUID, lastUsedAt *time.Time) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("goroutine panic recovered", "panic", r, "location", "webhook-token-updateLastUsed")
		}
	}()
	if lastUsedAt != nil && time.Since(*lastUsedAt) < 24*time.Hour {
		return
	}
	ctx, cancel := pgctx(context.Background())
	defer cancel()
	_, err := s.db.NewUpdate().Model((*models.WebhookToken)(nil)).
		Set("last_used_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		logger.Error("failed to update webhook token last_used_at", "error", err)
	}
}

func (s *pgWebhookTokenStore) Close() {}
