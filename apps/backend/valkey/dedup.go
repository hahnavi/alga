package valkey

import (
	"context"
	"time"
)

const dedupKeyPrefix = "alga:dedup:"

// DedupCache provides alert deduplication via Valkey key existence checks.
type DedupCache struct {
	client *Client
	ttl    time.Duration
}

// NewDedupCache creates a new dedup cache backed by Valkey.
func NewDedupCache(client *Client) *DedupCache {
	return &DedupCache{
		client: client,
		ttl:    time.Hour,
	}
}

// IsDuplicate returns true if the fingerprint is already tracked.
func (d *DedupCache) IsDuplicate(ctx context.Context, fingerprint string) bool {
	val, err := d.client.Do(ctx, d.client.Builder().Exists().Key(dedupKeyPrefix+fingerprint).Build()).AsInt64()
	if err != nil {
		return false // On error, treat as not duplicate
	}
	return val == 1
}

// MarkTracked records a fingerprint as tracked with a TTL.
func (d *DedupCache) MarkTracked(ctx context.Context, fingerprint string) error {
	return d.client.Do(ctx, d.client.Builder().Set().Key(dedupKeyPrefix+fingerprint).Value("1").ExSeconds(int64(d.ttl.Seconds())).Build()).Error()
}

// RemoveTracking removes the dedup tracking for a fingerprint.
func (d *DedupCache) RemoveTracking(ctx context.Context, fingerprint string) {
	d.client.Do(ctx, d.client.Builder().Del().Key(dedupKeyPrefix+fingerprint).Build())
}
