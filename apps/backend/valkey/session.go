package valkey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/valkey-io/valkey-go"

	algacrypto "alga/crypto"
	"alga/logger"
	"alga/store"
)

// Key namespace conventions:
//
//	alga:session:<HMAC(session_id)>     -> JSON-encoded SessionRecord (no
//	                                       plaintext tokens inside).
//	alga:session:rt:<HMAC(rt)>          -> "<HMAC(session_id)>" — refresh
//	                                       token hash maps to session id hash
//	                                       so we can find the session without
//	                                       a secondary index. Rotated-out
//	                                       tokens keep their mapping for the
//	                                       sliding window so replays resolve
//	                                       to the live session and are
//	                                       detectable.
//	alga:session:prev:<HMAC(session_id)> -> "<userID>" — short-lived tombstone
//	                                       for a rotated-out session ID;
//	                                       hitting one is family replay and
//	                                       triggers full revocation.
//	alga:user_sessions:<userID>         -> set of <HMAC(session_id)> values
//	                                       so DeleteAllUserSessions stays O(N).
//
// Plaintext session IDs and refresh tokens are returned to the caller exactly
// once (at create / refresh) and never persisted in Valkey or PostgreSQL.
const (
	sessionKeyPrefix = "alga:session:"
	sessionRTPrefix  = "alga:session:rt:"
	sessionPrevID    = "alga:session:prev:"
	userSessionsKey  = "alga:user_sessions:"
)

var createSessionLua = NewLuaScript(`
local sessionKey = KEYS[1]
local rtKey = KEYS[2]
local userSetKey = KEYS[3]
local sessionData = ARGV[1]
local rtData = ARGV[2]
local userID = ARGV[3]
local sessionExpiryMs = tonumber(ARGV[4])
local setExpirySec = tonumber(ARGV[5])
local idHash = ARGV[6]

redis.call('SET', sessionKey, sessionData, 'PX', sessionExpiryMs)
redis.call('SET', rtKey, rtData, 'PX', sessionExpiryMs)
redis.call('SADD', userSetKey, idHash)
redis.call('EXPIRE', userSetKey, setExpirySec)
return 'OK'
`)

// SessionStore implements store.SessionStore backed by Valkey.
type SessionStore struct {
	client             *Client
	sessionExpiry      time.Duration
	sessionMaxLifetime time.Duration
	refreshScript      *valkey.Lua
}

// NewSessionStore creates a new Valkey-backed session store. sessionMaxLifetime
// is the absolute (max) session lifetime enforced in addition to the sliding
// idle expiry (ASVS V3.2/V3.3).
func NewSessionStore(client *Client, sessionExpiry, sessionMaxLifetime time.Duration) *SessionStore {
	return &SessionStore{
		client:             client,
		sessionExpiry:      sessionExpiry,
		sessionMaxLifetime: sessionMaxLifetime,
		refreshScript:      NewLuaScript(refreshSessionScript),
	}
}

// refreshSessionScript atomically:
//   - reads the current session document keyed by session-id hash,
//   - rotates the stored refresh-token hash to the new value,
//   - moves the prior hash onto the bounded `prev_refresh_tokens` list,
//   - re-extends the document TTL,
//   - removes the old RT mapping key and writes the new one,
//   - writes a tombstone for the rotated-out session ID so a replayed old
//     cookie is recognizable for family replay detection.
//
// All of these steps must succeed together or fail together; otherwise a
// half-applied rotation could leave a refresh token "live" in either Postgres
// or Valkey but not both.
var refreshSessionScript = `
local sessionKey = KEYS[1]
local oldRTKey = KEYS[2]
local newRTKey = KEYS[3]
local prevIDKey = KEYS[4]
local newExpiry = tonumber(ARGV[1])
local newRTHash = ARGV[2]
local now = ARGV[3]
local ip = ARGV[4]
local ua = ARGV[5]
local maxPrev = tonumber(ARGV[6])
local userID = ARGV[7]

local data = redis.call("GET", sessionKey)
if not data then
	return nil
end

local session = cjson.decode(data)
local prev = session.prev_refresh_tokens or {}
table.insert(prev, 1, session.refresh_token)
while #prev > maxPrev do
	table.remove(prev)
end

session.refresh_token = newRTHash
session.prev_refresh_tokens = prev
session.expires_at = now
session.last_used_at = now
session.ip = ip
session.user_agent = ua

local encoded = cjson.encode(session)
redis.call("SET", sessionKey, encoded, 'PX', newExpiry)
redis.call("DEL", oldRTKey)
redis.call("SET", newRTKey, sessionKey, 'PX', newExpiry)
redis.call("SET", prevIDKey, userID, 'PX', newExpiry)
return encoded
`

// CreateSession generates a new session for the given user.
func (s *SessionStore) CreateSession(userID uuid.UUID, ip, userAgent string) (*store.SessionRecord, error) {
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
	idHash := algacrypto.Default().HMACString(sessionID)
	rtHash := algacrypto.Default().HMACString(refreshToken)

	record := &store.SessionRecord{
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

	stored := *record
	stored.ID = ""
	stored.RefreshToken = ""
	data, err := json.Marshal(&stored)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	expiryMs := int64(s.sessionExpiry / time.Millisecond)
	ctx := context.Background()

	keys := []string{
		sessionKeyPrefix + idHash,
		sessionRTPrefix + rtHash,
		userSessionsKey + userID.String(),
	}
	args := []string{
		string(data),
		sessionKeyPrefix + idHash,
		userID.String(),
		strconv.FormatInt(expiryMs, 10),
		strconv.FormatInt(int64(s.sessionExpiry/time.Second), 10),
		idHash,
	}

	if err := createSessionLua.Exec(ctx, s.client.client, keys, args).Error(); err != nil {
		return nil, fmt.Errorf("failed to store session atomically: %w", err)
	}

	return record, nil
}

// GetSession retrieves a session by the plaintext cookie value, hashing it
// before the lookup.
func (s *SessionStore) GetSession(id string) (*store.SessionRecord, error) {
	if id == "" {
		return nil, nil
	}
	ctx := context.Background()
	idHash := algacrypto.Default().HMACString(id)
	val, err := s.client.Do(ctx, s.client.Builder().Get().Key(sessionKeyPrefix+idHash).Build()).ToString()
	if err != nil {
		if isValkeyNil(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	rec, err := unmarshalSession(val)
	if err != nil {
		return nil, err
	}
	// Absolute (max) lifetime cap (ASVS V3.2/V3.3). The Valkey TTL already
	// covers the sliding idle expiry; this enforces the absolute ceiling.
	if !withinAbsoluteLifetime(rec, s.sessionMaxLifetime) {
		_ = s.DeleteSession(id)
		return nil, nil
	}
	return rec, nil
}

// GetSessionByRefreshToken resolves a refresh token back to its session.
func (s *SessionStore) GetSessionByRefreshToken(token string) (*store.SessionRecord, error) {
	if token == "" {
		return nil, nil
	}
	ctx := context.Background()
	rtHash := algacrypto.Default().HMACString(token)
	sessionKey, err := s.client.Do(ctx, s.client.Builder().Get().Key(sessionRTPrefix+rtHash).Build()).ToString()
	if err != nil {
		if isValkeyNil(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get session by refresh token: %w", err)
	}
	val, err := s.client.Do(ctx, s.client.Builder().Get().Key(sessionKey).Build()).ToString()
	if err != nil {
		if isValkeyNil(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load session: %w", err)
	}
	rec, err := unmarshalSession(val)
	if err != nil {
		return nil, err
	}
	if !withinAbsoluteLifetime(rec, s.sessionMaxLifetime) {
		_ = s.DeleteSession(rec.ID)
		return nil, nil
	}
	return rec, nil
}

// withinAbsoluteLifetime reports whether a session is still within its absolute
// (max) lifetime cap (ASVS V3.2/V3.3). A zero maxLifetime disables the check.
func withinAbsoluteLifetime(rec *store.SessionRecord, maxLifetime time.Duration) bool {
	if maxLifetime <= 0 || rec == nil {
		return true
	}
	return time.Now().Before(rec.CreatedAt.Add(maxLifetime))
}

// RefreshSession rotates BOTH the session ID and the refresh token and extends
// the sliding expiry, while preserving CreatedAt (absolute lifetime cap). The
// old session cookie is invalidated; the caller sets a fresh cookie with the
// returned ID. ASVS V3.3.
func (s *SessionStore) RefreshSession(sessionID string, ip, userAgent string) (*store.SessionRecord, error) {
	if sessionID == "" {
		return nil, nil
	}
	current, err := s.GetSession(sessionID)
	if err != nil || current == nil {
		return nil, err
	}
	// GetSession already enforces the absolute-lifetime cap, so reaching here
	// means the session is within its max window.
	oldIDHash := algacrypto.Default().HMACString(sessionID)
	return s.rotateByIDHash(oldIDHash, current, ip, userAgent)
}

// RefreshSessionByRefreshToken rotates the session identified by a presented
// refresh token. A token matching a recorded prev_refresh_tokens entry of a
// live session is replay: the owning session is returned together with
// store.ErrRefreshTokenReused so the caller revokes the family.
func (s *SessionStore) RefreshSessionByRefreshToken(token string, ip, userAgent string) (*store.SessionRecord, error) {
	if token == "" {
		return nil, nil
	}
	ctx := context.Background()
	rtHash := algacrypto.Default().HMACString(token)
	sessionKey, err := s.client.Do(ctx, s.client.Builder().Get().Key(sessionRTPrefix+rtHash).Build()).ToString()
	if err != nil {
		if isValkeyNil(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get session by refresh token: %w", err)
	}
	val, err := s.client.Do(ctx, s.client.Builder().Get().Key(sessionKey).Build()).ToString()
	if err != nil {
		if isValkeyNil(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load session: %w", err)
	}
	rec, err := unmarshalSession(val)
	if err != nil {
		return nil, err
	}
	if !withinAbsoluteLifetime(rec, s.sessionMaxLifetime) {
		_ = s.DeleteSession(rec.ID)
		return nil, nil
	}
	if !algacrypto.ConstantTimeEqualString(rec.RefreshTokenHash, rtHash) {
		// The mapping key resolved, but the presented token is a previously
		// rotated-out hash: replay.
		return rec, store.ErrRefreshTokenReused
	}
	return s.rotateByIDHash(strings.TrimPrefix(sessionKey, sessionKeyPrefix), rec, ip, userAgent)
}

// FindRotatedOutSession resolves a presented session ID against the
// rotated-out ID tombstones written by the refresh script, returning the
// owning session (UserID populated) while the tombstone lives. Unknown IDs
// return (nil, nil).
func (s *SessionStore) FindRotatedOutSession(id string) (*store.SessionRecord, error) {
	if id == "" {
		return nil, nil
	}
	ctx := context.Background()
	idHash := algacrypto.Default().HMACString(id)
	userIDStr, err := s.client.Do(ctx, s.client.Builder().Get().Key(sessionPrevID+idHash).Build()).ToString()
	if err != nil {
		if isValkeyNil(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to probe rotated-out session: %w", err)
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid rotated-session tombstone: %w", err)
	}
	return &store.SessionRecord{IDHash: idHash, UserID: userID}, nil
}

// rotateByIDHash performs the rotation for the session document stored under
// sessionKeyPrefix+oldIDHash. The Lua script rotates the refresh token,
// records the bounded prev history, and writes the rotated-out ID tombstone
// atomically; the caller then rekeys the document to the new session-id hash.
func (s *SessionStore) rotateByIDHash(oldIDHash string, current *store.SessionRecord, ip, userAgent string) (*store.SessionRecord, error) {
	newRefreshToken, err := generateSecureToken(32)
	if err != nil {
		return nil, err
	}
	newRTHash := algacrypto.Default().HMACString(newRefreshToken)
	// Rotate the session ID itself so the old cookie is invalidated.
	newSessionID, err := generateSecureToken(32)
	if err != nil {
		return nil, err
	}
	newIDHash := algacrypto.Default().HMACString(newSessionID)

	ctx := context.Background()
	expiryMs := int64(s.sessionExpiry / time.Millisecond)
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)

	// Run the refresh-token rotation on the existing key first (atomic Lua).
	keys := []string{
		sessionKeyPrefix + oldIDHash,
		sessionRTPrefix + current.RefreshTokenHash,
		sessionRTPrefix + newRTHash,
		sessionPrevID + oldIDHash,
	}
	args := []string{
		strconv.FormatInt(expiryMs, 10),
		newRTHash,
		nowStr,
		ip,
		userAgent,
		strconv.Itoa(store.MaxPrevRefreshTokens()),
		current.UserID.String(),
	}

	result, err := s.refreshScript.Exec(ctx, s.client.client, keys, args).ToString()
	if err != nil {
		if isValkeyNil(err) {
			return nil, nil
		}
		logger.Warn("Lua refresh script failed, falling back to non-atomic rotation", "component", "valkey", "error", err)
		return s.refreshSessionFallback(oldIDHash, current, newRefreshToken, newRTHash, ip, userAgent)
	}

	rec, err := unmarshalSession(result)
	if err != nil {
		return nil, err
	}
	// rec still carries the old idHash key. Rekey to the new session-id hash so
	// the old cookie no longer authenticates, keeping CreatedAt intact.
	stored := *rec
	stored.ID = ""
	stored.RefreshToken = ""
	data, err := json.Marshal(&stored)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal refreshed session: %w", err)
	}
	// Write the new key, then delete the old key, repoint the new RT mapping
	// at the new session key, and keep the rotated-out RT mapping resolvable
	// so a replayed token can be recognized against the prev history.
	if err := s.client.Do(ctx, s.client.Builder().Set().Key(sessionKeyPrefix+newIDHash).Value(string(data)).PxMilliseconds(expiryMs).Build()).Error(); err != nil {
		return nil, fmt.Errorf("failed to write rekeyed session: %w", err)
	}
	s.client.Do(ctx, s.client.Builder().Del().Key(sessionKeyPrefix+oldIDHash).Build())
	s.client.Do(ctx, s.client.Builder().Set().Key(sessionRTPrefix+newRTHash).Value(sessionKeyPrefix+newIDHash).PxMilliseconds(expiryMs).Build())
	s.client.Do(ctx, s.client.Builder().Set().Key(sessionRTPrefix+rec.RefreshTokenHash).Value(sessionKeyPrefix+newIDHash).PxMilliseconds(expiryMs).Build())

	rec.ID = newSessionID
	rec.IDHash = newIDHash
	rec.RefreshToken = newRefreshToken
	rec.RefreshTokenHash = newRTHash

	s.removeUserSession(ctx, rec.UserID.String(), oldIDHash)
	s.addUserSession(ctx, rec.UserID.String(), newIDHash, expiryMs)

	return rec, nil
}

// DeleteSession removes a session by ID.
func (s *SessionStore) DeleteSession(id string) error {
	if id == "" {
		return nil
	}
	ctx := context.Background()
	idHash := algacrypto.Default().HMACString(id)
	val, err := s.client.Do(ctx, s.client.Builder().Get().Key(sessionKeyPrefix+idHash).Build()).ToString()
	if err != nil || val == "" {
		return nil
	}
	session, err := unmarshalSession(val)
	if err != nil || session == nil {
		return nil
	}

	if err := s.client.Do(ctx, s.client.Builder().Del().Key(sessionKeyPrefix+idHash).Build()).Error(); err != nil {
		logger.Warn("failed to delete session key", "error", err)
	}
	if err := s.client.Do(ctx, s.client.Builder().Del().Key(sessionRTPrefix+session.RefreshTokenHash).Build()).Error(); err != nil {
		logger.Warn("failed to delete refresh token key", "error", err)
	}
	for _, prev := range session.PrevRefreshTokenHashes {
		if err := s.client.Do(ctx, s.client.Builder().Del().Key(sessionRTPrefix+prev).Build()).Error(); err != nil {
			logger.Warn("failed to delete prev refresh token key", "error", err)
		}
	}
	s.removeUserSession(ctx, session.UserID.String(), idHash)
	return nil
}

// DeleteAllUserSessions removes all sessions for a user.
func (s *SessionStore) DeleteAllUserSessions(userID uuid.UUID) error {
	ctx := context.Background()
	userKey := userSessionsKey + userID.String()

	idHashes, err := s.client.Do(ctx, s.client.Builder().Smembers().Key(userKey).Build()).AsStrSlice()
	if err != nil {
		logger.Error("failed to list user sessions for deletion", "component", "valkey", "user_id", userID.String(), "error", err)
		return fmt.Errorf("failed to get user sessions: %w", err)
	}
	for _, h := range idHashes {
		val, err := s.client.Do(ctx, s.client.Builder().Get().Key(sessionKeyPrefix+h).Build()).ToString()
		if err == nil {
			if rec, err := unmarshalSession(val); err == nil {
				s.client.Do(ctx, s.client.Builder().Del().Key(sessionRTPrefix+rec.RefreshTokenHash).Build())
				for _, prev := range rec.PrevRefreshTokenHashes {
					s.client.Do(ctx, s.client.Builder().Del().Key(sessionRTPrefix+prev).Build())
				}
			}
		}
		s.client.Do(ctx, s.client.Builder().Del().Key(sessionKeyPrefix+h).Build())
	}
	s.client.Do(ctx, s.client.Builder().Del().Key(userKey).Build())
	return nil
}

// DeleteExpired is a no-op for the Valkey-backed store: session keys carry a
// TTL (PX) that expires them automatically, and the absolute-lifetime cap is
// enforced on read. It satisfies the SessionStore interface for the reaper.
func (s *SessionStore) DeleteExpired(_ context.Context) (int, error) {
	return 0, nil
}

func (s *SessionStore) refreshSessionFallback(idHash string, current *store.SessionRecord, newRT, newRTHash, ip, userAgent string) (*store.SessionRecord, error) {
	now := time.Now()
	newExpiry := now.Add(s.sessionExpiry)

	prev := append([]string{current.RefreshTokenHash}, current.PrevRefreshTokenHashes...)
	if len(prev) > store.MaxPrevRefreshTokens() {
		prev = prev[:store.MaxPrevRefreshTokens()]
	}
	current.RefreshToken = newRT
	current.RefreshTokenHash = newRTHash
	current.PrevRefreshTokenHashes = prev
	current.ExpiresAt = newExpiry
	current.LastUsedAt = now
	current.IP = ip
	current.UserAgent = userAgent

	stored := *current
	stored.ID = ""
	stored.RefreshToken = ""
	data, err := json.Marshal(&stored)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	ctx := context.Background()
	expiryMs := int64(s.sessionExpiry / time.Millisecond)

	if err := s.client.Do(ctx, s.client.Builder().Set().Key(sessionKeyPrefix+idHash).Value(string(data)).PxMilliseconds(expiryMs).Build()).Error(); err != nil {
		return nil, fmt.Errorf("failed to update session: %w", err)
	}
	if len(prev) > 0 {
		// Keep the rotated-out token resolvable so a replayed token is
		// recognizable against the prev history (replay detection).
		s.client.Do(ctx, s.client.Builder().Set().Key(sessionRTPrefix+prev[0]).Value(sessionKeyPrefix+idHash).PxMilliseconds(expiryMs).Build())
	}
	s.client.Do(ctx, s.client.Builder().Set().Key(sessionRTPrefix+newRTHash).Value(sessionKeyPrefix+idHash).PxMilliseconds(expiryMs).Build())
	return current, nil
}

func (s *SessionStore) addUserSession(ctx context.Context, userID, idHash string, expiryMs int64) {
	s.client.Do(ctx, s.client.Builder().Sadd().Key(userSessionsKey+userID).Member(idHash).Build())
	s.client.Do(ctx, s.client.Builder().Expire().Key(userSessionsKey+userID).Seconds(int64(s.sessionExpiry/time.Second)).Build())
}

func (s *SessionStore) removeUserSession(ctx context.Context, userID, idHash string) {
	s.client.Do(ctx, s.client.Builder().Srem().Key(userSessionsKey+userID).Member(idHash).Build())
}

func unmarshalSession(data string) (*store.SessionRecord, error) {
	var record store.SessionRecord
	if err := json.Unmarshal([]byte(data), &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}
	return &record, nil
}

func isValkeyNil(err error) bool {
	return errors.Is(err, valkey.Nil)
}
