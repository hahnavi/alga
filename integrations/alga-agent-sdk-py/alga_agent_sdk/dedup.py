import threading
import time


class MessageDedup:
    def __init__(self, max_entries: int = 1000, ttl_seconds: float = 300.0):
        self._max = max_entries
        self._ttl = ttl_seconds
        self._seen: dict[str, float] = {}
        self._lock = threading.Lock()

    def is_duplicate(self, message_id: str) -> bool:
        now = time.monotonic()
        with self._lock:
            self._evict(now)
            if message_id in self._seen:
                return True
            self._seen[message_id] = now
            if len(self._seen) > self._max:
                oldest = next(iter(self._seen))
                del self._seen[oldest]
            return False

    def clear(self) -> None:
        with self._lock:
            self._seen.clear()

    def _evict(self, now: float) -> None:
        expired = [k for k, t in self._seen.items() if now - t > self._ttl]
        for k in expired:
            del self._seen[k]
