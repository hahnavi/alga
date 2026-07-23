package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	algacrypto "alga/crypto"
	"alga/ent"
	"alga/ent/personalaccesstoken"
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

func newPGPersonalAccessTokenStore(client *ent.Client) PersonalAccessTokenStore {
	return &pgPersonalAccessTokenStore{pgStoreBase{client: client}}
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

	b := s.client.PersonalAccessToken.Create().
		SetUserID(userID).
		SetName(name).
		SetTokenHash(hashToken(tokenStr)).
		SetLookupPrefix(lookupPrefix(tokenStr)).
		SetPermissions(permissions).
		SetCreatedAt(time.Now().UTC()).
		SetRevoked(false)

	if expiresAt != nil {
		b.SetExpiresAt(*expiresAt)
	}

	saved, err := b.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create personal access token: %w", err)
	}

	return &PATRecord{
		ID:          saved.ID,
		UserID:      userID,
		Name:        name,
		Token:       tokenStr,
		Permissions: permissions,
		ExpiresAt:   expiresAt,
		CreatedAt:   saved.CreatedAt,
		Revoked:     false,
	}, nil
}

func (s *pgPersonalAccessTokenStore) ListByUser(userID uuid.UUID) ([]PATRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	results, err := s.client.PersonalAccessToken.Query().
		Where(
			personalaccesstoken.UserIDEQ(userID),
		).
		Order(ent.Desc(personalaccesstoken.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list personal access tokens: %w", err)
	}

	out := make([]PATRecord, 0, len(results))
	for _, r := range results {
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

	results, err := s.client.PersonalAccessToken.Query().
		Order(ent.Desc(personalaccesstoken.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list all personal access tokens: %w", err)
	}

	out := make([]PATRecord, 0, len(results))
	for _, r := range results {
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

	_, err := s.client.PersonalAccessToken.UpdateOneID(id).
		SetRevoked(true).
		Where(
			personalaccesstoken.UserIDEQ(userID),
			personalaccesstoken.RevokedEQ(false),
		).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrTokenNotFound
		}
		return fmt.Errorf("failed to revoke personal access token: %w", err)
	}
	return nil
}

func (s *pgPersonalAccessTokenStore) RevokeTokenAdmin(id uuid.UUID) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	_, err := s.client.PersonalAccessToken.UpdateOneID(id).
		SetRevoked(true).
		Where(
			personalaccesstoken.RevokedEQ(false),
		).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return ErrTokenNotFound
		}
		return fmt.Errorf("failed to revoke personal access token: %w", err)
	}
	return nil
}

func (s *pgPersonalAccessTokenStore) ValidateToken(token string) (*PATRecord, error) {
	prefix := lookupPrefix(token)
	hmac := hashToken(token)

	ctx, cancel := pgctx(context.Background())
	defer cancel()

	candidates, err := s.client.PersonalAccessToken.Query().
		Where(
			personalaccesstoken.LookupPrefixEQ(prefix),
			personalaccesstoken.RevokedEQ(false),
		).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to validate personal access token: %w", err)
	}

	for _, c := range candidates {
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
	_ = s.client.PersonalAccessToken.UpdateOneID(id).
		SetLastUsedAt(time.Now().UTC()).
		Exec(ctx)
}

func (s *pgPersonalAccessTokenStore) Close() {}
