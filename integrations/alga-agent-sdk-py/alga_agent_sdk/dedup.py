from __future__ import annotations

import asyncio
import time


class MessageDedup:
    def __init__(self, max_entries: int = 1000, ttl_seconds: float = 300.0):
        self._max = max(1, max_entries)
        self._ttl = ttl_seconds if ttl_seconds > 0 else 300.0
        self._seen: dict[str, float] = {}
        self._lock = asyncio.Lock()

    async def is_duplicate(self, message_id: str) -> bool:
        now = time.monotonic()
        async with self._lock:
            self._evict(now)
            if len(self._seen) >= self._max:
                self._trim_to(self._max * 9 // 10)
            if message_id in self._seen:
                self._seen[message_id] = now
                return True
            self._seen[message_id] = now
            return False

    async def clear(self) -> None:
        async with self._lock:
            self._seen.clear()

    @property
    def size(self) -> int:
        return len(self._seen)

    def _evict(self, now: float) -> None:
        for k, t in list(self._seen.items()):
            if now - t > self._ttl:
                del self._seen[k]

    def _trim_to(self, target: int) -> None:
        target = max(target, 0)
        while len(self._seen) > target:
            oldest_key = None
            oldest_time = 0.0
            first = True
            for k, t in self._seen.items():
                if first or t < oldest_time:
                    oldest_time = t
                    oldest_key = k
                    first = False
            if oldest_key is None:
                break
            del self._seen[oldest_key]
