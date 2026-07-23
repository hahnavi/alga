package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"alga/logger"
)

// IdempotencyKeyHeader is the request header clients send to make a
// state-changing write safely retryable. When present on an opted-in route,
// the first successful response is cached and replayed for later requests
// carrying the same value.
const IdempotencyKeyHeader = "Idempotency-Key"

// IdempotentReplayedHeader is set to "true" on responses served from the
// idempotency cache so clients can distinguish a replay from a fresh execution.
const IdempotentReplayedHeader = "Idempotent-Replayed"

// maxIdempotencyKeyLen bounds the client-supplied key length. The value is
// hashed before use, but rejecting oversized keys early avoids buffering
// abusive input.
const maxIdempotencyKeyLen = 255

// DefaultIdempotencyTTL is the fallback replay window when a non-positive TTL
// is configured.
const DefaultIdempotencyTTL = 24 * time.Hour

// IdempotencyStore is the minimal Valkey surface WithIdempotency depends on.
// valkey.IdempotencyCache satisfies it structurally (no import cycle).
type IdempotencyStore interface {
	// Get returns the cached record and true when key is present. A miss
	// returns ("", false, nil).
	Get(ctx context.Context, key string) (value string, found bool, err error)
	// Set stores value under key with the given TTL.
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}

// idempotencyRecord is the JSON-serialized cached response. Body is the raw
// bytes the handler wrote; encoding/json base64-encodes []byte fields.
type idempotencyRecord struct {
	Status      int    `json:"status"`
	ContentType string `json:"content_type,omitempty"`
	Body        []byte `json:"body"`
}

// idempotencyCapture tees the handler's response to the real writer while
// buffering status + body so a successful result can be cached for replay.
type idempotencyCapture struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	buf         bytes.Buffer
}

func (c *idempotencyCapture) WriteHeader(status int) {
	if c.wroteHeader {
		return
	}
	c.status = status
	c.wroteHeader = true
	c.ResponseWriter.WriteHeader(status)
}

func (c *idempotencyCapture) Write(b []byte) (int, error) {
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	c.buf.Write(b)
	return c.ResponseWriter.Write(b)
}

// WithIdempotency wraps next so that a state-changing request carrying an
// Idempotency-Key header is executed at most once per key within the TTL: the
// first successful (2xx) response is cached in Valkey under a namespaced,
// hashed key and replayed verbatim on subsequent requests with the same key.
//
// It is an explicit, opt-in, per-route wrapper — never a global middleware.
// Requests that omit the header, use a non-state-changing method, or hit a nil
// store fall straight through to next, so behavior is unchanged for every
// existing client and every route the wrapper is not applied to.
//
// Only 2xx responses are cached: failures (validation, auth, conflict, 5xx)
// are never stored, so a client that retries after a transient failure still
// reaches the handler. scope namespaces keys per endpoint so the same key used
// against different endpoints never collides.
func WithIdempotency(store IdempotencyStore, ttl time.Duration, scope string, next http.HandlerFunc) http.HandlerFunc {
	if store == nil {
		return next
	}
	if ttl <= 0 {
		ttl = DefaultIdempotencyTTL
	}
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get(IdempotencyKeyHeader)
		if key == "" || !IsStateChangingMethod(r.Method) {
			next(w, r)
			return
		}
		if len(key) > maxIdempotencyKeyLen {
			WriteErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "Idempotency-Key header too long")
			return
		}

		storeKey := idempotencyStoreKey(scope, key)

		if cached, found, err := store.Get(r.Context(), storeKey); err != nil {
			// Fail open: a cache lookup failure must not block the write.
			logger.WarnCtx(r.Context(), "idempotency lookup failed; executing handler",
				"component", "idempotency", "scope", scope, "error", err)
		} else if found {
			var rec idempotencyRecord
			if err := json.Unmarshal([]byte(cached), &rec); err == nil {
				replayIdempotentResponse(w, rec)
				return
			}
			logger.WarnCtx(r.Context(), "idempotency record decode failed; executing handler",
				"component", "idempotency", "scope", scope)
		}

		capture := &idempotencyCapture{ResponseWriter: w}
		next(capture, r)

		status := capture.status
		if status == 0 {
			status = http.StatusOK
		}
		// Cache successful responses only so failures remain retryable.
		if status < http.StatusOK || status >= http.StatusMultipleChoices {
			return
		}

		payload, err := json.Marshal(idempotencyRecord{
			Status:      status,
			ContentType: capture.Header().Get("Content-Type"),
			Body:        capture.buf.Bytes(),
		})
		if err != nil {
			logger.WarnCtx(r.Context(), "idempotency record encode failed",
				"component", "idempotency", "scope", scope, "error", err)
			return
		}
		if err := store.Set(r.Context(), storeKey, string(payload), ttl); err != nil {
			logger.WarnCtx(r.Context(), "idempotency store failed",
				"component", "idempotency", "scope", scope, "error", err)
		}
	}
}

// replayIdempotentResponse writes a previously cached response to w.
func replayIdempotentResponse(w http.ResponseWriter, rec idempotencyRecord) {
	if rec.ContentType != "" {
		w.Header().Set("Content-Type", rec.ContentType)
	}
	w.Header().Set(IdempotentReplayedHeader, "true")
	status := rec.Status
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write(rec.Body)
}

// idempotencyStoreKey derives the physical logical key from scope + client key.
// Hashing keeps arbitrary client input out of the keyspace (safe key handling)
// and bounds the stored key length.
func idempotencyStoreKey(scope, key string) string {
	sum := sha256.Sum256([]byte(scope + ":" + key))
	return scope + ":" + hex.EncodeToString(sum[:])
}
