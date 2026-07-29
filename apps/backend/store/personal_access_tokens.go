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

type PATRecord struct {
	ID           uuid.UUID  `json:"id"`
	UserID       uuid.UUID  `json:"user_id"`
	Name         string     `json:"name"`
	TokenHash    string     `json:"-"`
	LookupPrefix string     `json:"-"`
	Token        string     `json:"token,omitempty"`
	Permissions  []string   `json:"permissions"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	Revoked      bool       `json:"revoked"`
}

type PersonalAccessTokenStore interface {
	CreateToken(userID uuid.UUID, name string, permissions []string, expiresAt *time.Time) (*PATRecord, error)
	ListByUser(userID uuid.UUID) ([]PATRecord, error)
	ListAll() ([]PATRecord, error)
	RevokeToken(id uuid.UUID, userID uuid.UUID) error
	RevokeTokenAdmin(id uuid.UUID) error
	ValidateToken(token string) (*PATRecord, error)
	Close()
}

type pgPersonalAccessTokenStore struct {
	pgStoreBase
}

func newPGPersonalAccessTokenStore(db *bun.DB) PersonalAccessTokenStore {
	return &pgPersonalAccessTokenStore{pgStoreBase{db: db}}
}

func generatePATToken() (string, error) {
	return generateTokenBase64("alga_pat_", 48)
}

func (s *pgPersonalAccessTokenStore) CreateToken(userID uuid.UUID, name string, permissions []string, expiresAt *time.Time) (*PATRecord, error) {
	tokenStr, err := generatePATToken()
	if err != nil {
		return nil, err
	}

	ctx, cancel := pgctx(context.Background())
	defer cancel()

	now := time.Now().UTC()
	m := &models.PersonalAccessToken{
		ID:           models.NewUUID(),
		UserID:       userID,
		Name:         name,
		TokenHash:    hashToken(tokenStr),
		LookupPrefix: lookupPrefix(tokenStr),
		Permissions:  permissions,
		CreatedAt:    now,
		Revoked:      false,
		ExpiresAt:    expiresAt,
	}

	_, err = s.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create personal access token: %w", err)
	}

	return &PATRecord{
		ID:          m.ID,
		UserID:      userID,
		Name:        name,
		Token:       tokenStr,
		Permissions: permissions,
		ExpiresAt:   expiresAt,
		CreatedAt:   m.CreatedAt,
		Revoked:     false,
	}, nil
}

func (s *pgPersonalAccessTokenStore) ListByUser(userID uuid.UUID) ([]PATRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var results []models.PersonalAccessToken
	err := s.db.NewSelect().Model(&results).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list personal access tokens: %w", err)
	}

	out := make([]PATRecord, 0, len(results))
	for i := range results {
		r := &results[i]
		out = append(out, PATRecord{
			ID:          r.ID,
			UserID:      r.UserID,
			Name:        r.Name,
			Permissions: r.Permissions,
			ExpiresAt:   r.ExpiresAt,
			LastUsedAt:  r.LastUsedAt,
			CreatedAt:   r.CreatedAt,
			Revoked:     r.Revoked,
		})
	}
	return out, nil
}

func (s *pgPersonalAccessTokenStore) ListAll() ([]PATRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var results []models.PersonalAccessToken
	err := s.db.NewSelect().Model(&results).
		Order("created_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list all personal access tokens: %w", err)
	}

	out := make([]PATRecord, 0, len(results))
	for i := range results {
		r := &results[i]
		out = append(out, PATRecord{
			ID:          r.ID,
			UserID:      r.UserID,
			Name:        r.Name,
			Permissions: r.Permissions,
			ExpiresAt:   r.ExpiresAt,
			LastUsedAt:  r.LastUsedAt,
			CreatedAt:   r.CreatedAt,
			Revoked:     r.Revoked,
		})
	}
	return out, nil
}

func (s *pgPersonalAccessTokenStore) RevokeToken(id uuid.UUID, userID uuid.UUID) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	res, err := s.db.NewUpdate().Model((*models.PersonalAccessToken)(nil)).
		Set("revoked = ?", true).
		Where("id = ?", id).
		Where("user_id = ?", userID).
		Where("revoked = ?", false).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to revoke personal access token: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to revoke personal access token: %w", err)
	}
	if n == 0 {
		return ErrTokenNotFound
	}
	return nil
}

func (s *pgPersonalAccessTokenStore) RevokeTokenAdmin(id uuid.UUID) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	res, err := s.db.NewUpdate().Model((*models.PersonalAccessToken)(nil)).
		Set("revoked = ?", true).
		Where("id = ?", id).
		Where("revoked = ?", false).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to revoke personal access token: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to revoke personal access token: %w", err)
	}
	if n == 0 {
		return ErrTokenNotFound
	}
	return nil
}

func (s *pgPersonalAccessTokenStore) ValidateToken(token string) (*PATRecord, error) {
	prefix := lookupPrefix(token)
	hmac := hashToken(token)

	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var candidates []models.PersonalAccessToken
	err := s.db.NewSelect().Model(&candidates).
		Where("lookup_prefix = ?", prefix).
		Where("revoked = ?", false).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to validate personal access token: %w", err)
	}

	for i := range candidates {
		c := &candidates[i]
		if !algacrypto.ConstantTimeEqual([]byte(hmac), []byte(c.TokenHash)) {
			continue
		}

		if c.ExpiresAt != nil && c.ExpiresAt.Before(time.Now().UTC()) {
			continue
		}

		go s.updateLastUsed(c.ID, c.LastUsedAt)

		return &PATRecord{
			ID:          c.ID,
			UserID:      c.UserID,
			Name:        c.Name,
			Permissions: c.Permissions,
			ExpiresAt:   c.ExpiresAt,
			LastUsedAt:  c.LastUsedAt,
			CreatedAt:   c.CreatedAt,
			Revoked:     c.Revoked,
		}, nil
	}

	return nil, nil
}

func (s *pgPersonalAccessTokenStore) updateLastUsed(id uuid.UUID, lastUsedAt *time.Time) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("goroutine panic recovered", "panic", r, "location", "pat-updateLastUsed")
		}
	}()
	if lastUsedAt != nil && time.Since(*lastUsedAt) < 24*time.Hour {
		return
	}
	ctx, cancel := pgctx(context.Background())
	defer cancel()
	_, _ = s.db.NewUpdate().Model((*models.PersonalAccessToken)(nil)).
		Set("last_used_at = ?", time.Now().UTC()).
		Where("id = ?", id).
		Exec(ctx)
}

func (s *pgPersonalAccessTokenStore) Close() {}
