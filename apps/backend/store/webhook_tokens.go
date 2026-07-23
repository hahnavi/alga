package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	algacrypto "alga/crypto"
	"alga/ent"
	"alga/ent/webhooktoken"
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
	Revoked      bool       `json:"revoked"`
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

func newPGWebhookTokenStore(client *ent.Client) WebhookTokenStore {
	return &pgWebhookTokenStore{pgStoreBase{client: client}}
}

func (s *pgWebhookTokenStore) CreateToken(name string, expiresAt *time.Time) (*WebhookTokenRecord, error) {
	tokenStr, err := generateWebhookToken()
	if err != nil {
		return nil, err
	}

	ctx, cancel := pgctx(context.Background())
	defer cancel()

	record := WebhookTokenRecord{
		Name:         name,
		TokenHash:    hashToken(tokenStr),
		LookupPrefix: lookupPrefix(tokenStr),
		CreatedAt:    time.Now().UTC(),
		Revoked:      false,
		ExpiresAt:    expiresAt,
	}

	b := s.client.WebhookToken.Create().
		SetName(record.Name).
		SetTokenHash(record.TokenHash).
		SetLookupPrefix(record.LookupPrefix).
		SetCreatedAt(record.CreatedAt).
		SetRevoked(false)

	if expiresAt != nil {
		b.SetExpiresAt(*expiresAt)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	record.ID = saved.ID
	record.Token = tokenStr
	return &record, nil
}

func (s *pgWebhookTokenStore) ListTokens() ([]WebhookTokenRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tokens, err := s.client.WebhookToken.Query().Where(webhooktoken.Revoked(false)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}

	records := make([]WebhookTokenRecord, 0, len(tokens))
	for _, t := range tokens {
		records = append(records, WebhookTokenRecord{
			ID:           t.ID,
			Name:         t.Name,
			TokenHash:    t.TokenHash,
			LookupPrefix: t.LookupPrefix,
			Token:        maskSuffix(t.LookupPrefix),
			CreatedAt:    t.CreatedAt,
			LastUsedAt:   t.LastUsedAt,
			ExpiresAt:    t.ExpiresAt,
			Revoked:      t.Revoked,
		})
	}
	return records, nil
}

func (s *pgWebhookTokenStore) RevokeToken(id uuid.UUID) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	n, err := s.client.WebhookToken.Delete().Where(webhooktoken.ID(id)).Exec(ctx)
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

	tokens, err := s.client.WebhookToken.Query().
		Where(webhooktoken.LookupPrefix(prefix), webhooktoken.Revoked(false)).
		All(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to validate token: %w", err)
	}

	for _, t := range tokens {
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
	if err := s.client.WebhookToken.UpdateOneID(id).
		SetLastUsedAt(time.Now().UTC()).
		Exec(ctx); err != nil {
		logger.Error("failed to update webhook token last_used_at", "error", err)
	}
}

func (s *pgWebhookTokenStore) Close() {}
