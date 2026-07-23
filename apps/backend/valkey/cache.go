package valkey

import (
	"context"
	"fmt"
	"time"

	"alga/logger"

	valkeygo "github.com/valkey-io/valkey-go"
)

type valkeyDoer interface {
	get(ctx context.Context, key string) (string, error)
	set(ctx context.Context, key, value string, ttl time.Duration) error
	del(ctx context.Context, keys ...string) error
	scan(ctx context.Context, cursor uint64, match string, count int64) (valkeygo.ScanEntry, error)
}

type realValkeyDoer struct {
	client *Client
}

func (r *realValkeyDoer) get(ctx context.Context, key string) (string, error) {
	return r.client.Do(ctx, r.client.Builder().Get().Key(key).Build()).ToString()
}

func (r *realValkeyDoer) set(ctx context.Context, key, value string, ttl time.Duration) error {
	return r.client.Do(ctx, r.client.Builder().Set().Key(key).Value(value).ExSeconds(int64(ttl.Seconds())).Build()).Error()
}

func (r *realValkeyDoer) del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return r.client.Do(ctx, r.client.Builder().Del().Key(keys...).Build()).Error()
}

func (r *realValkeyDoer) scan(ctx context.Context, cursor uint64, match string, count int64) (valkeygo.ScanEntry, error) {
	return r.client.Do(ctx, r.client.Builder().Scan().Cursor(cursor).Match(match).Count(count).Build()).AsScanEntry()
}

type Cache struct {
	client valkeyDoer
}

func NewCache(client *Client) *Cache {
	if client == nil {
		return &Cache{}
	}
	return &Cache{client: &realValkeyDoer{client: client}}
}

func (c *Cache) Available() bool {
	return c != nil && c.client != nil
}

func (c *Cache) GetOrSet(ctx context.Context, key string, ttl time.Duration, fetchFn func(context.Context) ([]byte, error)) ([]byte, error) {
	if !c.Available() {
		return fetchFn(ctx)
	}

	data, err := c.client.get(ctx, key)
	if err == nil && data != "" {
		return []byte(data), nil
	}

	result, err := fetchFn(ctx)
	if err != nil {
		return nil, err
	}

	if err := c.client.set(ctx, key, string(result), ttl); err != nil {
		logger.Warn("cache: failed to store value", "key", key, "error", err)
	}

	return result, nil
}

func (c *Cache) Invalidate(ctx context.Context, key string) error {
	if !c.Available() {
		return nil
	}
	return c.client.del(ctx, key)
}

func (c *Cache) InvalidatePrefix(ctx context.Context, prefix string) error {
	if !c.Available() {
		return nil
	}

	var cursor uint64
	for {
		entry, err := c.client.scan(ctx, cursor, prefix+"*", 100)
		if err != nil {
			return fmt.Errorf("cache: scan prefix %q: %w", prefix, err)
		}
		if len(entry.Elements) > 0 {
			if err := c.client.del(ctx, entry.Elements...); err != nil {
				logger.Warn("cache: failed to delete prefix keys", "prefix", prefix, "error", err)
			}
		}
		cursor = entry.Cursor
		if cursor == 0 {
			break
		}
	}
	return nil
}
