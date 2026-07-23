package valkey

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

const cancelKeyPrefix = "alga:cancel:"

func CancelKeyAlert(fingerprint string) string {
	return cancelKeyPrefix + "alert:" + fingerprint
}

func CancelKeyAlertNum(alertNumber int64) string {
	return cancelKeyPrefix + "alert_num:" + strconv.FormatInt(alertNumber, 10)
}

func CancelKeyIncident(incidentNumber int64) string {
	return cancelKeyPrefix + "incident:" + strconv.FormatInt(incidentNumber, 10)
}

func CancelKeyInvestigation(investigationID string) string {
	return cancelKeyPrefix + "investigation:" + investigationID
}

// CancelSet records deleted entity IDs in Valkey so consumers/schedulers can
// skip work for them without hitting Postgres. It is nil-safe: a nil *CancelSet
// reports unavailable and makes Add/Contains no-ops so callers fall back to the
// Postgres deleted_at/missing-row guard.
type CancelSet struct {
	client *Client
	ttl    time.Duration
}

func NewCancelSet(client *Client, ttl time.Duration) *CancelSet {
	if client == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	return &CancelSet{client: client, ttl: ttl}
}

func (c *CancelSet) Available() bool { return c != nil && c.client != nil }

// Add stores a cancel marker with the CancelSet's default TTL. Prefer
// AddWithTTL when the key is not a canonical unique identifier (e.g. an alert
// fingerprint, which can be reused across alert lifecycles).
func (c *CancelSet) Add(ctx context.Context, key string) error {
	if c == nil {
		return nil
	}
	return c.AddWithTTL(ctx, key, c.ttl)
}

// AddWithTTL stores a cancel marker with an explicit TTL. Use a short TTL for
// non-canonical keys (alert fingerprints) so a previously-resolved alert does
// not poison the cancel set for the next alert that reuses the fingerprint.
func (c *CancelSet) AddWithTTL(ctx context.Context, key string, ttl time.Duration) error {
	if !c.Available() {
		return nil
	}
	if ttl <= 0 {
		ttl = c.ttl
	}
	if err := c.client.Do(ctx, c.client.Builder().Set().
		Key(key).Value("1").
		ExSeconds(int64(ttl.Seconds())).
		Build()).Error(); err != nil {
		return fmt.Errorf("cancel set add: %w", err)
	}
	return nil
}

func (c *CancelSet) Contains(ctx context.Context, key string) bool {
	if !c.Available() {
		return false
	}
	n, err := c.client.Do(ctx, c.client.Builder().Exists().Key(key).Build()).AsInt64()
	if err != nil {
		return false
	}
	return n == 1
}
