package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"alga/crypto"
	"alga/ent"
	"alga/ent/session"
)

type SessionRecord struct {
	ID                     string    `json:"-"`
	IDHash                 string    `json:"id"`
	UserID                 uuid.UUID `json:"user_id"`
	RefreshToken           string    `json:"-"`
	RefreshTokenHash       string    `json:"refresh_token,omitempty"`
	PrevRefreshTokenHashes []string  `json:"prev_refresh_tokens,omitempty"`
	FamilyID               string    `json:"family_id"`
	CreatedAt              time.Time `json:"created_at"`
	ExpiresAt              time.Time `json:"expires_at"`
	LastUsedAt             time.Time `json:"last_used_at"`
	IP                     string    `json:"ip"`
	UserAgent              string    `json:"user_agent"`
}

const maxPrevRefreshTokens = 3

func MaxPrevRefreshTokens() int { return maxPrevRefreshTokens }

type SessionStore interface {
	CreateSession(userID uuid.UUID, ip, userAgent string) (*SessionRecord, error)
	GetSession(id string) (*SessionRecord, error)
	GetSessionByRefreshToken(token string) (*SessionRecord, error)
	RefreshSession(sessionID string, ip, userAgent string) (*SessionRecord, error)
	DeleteSession(id string) error
	DeleteAllUserSessions(userID uuid.UUID) error
	// DeleteExpired removes sessions past their idle or absolute-max lifetime.
	// Valkey-backed stores self-expire via TTL and return 0; the PG store
	// hard-deletes reaped rows. Used by the background session reaper.
	DeleteExpired(ctx context.Context) (int, error)
}

type pgSessionStore struct {
	pgStoreBase
	sessionExpiry      time.Duration
	sessionMaxLifetime time.Duration
}

func newPGSessionStore(client *ent.Client, expiry, maxLifetime time.Duration) SessionStore {
	return &pgSessionStore{
		pgStoreBase:        pgStoreBase{client: client},
		sessionExpiry:      expiry,
		sessionMaxLifetime: maxLifetime,
	}
}

// isWithinAbsoluteLifetime reports whether a session is still within its
// absolute (max) lifetime cap (ASVS V3.2/V3.3). A zero maxLifetime disables the
// check (used by mocks/tests that do not configure it).
func isWithinAbsoluteLifetime(rec *SessionRecord, maxLifetime time.Duration) bool {
	if maxLifetime <= 0 || rec == nil {
		return true
	}
	return time.Now().Before(rec.CreatedAt.Add(maxLifetime))
}

func hashSessionToken(token string) string {
	return crypto.Default().HMACString(token)
}

func pgSessionToRecord(e *ent.Session) *SessionRecord {
	return &SessionRecord{
		IDHash:                 e.IDHash,
		UserID:                 e.UserID,
		RefreshTokenHash:       e.RefreshTokenHash,
		PrevRefreshTokenHashes: e.PrevRefreshTokenHashes,
		FamilyID:               e.FamilyID,
		CreatedAt:              e.CreatedAt,
		ExpiresAt:              e.ExpiresAt,
		LastUsedAt:             e.LastUsedAt,
		IP:                     e.IP,
		UserAgent:              e.UserAgent,
	}
}

func (s *pgSessionStore) CreateSession(userID uuid.UUID, ip, userAgent string) (*SessionRecord, error) {
	sessionID, err := generateSecureToken(32)
	if err != nil {
		return nil, err
	}
	refreshToken, err := generateSecureToken(32)
	if err != nil {
		return nil, err
	}
	familyID, err := generateSecureToken(16)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	idHash := hashSessionToken(sessionID)
	rtHash := hashSessionToken(refreshToken)

	ctx, cancel := pgctx(context.Background())
	defer cancel()

	_, err = s.client.Session.Create().
		SetUserID(userID).
		SetIDHash(idHash).
		SetRefreshTokenHash(rtHash).
		SetFamilyID(familyID).
		SetCreatedAt(now).
		SetExpiresAt(now.Add(s.sessionExpiry)).
		SetLastUsedAt(now).
		SetIP(ip).
		SetUserAgent(userAgent).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	record := &SessionRecord{
		ID:               sessionID,
		IDHash:           idHash,
		UserID:           userID,
		RefreshToken:     refreshToken,
		RefreshTokenHash: rtHash,
		FamilyID:         familyID,
		CreatedAt:        now,
		ExpiresAt:        now.Add(s.sessionExpiry),
		LastUsedAt:       now,
		IP:               ip,
		UserAgent:        userAgent,
	}
	return record, nil
}

func (s *pgSessionStore) GetSession(id string) (*SessionRecord, error) {
	if id == "" {
		return nil, nil
	}
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	e, err := s.client.Session.Query().
		Where(
			session.IDHash(hashSessionToken(id)),
			session.ExpiresAtGT(time.Now()),
		).
		Only(ctx)
	if err != nil {
		return handleQueryErr[*SessionRecord](err, "session")
	}
	rec := pgSessionToRecord(e)
	// Absolute (max) lifetime cap: a session older than the max window is
	// treated as expired even if it was just refreshed (ASVS V3.2/V3.3).
	if !isWithinAbsoluteLifetime(rec, s.sessionMaxLifetime) {
		_ = s.DeleteSession(id)
		return nil, nil
	}
	rec.ID = id
	return rec, nil
}

func (s *pgSessionStore) GetSessionByRefreshToken(token string) (*SessionRecord, error) {
	if token == "" {
		return nil, nil
	}
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	hash := hashSessionToken(token)
	e, err := s.client.Session.Query().
		Where(
			session.RefreshTokenHash(hash),
			session.ExpiresAtGT(time.Now()),
		).
		Only(ctx)
	if err != nil {
		return handleQueryErr[*SessionRecord](err, "session")
	}
	rec := pgSessionToRecord(e)
	if !isWithinAbsoluteLifetime(rec, s.sessionMaxLifetime) {
		// Expired by absolute cap; delete the row so it cannot be reused.
		_, _ = s.client.Session.Delete().Where(session.IDHash(rec.IDHash)).Exec(ctx)
		return nil, nil
	}
	return rec, nil
}

func (s *pgSessionStore) RefreshSession(sessionID string, ip, userAgent string) (*SessionRecord, error) {
	if sessionID == "" {
		return nil, nil
	}
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	idHash := hashSessionToken(sessionID)
	current, err := s.client.Session.Query().
		Where(session.IDHash(idHash)).
		Only(ctx)
	if err != nil {
		return handleQueryErr[*SessionRecord](err, "session")
	}

	// Absolute (max) lifetime cap: never refresh a session past its max age.
	nowRec := pgSessionToRecord(current)
	if !isWithinAbsoluteLifetime(nowRec, s.sessionMaxLifetime) {
		_, _ = s.client.Session.Delete().Where(session.IDHash(idHash)).Exec(ctx)
		return nil, nil
	}

	// Rotate the session ID itself (not just the refresh token) so a stolen
	// cookie is invalidated at the next refresh (ASVS V3.3). The plaintext
	// new ID is returned to the caller to set as a fresh cookie; the old hash
	// is removed so the old cookie no longer authenticates.
	newSessionID, err := generateSecureToken(32)
	if err != nil {
		return nil, err
	}
	newIDHash := hashSessionToken(newSessionID)

	newRefreshToken, err := generateSecureToken(32)
	if err != nil {
		return nil, err
	}
	newHash := hashSessionToken(newRefreshToken)

	prev := append([]string{current.RefreshTokenHash}, current.PrevRefreshTokenHashes...)
	if len(prev) > maxPrevRefreshTokens {
		prev = prev[:maxPrevRefreshTokens]
	}

	now := time.Now()
	newExpiry := now.Add(s.sessionExpiry)

	// Update the existing row in place to the new ID hash + rotated tokens.
	// CreatedAt is intentionally preserved so the absolute lifetime cap keeps
	// counting from the original login.
	_, err = current.Update().
		SetIDHash(newIDHash).
		SetRefreshTokenHash(newHash).
		SetPrevRefreshTokenHashes(prev).
		SetExpiresAt(newExpiry).
		SetLastUsedAt(now).
		SetIP(ip).
		SetUserAgent(userAgent).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh session: %w", err)
	}

	rec := pgSessionToRecord(current)
	rec.ID = newSessionID
	rec.IDHash = newIDHash
	rec.RefreshToken = newRefreshToken
	rec.RefreshTokenHash = newHash
	rec.PrevRefreshTokenHashes = prev
	rec.ExpiresAt = newExpiry
	rec.LastUsedAt = now
	rec.IP = ip
	rec.UserAgent = userAgent
	return rec, nil
}

func (s *pgSessionStore) DeleteSession(id string) error {
	if id == "" {
		return nil
	}
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	n, err := s.client.Session.Delete().
		Where(session.IDHash(hashSessionToken(id))).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	if n == 0 {
		return nil
	}
	return nil
}

func (s *pgSessionStore) DeleteAllUserSessions(userID uuid.UUID) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	_, err := s.client.Session.Delete().
		Where(session.UserID(userID)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete user sessions: %w", err)
	}
	return nil
}

// DeleteExpired hard-deletes sessions past their idle expiry OR their absolute
// (max) lifetime. Valkey sessions self-expire via TTL, so this is only needed
// for the PostgreSQL path; it keeps the sessions table from growing unbounded.
func (s *pgSessionStore) DeleteExpired(ctx context.Context) (int, error) {
	now := time.Now()
	pred := session.ExpiresAtLT(now)
	if s.sessionMaxLifetime > 0 {
		// Also reap sessions past their absolute max age even if a recent
		// refresh pushed expires_at forward.
		pred = session.Or(pred, session.CreatedAtLT(now.Add(-s.sessionMaxLifetime)))
	}
	n, err := s.client.Session.Delete().Where(pred).Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired sessions: %w", err)
	}
	return n, nil
}
