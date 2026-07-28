// MessageDedup is a bounded, TTL-based dedup cache for SSE message IDs.
//
// Eviction happens before insertion: expired entries are removed first, and
// if the cache is still at capacity the oldest entries are trimmed down to
// 90% of maxSize. This guarantees a just-accepted ID is never evicted by its
// own insertion (which would let an immediate replay be treated as new).
export class MessageDedup {
  private seen = new Map<string, number>();
  private maxSize: number;
  private ttlMs: number;

  constructor(maxSize = 1000, ttlMs = 300_000) {
    this.maxSize = Math.max(1, maxSize);
    this.ttlMs = ttlMs > 0 ? ttlMs : 300_000;
  }

  // isDuplicate reports whether messageId has been observed within the TTL
  // window. The first observation records the ID and returns false.
  isDuplicate(messageId: string): boolean {
    const now = Date.now();
    this.evict(now);

    // Trim by count BEFORE inserting so the new entry is never the victim of
    // its own capacity eviction.
    if (this.seen.size >= this.maxSize) {
      this.trimTo(Math.floor(this.maxSize * 0.9));
    }

    const lastSeen = this.seen.get(messageId);
    if (lastSeen !== undefined) {
      // Refresh the timestamp so the window reflects recent replay.
      this.seen.set(messageId, now);
      return true;
    }

    this.seen.set(messageId, now);
    return false;
  }

  clear(): void {
    this.seen.clear();
  }

  get size(): number {
    return this.seen.size;
  }

  private evict(now: number): void {
    for (const [k, t] of this.seen) {
      if (now - t > this.ttlMs) this.seen.delete(k);
    }
  }

  // trimTo removes the oldest entries until size <= target.
  private trimTo(target: number): void {
    while (this.seen.size > target) {
      let oldestKey: string | null = null;
      let oldestTime = Infinity;
      for (const [k, t] of this.seen) {
        if (t < oldestTime) {
          oldestTime = t;
          oldestKey = k;
        }
      }
      if (oldestKey === null) break;
      this.seen.delete(oldestKey);
    }
  }
}
