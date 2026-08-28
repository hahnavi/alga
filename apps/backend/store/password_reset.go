package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/db/models"
)

type PasswordResetToken struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
	CreatedAt time.Time `json:"created_at"`
}

type PasswordResetStore interface {
	CreateToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) (*PasswordResetToken, error)
	GetByTokenHash(ctx context.Context, tokenHash string) (*PasswordResetToken, error)
	MarkUsed(ctx context.Context, id uuid.UUID) error
	// DeleteConsumedExpired purges used tokens and tokens expired before the
	// cutoff (retention family; security hygiene on a tiny table).
	DeleteConsumedExpired(ctx context.Context, expiredBefore time.Time) (int64, error)
}

type pgPasswordResetStore struct {
	pgStoreBase
}

func newPGPasswordResetStore(db *bun.DB) PasswordResetStore {
	return &pgPasswordResetStore{pgStoreBase{db: db}}
}

func (s *pgPasswordResetStore) CreateToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) (*PasswordResetToken, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	m := &models.PasswordResetToken{
		ID:        models.NewUUID(),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: expiresAt,
	}
	if _, err := s.db.NewInsert().Model(m).Exec(ctx); err != nil {
		return nil, fmt.Errorf("create password reset token: %w", err)
	}
	return passwordResetTokenToRecord(m), nil
}

func (s *pgPasswordResetStore) GetByTokenHash(ctx context.Context, tokenHash string) (*PasswordResetToken, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	m := &models.PasswordResetToken{}
	err := s.db.NewSelect().Model(m).Where("token_hash = ?", tokenHash).Scan(ctx)
	if err != nil {
		return handleQueryErr[*PasswordResetToken](err, "password reset token")
	}
	return passwordResetTokenToRecord(m), nil
}

func (s *pgPasswordResetStore) MarkUsed(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	res, err := s.db.NewUpdate().Model((*models.PasswordResetToken)(nil)).
		Set("used = ?", true).
		Where("id = ?", id).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("mark password reset token used: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *pgPasswordResetStore) DeleteConsumedExpired(ctx context.Context, expiredBefore time.Time) (int64, error) {
	ctx, cancel := pgctx(ctx)
	defer cancel()

	res, err := s.db.NewDelete().Model((*models.PasswordResetToken)(nil)).
		Where("used = ?", true).
		WhereOr("expires_at < ?", expiredBefore).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("delete consumed/expired password reset tokens: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func passwordResetTokenToRecord(m *models.PasswordResetToken) *PasswordResetToken {
	return &PasswordResetToken{
		ID:        m.ID,
		UserID:    m.UserID,
		TokenHash: m.TokenHash,
		ExpiresAt: m.ExpiresAt,
		Used:      m.Used,
		CreatedAt: m.CreatedAt,
	}
}
