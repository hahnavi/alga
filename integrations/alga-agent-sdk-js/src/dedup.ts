export class MessageDedup {
  private seen = new Map<string, number>();
  private maxSize: number;
  private ttlMs: number;

  constructor(maxSize = 1000, ttlMs = 300000) {
    this.maxSize = maxSize;
    this.ttlMs = ttlMs;
  }

  isDuplicate(messageId: string): boolean {
    const now = Date.now();
    const lastSeen = this.seen.get(messageId);
    if (lastSeen !== undefined && now - lastSeen < this.ttlMs) {
      return true;
    }

    if (this.seen.size >= this.maxSize) {
      const oldestKey = this.seen.keys().next().value;
      if (oldestKey !== undefined) {
        this.seen.delete(oldestKey);
      }
    }

    this.seen.set(messageId, now);
    return false;
  }

  clear(): void {
    this.seen.clear();
  }
}
