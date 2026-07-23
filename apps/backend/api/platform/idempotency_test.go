package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// mockIdempotencyStore is an in-memory IdempotencyStore for tests.
type mockIdempotencyStore struct {
	mu       sync.Mutex
	data     map[string]string
	getCalls int
	setCalls int
	getErr   error
	setErr   error
}

func newMockIdempotencyStore() *mockIdempotencyStore {
	return &mockIdempotencyStore{data: map[string]string{}}
}

func (m *mockIdempotencyStore) Get(_ context.Context, key string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getCalls++
	if m.getErr != nil {
		return "", false, m.getErr
	}
	v, ok := m.data[key]
	return v, ok, nil
}

func (m *mockIdempotencyStore) Set(_ context.Context, key, value string, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setCalls++
	if m.setErr != nil {
		return m.setErr
	}
	m.data[key] = value
	return nil
}

// countingHandler records how many times it ran and returns a fixed body.
func countingHandler(count *int, status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*count++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

func postWithKey(key string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/incidents", nil)
	if key != "" {
		req.Header.Set(IdempotencyKeyHeader, key)
	}
	return req
}

func TestWithIdempotency_MissExecutesAndStores(t *testing.T) {
	t.Parallel()
	store := newMockIdempotencyStore()
	calls := 0
	h := WithIdempotency(store, time.Hour, "incident:create", countingHandler(&calls, http.StatusCreated, `{"id":"1"}`))

	rec := httptest.NewRecorder()
	h(rec, postWithKey("key-1"))

	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if rec.Body.String() != `{"id":"1"}` {
		t.Fatalf("body = %q", rec.Body.String())
	}
	if store.setCalls != 1 {
		t.Fatalf("setCalls = %d, want 1", store.setCalls)
	}
	if rec.Header().Get(IdempotentReplayedHeader) != "" {
		t.Fatal("first request must not be flagged as replayed")
	}
}

func TestWithIdempotency_DuplicateReplaysCachedResponse(t *testing.T) {
	t.Parallel()
	store := newMockIdempotencyStore()
	calls := 0
	h := WithIdempotency(store, time.Hour, "incident:create", countingHandler(&calls, http.StatusCreated, `{"id":"1"}`))

	// First request executes and caches.
	first := httptest.NewRecorder()
	h(first, postWithKey("key-1"))

	// Second request with the same key replays without re-executing.
	second := httptest.NewRecorder()
	h(second, postWithKey("key-1"))

	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1 (duplicate must not re-execute)", calls)
	}
	if second.Code != http.StatusCreated {
		t.Fatalf("replay status = %d, want %d", second.Code, http.StatusCreated)
	}
	if second.Body.String() != `{"id":"1"}` {
		t.Fatalf("replay body = %q, want original", second.Body.String())
	}
	if second.Header().Get(IdempotentReplayedHeader) != "true" {
		t.Fatal("replay must set the Idempotent-Replayed header")
	}
	if second.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("replay content-type = %q", second.Header().Get("Content-Type"))
	}
}

func TestWithIdempotency_DifferentKeysExecuteSeparately(t *testing.T) {
	t.Parallel()
	store := newMockIdempotencyStore()
	calls := 0
	h := WithIdempotency(store, time.Hour, "incident:create", countingHandler(&calls, http.StatusCreated, `{"id":"1"}`))

	h(httptest.NewRecorder(), postWithKey("key-a"))
	h(httptest.NewRecorder(), postWithKey("key-b"))

	if calls != 2 {
		t.Fatalf("handler calls = %d, want 2 (distinct keys run separately)", calls)
	}
	if store.setCalls != 2 {
		t.Fatalf("setCalls = %d, want 2", store.setCalls)
	}
}

func TestWithIdempotency_MissingHeaderPassesThrough(t *testing.T) {
	t.Parallel()
	store := newMockIdempotencyStore()
	calls := 0
	h := WithIdempotency(store, time.Hour, "incident:create", countingHandler(&calls, http.StatusCreated, `{"id":"1"}`))

	rec := httptest.NewRecorder()
	h(rec, postWithKey("")) // no Idempotency-Key header

	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
	if store.getCalls != 0 || store.setCalls != 0 {
		t.Fatalf("store must be untouched without header: get=%d set=%d", store.getCalls, store.setCalls)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestWithIdempotency_NilStoreIsPassThrough(t *testing.T) {
	t.Parallel()
	calls := 0
	h := WithIdempotency(nil, time.Hour, "incident:create", countingHandler(&calls, http.StatusCreated, `{"ok":true}`))

	rec := httptest.NewRecorder()
	h(rec, postWithKey("key-1"))

	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestWithIdempotency_NonSuccessNotCached(t *testing.T) {
	t.Parallel()
	store := newMockIdempotencyStore()
	calls := 0
	h := WithIdempotency(store, time.Hour, "incident:create", countingHandler(&calls, http.StatusConflict, `{"error":"dup"}`))

	// First request returns a 409 and must NOT be cached.
	h(httptest.NewRecorder(), postWithKey("key-1"))
	if store.setCalls != 0 {
		t.Fatalf("setCalls = %d, want 0 (failures are not cached)", store.setCalls)
	}

	// A retry therefore re-executes the handler.
	h(httptest.NewRecorder(), postWithKey("key-1"))
	if calls != 2 {
		t.Fatalf("handler calls = %d, want 2 (failed writes stay retryable)", calls)
	}
}

func TestWithIdempotency_GetOnlyOnGetMethodPassesThrough(t *testing.T) {
	t.Parallel()
	store := newMockIdempotencyStore()
	calls := 0
	h := WithIdempotency(store, time.Hour, "incident:create", countingHandler(&calls, http.StatusOK, `[]`))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents", nil)
	req.Header.Set(IdempotencyKeyHeader, "key-1")
	h(httptest.NewRecorder(), req)

	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
	if store.getCalls != 0 || store.setCalls != 0 {
		t.Fatalf("GET must bypass idempotency: get=%d set=%d", store.getCalls, store.setCalls)
	}
}

func TestWithIdempotency_LookupErrorFailsOpen(t *testing.T) {
	t.Parallel()
	store := newMockIdempotencyStore()
	store.getErr = context.DeadlineExceeded
	calls := 0
	h := WithIdempotency(store, time.Hour, "incident:create", countingHandler(&calls, http.StatusCreated, `{"id":"1"}`))

	rec := httptest.NewRecorder()
	h(rec, postWithKey("key-1"))

	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1 (lookup failure must fail open)", calls)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestWithIdempotency_OversizedKeyRejected(t *testing.T) {
	t.Parallel()
	store := newMockIdempotencyStore()
	calls := 0
	h := WithIdempotency(store, time.Hour, "incident:create", countingHandler(&calls, http.StatusCreated, `{}`))

	big := make([]byte, maxIdempotencyKeyLen+1)
	for i := range big {
		big[i] = 'a'
	}
	rec := httptest.NewRecorder()
	h(rec, postWithKey(string(big)))

	if calls != 0 {
		t.Fatalf("handler calls = %d, want 0 (oversized key rejected)", calls)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestIdempotencyStoreKey_ScopeIsolation(t *testing.T) {
	t.Parallel()
	a := idempotencyStoreKey("incident:create", "same-key")
	b := idempotencyStoreKey("notification:send", "same-key")
	if a == b {
		t.Fatal("same client key under different scopes must produce different store keys")
	}
	if got := idempotencyStoreKey("incident:create", "same-key"); got != a {
		t.Fatal("store key derivation must be deterministic")
	}
}
