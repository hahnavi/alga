package valkey

import (
	"context"
	"fmt"
	"time"

	"github.com/valkey-io/valkey-go"

	"alga/logger"
)

// Client wraps a valkey-go client for Valkey/Redis operations.
type Client struct {
	client valkey.Client
}

// NewClient creates a new Valkey client. Returns nil if addr is empty.
func NewClient(addr, password string, db int) (*Client, error) {
	if addr == "" {
		return nil, nil
	}

	opts := valkey.ClientOption{
		InitAddress: []string{addr},
		SelectDB:    db,
	}
	if password != "" {
		opts.Password = password
	}

	client, err := valkey.NewClient(opts)
	if err != nil {
		logger.Error("failed to connect to Valkey", "component", "valkey", "addr", addr, "error", err)
		return nil, fmt.Errorf("failed to create valkey client: %w", err)
	}

	logger.Info("connected to Valkey", "component", "valkey", "addr", addr)
	return &Client{client: client}, nil
}

// Ping checks connectivity to Valkey.
func (c *Client) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.client.Do(ctx, c.client.B().Ping().Build()).Error()
}

// Close releases Valkey connection resources.
func (c *Client) Close() {
	logger.Debug("closing Valkey connection", "component", "valkey")
	c.client.Close()
}

// Do executes a single Valkey command.
func (c *Client) Do(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
	return c.client.Do(ctx, cmd)
}

// Builder returns the command builder for constructing Valkey commands.
func (c *Client) Builder() valkey.Builder {
	return c.client.B()
}

// Dedicated returns a dedicated connection for pub/sub or other blocking operations.
func (c *Client) Dedicated() (valkey.DedicatedClient, func()) {
	return c.client.Dedicate()
}

// Receive subscribes to channels and calls the handler for each message.
func (c *Client) Receive(ctx context.Context, cmd valkey.Completed, handler func(valkey.PubSubMessage)) error {
	return c.client.Receive(ctx, cmd, handler)
}

// Client returns the underlying valkey-go client for use by other packages.
func (c *Client) Client() valkey.Client {
	return c.client
}

// NewLuaScript creates a new Lua script that can be executed atomically.
func NewLuaScript(src string) *valkey.Lua {
	return valkey.NewLuaScript(src)
}

// SetNX is the canonical "set-if-not-exists with TTL" used by idempotency
// guards (correlator flush lock, investigate worker dedupe, etc). Returns
// true when the key was set, false when it already existed.
func (c *Client) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	if ttl <= 0 {
		ttl = 1 * time.Second
	}
	got, err := c.client.Do(ctx, c.client.B().Set().Key(key).Value(value).
		Nx().ExSeconds(int64(ttl.Seconds())).Build()).ToString()
	if err != nil {
		if err == valkey.Nil {
			return false, nil
		}
		return false, err
	}
	return got == "OK", nil
}

// Del removes one or more keys. Errors are bubbled so callers can decide
// whether to log or ignore.
func (c *Client) Del(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return c.client.Do(ctx, c.client.B().Del().Key(keys...).Build()).Error()
}

// HSet sets a field in a hash to the given value.
func (c *Client) HSet(ctx context.Context, key, field, value string) error {
	return c.client.Do(ctx, c.client.B().Hset().Key(key).FieldValue().
		FieldValue(field, value).Build()).Error()
}

func (c *Client) HMSet(ctx context.Context, key string, fields map[string]string) error {
	for f, v := range fields {
		if err := c.HSet(ctx, key, f, v); err != nil {
			return err
		}
	}
	return nil
}

// HGet gets a field from a hash. Returns empty string if field doesn't exist.
func (c *Client) HGet(ctx context.Context, key, field string) (string, error) {
	return c.client.Do(ctx, c.client.B().Hget().Key(key).Field(field).Build()).ToString()
}

// ZAdd adds a member to a sorted set with the given score.
func (c *Client) ZAdd(ctx context.Context, key string, score float64, member string) error {
	return c.client.Do(ctx, c.client.B().Zadd().Key(key).ScoreMember().
		ScoreMember(score, member).Build()).Error()
}

// ZRem removes a member from a sorted set.
func (c *Client) ZRem(ctx context.Context, key, member string) error {
	return c.client.Do(ctx, c.client.B().Zrem().Key(key).Member(member).Build()).Error()
}

// ZRangeByScore returns members with scores between min and max (inclusive).
// Uses "-inf" for negative infinity and Unix timestamp for max.
func (c *Client) ZRangeByScore(ctx context.Context, key string, min, max float64) ([]string, error) {
	minStr := fmt.Sprintf("%.0f", min)
	maxStr := fmt.Sprintf("%.0f", max)
	if min <= 0 {
		minStr = "-inf"
	}
	msgs := c.client.Do(ctx, c.client.B().Zrange().Key(key).Min(minStr).Max(maxStr).Byscore().Build())
	result, err := msgs.AsStrSlice()
	if err != nil {
		return nil, err
	}
	return result, nil
}
