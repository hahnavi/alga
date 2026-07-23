package valkey

import "testing"

func TestNewIdempotencyCache_NilClient(t *testing.T) {
	t.Parallel()
	if c := NewIdempotencyCache(nil); c != nil {
		t.Fatal("NewIdempotencyCache(nil) must return nil so callers treat idempotency as disabled")
	}
}
