package valkey

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/valkey-io/valkey-go"

	"alga/logger"
)

// LeaderLease is a Valkey-backed leader-election lease.
//
// Acquire performs a SET NX EX <ttl>; the holder periodically calls Renew to
// extend the TTL. When Valkey is absent (NewLeaderLease(nil, ...) or any
// command error), Acquire and IsLeader return false and the caller should
// fall back to single-replica behavior (every replica acts as leader, see
// acquireLeadership in worker/scheduler.go).
type LeaderLease struct {
	client        *Client
	key           string
	identity      string
	ttl           time.Duration
	mu            sync.Mutex
	held          bool
	renewScript   *valkey.Lua
	releaseScript *valkey.Lua
}

// NewLeaderLease creates a new leader lease for the given key. Identity is a
// random hex string unique to this process so we can verify ownership at
// renew/release time.
const renewLuaSrc = `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("EXPIRE", KEYS[1], ARGV[2]) else return 0 end`
const releaseLuaSrc = `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`

func NewLeaderLease(client *Client, key string, ttl time.Duration) *LeaderLease {
	if ttl <= 0 {
		ttl = 15 * time.Second
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		id = []byte(time.Now().UTC().Format(time.RFC3339Nano))
	}
	return &LeaderLease{
		client:        client,
		key:           key,
		identity:      hex.EncodeToString(id),
		ttl:           ttl,
		renewScript:   valkey.NewLuaScript(renewLuaSrc),
		releaseScript: valkey.NewLuaScript(releaseLuaSrc),
	}
}

// Identity returns the random identity string used to fence renew/release.
// Exposed for tests/observability only.
func (l *LeaderLease) Identity() string { return l.identity }

// IsLeader returns true if this lease is currently believed to be held.
// Single-replica fallback: when no Valkey client is configured, returns true.
func (l *LeaderLease) IsLeader() bool {
	if l == nil {
		return false
	}
	if l.client == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.held
}

// Acquire tries to take the lease. Returns true on success, false if another
// replica currently holds it. Fail-closed on Valkey errors (returns false +
// error). When the client is nil, acts as a single-replica leader (returns
// true with no error).
func (l *LeaderLease) Acquire(ctx context.Context) (bool, error) {
	if l == nil {
		return false, nil
	}
	if l.client == nil {
		l.mu.Lock()
		l.held = true
		l.mu.Unlock()
		return true, nil
	}
	secs := int64(l.ttl.Seconds())
	if secs <= 0 {
		secs = 15
	}
	res, err := l.client.Do(ctx, l.client.Builder().Set().
		Key(l.key).Value(l.identity).Nx().ExSeconds(secs).Build()).ToString()
	if err != nil {
		if errors.Is(err, valkey.Nil) {
			l.mu.Lock()
			l.held = false
			l.mu.Unlock()
			return false, nil
		}
		return false, fmt.Errorf("leader acquire: %w", err)
	}
	ok := res == "OK"
	l.mu.Lock()
	l.held = ok
	l.mu.Unlock()
	if ok {
		logger.Info("acquired leader lease", "component", "valkey", "key", l.key, "identity", l.identity)
	}
	return ok, nil
}

// Renew refreshes the lease iff this process still owns it. Returns true on
// success, false if the lease was lost.
func (l *LeaderLease) Renew(ctx context.Context) (bool, error) {
	if l == nil {
		return false, nil
	}
	if l.client == nil {
		l.mu.Lock()
		l.held = true
		l.mu.Unlock()
		return true, nil
	}
	secs := int64(l.ttl.Seconds())
	if secs <= 0 {
		secs = 15
	}
	// Fence with a Lua check so we never extend a key owned by someone else.
	resp := l.renewScript.Exec(ctx, l.client.Client(), []string{l.key}, []string{l.identity, strconv.FormatInt(secs, 10)})
	if err := resp.Error(); err != nil {
		logger.Error("failed to renew leader lease", "component", "valkey", "key", l.key, "error", err)
		return false, fmt.Errorf("leader renew: %w", err)
	}
	n, _ := resp.AsInt64()
	ok := n == 1
	l.mu.Lock()
	l.held = ok
	l.mu.Unlock()
	if !ok {
		logger.Warn("lost leader lease", "component", "valkey", "key", l.key, "identity", l.identity)
	}
	return ok, nil
}

// Release voluntarily relinquishes the lease. Safe to call multiple times.
func (l *LeaderLease) Release(ctx context.Context) {
	if l == nil || l.client == nil {
		if l != nil {
			l.mu.Lock()
			l.held = false
			l.mu.Unlock()
		}
		return
	}
	_ = l.releaseScript.Exec(ctx, l.client.Client(), []string{l.key}, []string{l.identity}).Error()
	l.mu.Lock()
	wasHeld := l.held
	l.held = false
	l.mu.Unlock()
	if wasHeld {
		logger.Info("released leader lease", "component", "valkey", "key", l.key, "identity", l.identity)
	}
}
