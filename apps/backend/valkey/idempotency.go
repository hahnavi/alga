package valkey

import (
	"context"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
)

// idempotencyKeyPrefix namespaces all Idempotency-Key replay records so they
// never collide with other Valkey data. The middleware supplies an already
// scoped+hashed logical key; this prefix keeps the physical keyspace tidy.
const idempotencyKeyPrefix = "alga:idempotency:"

// IdempotencyCache stores and replays cached HTTP responses for retry-safe
// writes keyed by the client-supplied Idempotency-Key. It implements the
// platform.IdempotencyStore interface without importing package platform
// (structural typing), so there is no import cycle.
type IdempotencyCache struct {
	client *Client
}

// NewIdempotencyCache returns a cache backed by the given Valkey client, or nil
// when the client is nil so callers can treat idempotency as a no-op when
// Valkey is not configured.
func NewIdempotencyCache(client *Client) *IdempotencyCache {
	if client == nil {
		return nil
	}
	return &IdempotencyCache{client: client}
}

// Get returns the cached record for key. found is false (with a nil error) when
// the key is absent or expired.
func (c *IdempotencyCache) Get(ctx context.Context, key string) (string, bool, error) {
	val, err := c.client.Do(ctx, c.client.Builder().Get().Key(idempotencyKeyPrefix+key).Build()).ToString()
	if err != nil {
		if err == valkeygo.Nil {
			return "", false, nil
		}
		return "", false, err
	}
	return val, true, nil
}

// Set stores value under key with the given TTL. A non-positive TTL falls back
// to 24h so a misconfiguration never persists records indefinitely.
func (c *IdempotencyCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return c.client.Do(ctx, c.client.Builder().Set().Key(idempotencyKeyPrefix+key).
		Value(value).ExSeconds(int64(ttl.Seconds())).Build()).Error()
}
