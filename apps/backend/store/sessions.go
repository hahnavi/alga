package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"alga/crypto"
	"alga/db/models"
)

type SessionRecord struct {
	ID                     string    `json:"-"`
	IDHash                 string    `json:"id"`
	UserID                 uuid.UUID `json:"user_id"`
	RefreshToken           string    `json:"-"`
	RefreshTokenHash       string    `json:"refresh_token,omitempty"`
	PrevRefreshTokenHashes []string  `json:"prev_refresh_tokens,omitempty"`
	PrevSessionIDHashes    []string  `json:"prev_session_ids,omitempty"`
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
	// RefreshSessionByRefreshToken rotates the session identified by a
	// presented refresh token (API clients that hold the token rather than the
	// session cookie). When the token matches a previously-rotated hash of a
	// live session it returns the owning session together with
	// ErrRefreshTokenReused so the caller can revoke the family.
	RefreshSessionByRefreshToken(token string, ip, userAgent string) (*SessionRecord, error)
	// FindRotatedOutSession resolves a presented session ID against the
	// bounded history of recently rotated-out ID hashes, returning the owning
	// session (UserID populated) while its family is still live. Replayed
	// cookies detect replay this way; unknown IDs return (nil, nil).
	FindRotatedOutSession(id string) (*SessionRecord, error)
	DeleteSession(id string) error
	// DeleteSessionByIDHash deletes a session by its persisted id digest —
	// used by the self-service session API, which only ever sees digests.
	DeleteSessionByIDHash(idHash string) error
	DeleteAllUserSessions(userID uuid.UUID) error
	// ListUserSessions returns the user's active (unexpired) sessions, most
	// recently used first. Records carry id digests only — plaintext session
	// IDs are never persisted, so they cannot appear here.
	ListUserSessions(userID uuid.UUID) ([]SessionRecord, error)
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

func newPGSessionStore(db *bun.DB, expiry, maxLifetime time.Duration) SessionStore {
	return &pgSessionStore{
		pgStoreBase:        pgStoreBase{db: db},
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

func pgSessionToRecord(e *models.Session) *SessionRecord {
	return &SessionRecord{
		IDHash:                 e.IDHash,
		UserID:                 e.UserID,
		RefreshTokenHash:       e.RefreshTokenHash,
		PrevRefreshTokenHashes: e.PrevRefreshTokenHashes,
		PrevSessionIDHashes:    e.PrevSessionIDHashes,
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

	m := &models.Session{
		ID:               models.NewUUID(),
		UserID:           userID,
		IDHash:           idHash,
		RefreshTokenHash: rtHash,
		FamilyID:         familyID,
		CreatedAt:        now,
		ExpiresAt:        now.Add(s.sessionExpiry),
		LastUsedAt:       now,
		IP:               ip,
		UserAgent:        userAgent,
	}

	_, err = s.db.NewInsert().Model(m).Exec(ctx)
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

	e := new(models.Session)
	err := s.db.NewSelect().Model(e).
		Where("id_hash = ?", hashSessionToken(id)).
		Where("expires_at > ?", time.Now()).
		Scan(ctx)
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
	e := new(models.Session)
	err := s.db.NewSelect().Model(e).
		Where("refresh_token_hash = ?", hash).
		Where("expires_at > ?", time.Now()).
		Scan(ctx)
	if err != nil {
		return handleQueryErr[*SessionRecord](err, "session")
	}
	rec := pgSessionToRecord(e)
	if !isWithinAbsoluteLifetime(rec, s.sessionMaxLifetime) {
		// Expired by absolute cap; delete the row so it cannot be reused.
		_, _ = s.db.NewDelete().Model((*models.Session)(nil)).Where("id_hash = ?", rec.IDHash).Exec(ctx)
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
	current := new(models.Session)
	err := s.db.NewSelect().Model(current).
		Where("id_hash = ?", idHash).
		Scan(ctx)
	if err != nil {
		return handleQueryErr[*SessionRecord](err, "session")
	}

	return s.rotateSessionRow(ctx, current, ip, userAgent)
}

func (s *pgSessionStore) RefreshSessionByRefreshToken(token string, ip, userAgent string) (*SessionRecord, error) {
	if token == "" {
		return nil, nil
	}
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	rtHash := hashSessionToken(token)
	current := new(models.Session)
	err := s.db.NewSelect().Model(current).
		Where("refresh_token_hash = ?", rtHash).
		Where("expires_at > ?", time.Now()).
		Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to query session by refresh token: %w", err)
	}
	if current.ID == uuid.Nil {
		// Not the live token; a replay resolves against the bounded
		// prev_refresh_token_hashes history of a still-live session.
		current = new(models.Session)
		err = s.db.NewSelect().Model(current).
			Where("prev_refresh_token_hashes @> ?::jsonb", marshalHashes(rtHash)).
			Where("expires_at > ?", time.Now()).
			Limit(1).
			Scan(ctx)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("failed to query session by previous refresh token: %w", err)
		}
		if current.ID == uuid.Nil {
			return nil, nil
		}
		// Replay: return the owning session so the caller revokes the family.
		return pgSessionToRecord(current), ErrRefreshTokenReused
	}

	return s.rotateSessionRow(ctx, current, ip, userAgent)
}

func (s *pgSessionStore) FindRotatedOutSession(id string) (*SessionRecord, error) {
	if id == "" {
		return nil, nil
	}
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	current := new(models.Session)
	err := s.db.NewSelect().Model(current).
		Where("prev_session_id_hashes @> ?::jsonb", marshalHashes(hashSessionToken(id))).
		Where("expires_at > ?", time.Now()).
		Limit(1).
		Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to query rotated-out session: %w", err)
	}
	if current.ID == uuid.Nil {
		return nil, nil
	}
	return pgSessionToRecord(current), nil
}

func marshalHashes(hashes ...string) string {
	data, err := json.Marshal(hashes)
	if err != nil {
		// Marshal of a []string cannot fail; kept total for the jsonb casts.
		return "[]"
	}
	return string(data)
}

// rotateSessionRow rotates BOTH the session ID and the refresh token of a
// loaded session row, preserving CreatedAt so the absolute lifetime cap keeps
// counting from the original login. The rotated-out ID hash is remembered in a
// bounded history so a replayed old cookie stays recognizable (family reuse
// detection). The plaintext new IDs are returned to the caller to set as fresh
// cookies; the old hashes only authenticate the replay-detection history.
func (s *pgSessionStore) rotateSessionRow(ctx context.Context, current *models.Session, ip, userAgent string) (*SessionRecord, error) {
	if current.ID == uuid.Nil {
		return nil, nil
	}

	// Absolute (max) lifetime cap: never refresh a session past its max age.
	if !isWithinAbsoluteLifetime(pgSessionToRecord(current), s.sessionMaxLifetime) {
		_, _ = s.db.NewDelete().Model((*models.Session)(nil)).Where("id_hash = ?", current.IDHash).Exec(ctx)
		return nil, nil
	}

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

	prevRT := append([]string{current.RefreshTokenHash}, current.PrevRefreshTokenHashes...)
	if len(prevRT) > maxPrevRefreshTokens {
		prevRT = prevRT[:maxPrevRefreshTokens]
	}
	prevIDs := append([]string{current.IDHash}, current.PrevSessionIDHashes...)
	if len(prevIDs) > maxPrevRefreshTokens {
		prevIDs = prevIDs[:maxPrevRefreshTokens]
	}

	now := time.Now()
	newExpiry := now.Add(s.sessionExpiry)

	// Update the existing row in place to the new ID hash + rotated tokens.
	_, err = s.db.NewUpdate().Model((*models.Session)(nil)).
		Set("id_hash = ?", newIDHash).
		Set("refresh_token_hash = ?", newHash).
		Set("prev_refresh_token_hashes = ?", prevRT).
		Set("prev_session_id_hashes = ?", prevIDs).
		Set("expires_at = ?", newExpiry).
		Set("last_used_at = ?", now).
		Set("ip = ?", ip).
		Set("user_agent = ?", userAgent).
		Where("id = ?", current.ID).
		Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh session: %w", err)
	}

	rec := pgSessionToRecord(current)
	rec.ID = newSessionID
	rec.IDHash = newIDHash
	rec.RefreshToken = newRefreshToken
	rec.RefreshTokenHash = newHash
	rec.PrevRefreshTokenHashes = prevRT
	rec.PrevSessionIDHashes = prevIDs
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

	_, err := s.db.NewDelete().Model((*models.Session)(nil)).
		Where("id_hash = ?", hashSessionToken(id)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

func (s *pgSessionStore) DeleteSessionByIDHash(idHash string) error {
	if idHash == "" {
		return nil
	}
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	_, err := s.db.NewDelete().Model((*models.Session)(nil)).
		Where("id_hash = ?", idHash).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

func (s *pgSessionStore) DeleteAllUserSessions(userID uuid.UUID) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	_, err := s.db.NewDelete().Model((*models.Session)(nil)).
		Where("user_id = ?", userID).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete user sessions: %w", err)
	}
	return nil
}

func (s *pgSessionStore) ListUserSessions(userID uuid.UUID) ([]SessionRecord, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var rows []models.Session
	err := s.db.NewSelect().Model(&rows).
		Where("user_id = ?", userID).
		Where("expires_at > ?", time.Now()).
		Order("last_used_at DESC").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list user sessions: %w", err)
	}

	now := time.Now()
	records := make([]SessionRecord, 0, len(rows))
	for i := range rows {
		rec := pgSessionToRecord(&rows[i])
		// Absolute-cap rows are effectively expired; GetSession would reap
		// them on use, so they are excluded from the listing as well.
		if !isWithinAbsoluteLifetime(rec, s.sessionMaxLifetime) {
			continue
		}
		if rec.ExpiresAt.Before(now) {
			continue
		}
		records = append(records, *rec)
	}
	return records, nil
}

// DeleteExpired hard-deletes sessions past their idle expiry OR their absolute
// (max) lifetime. Valkey sessions self-expire via TTL, so this is only needed
// for the PostgreSQL path; it keeps the sessions table from growing unbounded.
func (s *pgSessionStore) DeleteExpired(ctx context.Context) (int, error) {
	now := time.Now()

	q := s.db.NewDelete().Model((*models.Session)(nil)).
		Where("expires_at < ?", now)

	if s.sessionMaxLifetime > 0 {
		// Also reap sessions past their absolute max age even if a recent
		// refresh pushed expires_at forward.
		q = s.db.NewDelete().Model((*models.Session)(nil)).
			Where("(expires_at < ? OR created_at < ?)", now, now.Add(-s.sessionMaxLifetime))
	}

	res, err := q.Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired sessions: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired sessions: %w", err)
	}
	return int(n), nil
}
