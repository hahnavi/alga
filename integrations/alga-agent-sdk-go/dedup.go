package alga

import (
	"sync"
	"time"
)

// MessageDedup is a bounded, TTL-based dedup cache for SSE message IDs.
//
// Eviction happens before insertion: expired entries are removed first, and
// if the cache is still at capacity the oldest entries are trimmed down to
// 90% of maxEntries. This guarantees a just-accepted ID is never evicted by
// its own insertion (which would let an immediate replay be treated as new).
type MessageDedup struct {
	mu         sync.Mutex
	seen       map[string]time.Time
	maxEntries int
	ttl        time.Duration
}

func NewMessageDedup(maxEntries int, ttl time.Duration) *MessageDedup {
	if maxEntries < 1 {
		maxEntries = 1
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &MessageDedup{
		seen:       make(map[string]time.Time),
		maxEntries: maxEntries,
		ttl:        ttl,
	}
}

// IsDuplicate reports whether messageID has been observed within the TTL
// window. The first observation records the ID and returns false.
func (d *MessageDedup) IsDuplicate(messageID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	d.evict(now)

	// Trim by count BEFORE inserting so the new entry is never the victim of
	// its own capacity eviction.
	if len(d.seen) >= d.maxEntries {
		d.trimTo(d.maxEntries * 9 / 10)
	}

	if _, ok := d.seen[messageID]; ok {
		// Refresh the timestamp so the LRU-ish window reflects recent replay.
		d.seen[messageID] = now
		return true
	}

	d.seen[messageID] = now
	return false
}

// Clear removes all entries.
func (d *MessageDedup) Clear() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen = make(map[string]time.Time)
}

// Size returns the current number of cached IDs (mostly useful for tests
// and metrics).
func (d *MessageDedup) Size() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.seen)
}

func (d *MessageDedup) evict(now time.Time) {
	for k, t := range d.seen {
		if now.Sub(t) > d.ttl {
			delete(d.seen, k)
		}
	}
}

// trimTo removes the oldest entries until len(seen) <= target.
func (d *MessageDedup) trimTo(target int) {
	target = max(target, 0)
	for len(d.seen) > target {
		var oldest string
		var oldestTime time.Time
		first := true
		for k, t := range d.seen {
			if first || t.Before(oldestTime) {
				oldest, oldestTime = k, t
				first = false
			}
		}
		delete(d.seen, oldest)
	}
}
