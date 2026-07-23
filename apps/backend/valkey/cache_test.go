package valkey

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
)

type mockDoer struct {
	store     map[string]string
	getCalls  int
	setCalls  int
	delCalls  int
	scanCalls int
	scanErr   error
	getErr    error
	setErr    error
	delErr    error
}

func (m *mockDoer) get(ctx context.Context, key string) (string, error) {
	m.getCalls++
	if m.getErr != nil {
		return "", m.getErr
	}
	v, ok := m.store[key]
	if !ok {
		return "", valkeygo.Nil
	}
	return v, nil
}

func (m *mockDoer) set(ctx context.Context, key, value string, ttl time.Duration) error {
	m.setCalls++
	if m.setErr != nil {
		return m.setErr
	}
	if m.store == nil {
		m.store = make(map[string]string)
	}
	m.store[key] = value
	return nil
}

func (m *mockDoer) del(ctx context.Context, keys ...string) error {
	m.delCalls++
	if m.delErr != nil {
		return m.delErr
	}
	for _, k := range keys {
		delete(m.store, k)
	}
	return nil
}

func (m *mockDoer) scan(ctx context.Context, cursor uint64, match string, count int64) (valkeygo.ScanEntry, error) {
	m.scanCalls++
	if m.scanErr != nil {
		return valkeygo.ScanEntry{}, m.scanErr
	}
	pattern := strings.TrimSuffix(match, "*")
	var elements []string
	for k := range m.store {
		if strings.HasPrefix(k, pattern) {
			elements = append(elements, k)
		}
	}
	return valkeygo.ScanEntry{Cursor: 0, Elements: elements}, nil
}

func TestCacheNilClient(t *testing.T) {
	t.Parallel()
	c := NewCache(nil)
	if c.Available() {
		t.Fatal("Available() must be false for nil client")
	}

	ctx := context.Background()
	fetchCount := 0
	fetchFn := func(ctx context.Context) ([]byte, error) {
		fetchCount++
		return json.Marshal(map[string]string{"key": "value"})
	}

	data, err := c.GetOrSet(ctx, "test:key", 10*time.Second, fetchFn)
	if err != nil {
		t.Fatalf("GetOrSet(nil client) error = %v", err)
	}
	if fetchCount != 1 {
		t.Fatalf("fetchCount = %d, want 1", fetchCount)
	}

	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if result["key"] != "value" {
		t.Fatalf("result[key] = %q, want %q", result["key"], "value")
	}

	if err := c.Invalidate(ctx, "test:key"); err != nil {
		t.Fatalf("Invalidate(nil client) error = %v", err)
	}
	if err := c.InvalidatePrefix(ctx, "test:"); err != nil {
		t.Fatalf("InvalidatePrefix(nil client) error = %v", err)
	}
}

func TestCacheGetOrSetFetchError(t *testing.T) {
	t.Parallel()
	c := NewCache(nil)
	ctx := context.Background()

	fetchFn := func(ctx context.Context) ([]byte, error) {
		return nil, context.DeadlineExceeded
	}

	_, err := c.GetOrSet(ctx, "test:err", 10*time.Second, fetchFn)
	if err != context.DeadlineExceeded {
		t.Fatalf("GetOrSet error = %v, want %v", err, context.DeadlineExceeded)
	}
}

func TestCacheGetOrSet_Hit(t *testing.T) {
	t.Parallel()
	mc := &mockDoer{store: map[string]string{"test:key": `{"cached":"true"}`}}
	c := &Cache{client: mc}

	ctx := context.Background()
	fetchCalled := false
	fetchFn := func(ctx context.Context) ([]byte, error) {
		fetchCalled = true
		return json.Marshal(map[string]string{"fresh": "data"})
	}

	data, err := c.GetOrSet(ctx, "test:key", 10*time.Second, fetchFn)
	if err != nil {
		t.Fatalf("GetOrSet error = %v", err)
	}
	if fetchCalled {
		t.Fatal("fetchFn should not be called on cache hit")
	}
	if mc.getCalls != 1 {
		t.Fatalf("getCalls = %d, want 1", mc.getCalls)
	}
	if mc.setCalls != 0 {
		t.Fatalf("setCalls = %d, want 0", mc.setCalls)
	}

	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if result["cached"] != "true" {
		t.Fatalf("result[cached] = %q, want %q", result["cached"], "true")
	}
}

func TestCacheGetOrSet_Miss(t *testing.T) {
	t.Parallel()
	mc := &mockDoer{store: map[string]string{}}
	c := &Cache{client: mc}

	ctx := context.Background()
	fetchFn := func(ctx context.Context) ([]byte, error) {
		return json.Marshal(map[string]string{"fresh": "data"})
	}

	data, err := c.GetOrSet(ctx, "test:miss", 10*time.Second, fetchFn)
	if err != nil {
		t.Fatalf("GetOrSet error = %v", err)
	}
	if mc.getCalls != 1 {
		t.Fatalf("getCalls = %d, want 1", mc.getCalls)
	}
	if mc.setCalls != 1 {
		t.Fatalf("setCalls = %d, want 1", mc.setCalls)
	}

	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if result["fresh"] != "data" {
		t.Fatalf("result[fresh] = %q, want %q", result["fresh"], "data")
	}
	if mc.store["test:miss"] != `{"fresh":"data"}` {
		t.Fatalf("store[test:miss] = %q, want %q", mc.store["test:miss"], `{"fresh":"data"}`)
	}
}

func TestCacheGetOrSet_MissFetchError(t *testing.T) {
	t.Parallel()
	mc := &mockDoer{store: map[string]string{}}
	c := &Cache{client: mc}

	ctx := context.Background()
	fetchFn := func(ctx context.Context) ([]byte, error) {
		return nil, context.DeadlineExceeded
	}

	_, err := c.GetOrSet(ctx, "test:err", 10*time.Second, fetchFn)
	if err != context.DeadlineExceeded {
		t.Fatalf("GetOrSet error = %v, want %v", err, context.DeadlineExceeded)
	}
	if mc.setCalls != 0 {
		t.Fatalf("setCalls = %d, want 0 (should not store on fetch error)", mc.setCalls)
	}
}

func TestCacheInvalidate(t *testing.T) {
	t.Parallel()
	mc := &mockDoer{store: map[string]string{"test:key": "value"}}
	c := &Cache{client: mc}

	ctx := context.Background()
	if err := c.Invalidate(ctx, "test:key"); err != nil {
		t.Fatalf("Invalidate error = %v", err)
	}
	if mc.delCalls != 1 {
		t.Fatalf("delCalls = %d, want 1", mc.delCalls)
	}
	if _, ok := mc.store["test:key"]; ok {
		t.Fatal("key should be deleted after Invalidate")
	}
}

func TestCacheInvalidatePrefix(t *testing.T) {
	t.Parallel()
	mc := &mockDoer{store: map[string]string{
		"test:a": "1",
		"test:b": "2",
		"other":  "3",
	}}
	c := &Cache{client: mc}

	ctx := context.Background()
	if err := c.InvalidatePrefix(ctx, "test:"); err != nil {
		t.Fatalf("InvalidatePrefix error = %v", err)
	}
	if mc.scanCalls != 1 {
		t.Fatalf("scanCalls = %d, want 1", mc.scanCalls)
	}
	if mc.delCalls != 1 {
		t.Fatalf("delCalls = %d, want 1", mc.delCalls)
	}
	if _, ok := mc.store["test:a"]; ok {
		t.Fatal("test:a should be deleted")
	}
	if _, ok := mc.store["test:b"]; ok {
		t.Fatal("test:b should be deleted")
	}
	if _, ok := mc.store["other"]; !ok {
		t.Fatal("other should not be deleted")
	}
}
