from __future__ import annotations

import asyncio
import json
import logging
import random
from typing import Any, Callable, Coroutine

import httpx

from .dedup import MessageDedup
from .errors import AlgaAuthError, AlgaConnectionError
from .models import (
    AgentPresenceEvent,
    ConnectedEvent,
    MessageEvent,
    PeerAskEvent,
    PeerFindingEvent,
    PeerReplyEvent,
    TypingEvent,
)

EventHandler = Callable[..., Coroutine[Any, Any, None]]

_RECONNECT_BASE = 2.0
_RECONNECT_MAX = 60.0
_JITTER_FACTOR = 0.2
_HEARTBEAT_INTERVAL = 30.0

_EVENT_MODELS: dict[str, type] = {
    "connected": ConnectedEvent,
    "message": MessageEvent,
    "typing": TypingEvent,
    "peer_finding": PeerFindingEvent,
    "peer_ask": PeerAskEvent,
    "peer_reply": PeerReplyEvent,
    "agent_presence": AgentPresenceEvent,
}

_LOCK_EMOJI = "\U0001f512"

_logger = logging.getLogger("alga_agent_sdk.sse")


class SSEClient:
    def __init__(
        self,
        http_base: str,
        token: str,
        dedup: MessageDedup | None = None,
        heartbeat_interval: float = 30.0,
    ):
        self._http_base = http_base.rstrip("/")
        self._token = token
        self._dedup = dedup or MessageDedup()
        self._heartbeat_interval = heartbeat_interval
        self._client: httpx.AsyncClient | None = None
        self._sse_task: asyncio.Task | None = None
        self._heartbeat_task: asyncio.Task | None = None
        self._backoff = _RECONNECT_BASE
        self._stopped = False

        self.on_connected: EventHandler | None = None
        self.on_message: EventHandler | None = None
        self.on_typing: EventHandler | None = None
        self.on_investigation_cancel: EventHandler | None = None
        self.on_investigation_pause: EventHandler | None = None
        self.on_investigation_resume: EventHandler | None = None
        self.on_peer_finding: EventHandler | None = None
        self.on_peer_ask: EventHandler | None = None
        self.on_peer_reply: EventHandler | None = None
        self.on_agent_presence: EventHandler | None = None

    async def start(self) -> None:
        self._stopped = False
        self._client = httpx.AsyncClient(
            base_url=self._http_base,
            headers={"Authorization": f"Bearer {self._token}"},
            timeout=httpx.Timeout(30.0),
        )
        self._sse_task = asyncio.create_task(self._sse_loop())
        self._heartbeat_task = asyncio.create_task(self._heartbeat_loop())

    async def stop(self) -> None:
        self._stopped = True
        for task in (self._sse_task, self._heartbeat_task):
            if task and not task.done():
                task.cancel()
        if self._client:
            await self._client.aclose()
            self._client = None

    async def wait(self) -> None:
        tasks = [t for t in (self._sse_task, self._heartbeat_task) if t is not None]
        if tasks:
            await asyncio.gather(*tasks, return_exceptions=True)

    async def _sse_loop(self) -> None:
        while not self._stopped:
            try:
                await self._connect_sse()
            except AlgaAuthError:
                raise
            except (httpx.HTTPError, AlgaConnectionError, Exception) as exc:
                if self._stopped:
                    return
                _logger.warning("SSE disconnected: %s — reconnecting in %.1fs", exc, self._backoff)
                await self._sleep_with_jitter()
                self._backoff = min(self._backoff * 2, _RECONNECT_MAX)

    async def _connect_sse(self) -> None:
        if not self._client:
            return
        url = "/api/v1/agent/events"
        async with self._client.stream("GET", url) as resp:
            if resp.status_code in (401, 403):
                body = await resp.aread()
                raise AlgaAuthError(resp.status_code, body.decode(errors="replace"))
            if resp.status_code >= 400:
                body = await resp.aread()
                from .errors import AlgaAPIError
                raise AlgaAPIError(resp.status_code, body.decode(errors="replace"))

            self._backoff = _RECONNECT_BASE
            event_type = ""
            data_buf: list[str] = []

            async for line_bytes in resp.aiter_lines():
                if self._stopped:
                    return
                line = line_bytes if isinstance(line_bytes, str) else line_bytes.decode(errors="replace")

                if line.startswith(":"):
                    continue

                if line.startswith("event:"):
                    event_type = line[len("event:"):].strip()
                    continue

                if line.startswith("data:"):
                    data_buf.append(line[len("data:"):].strip())
                    continue

                if line == "" and (event_type or data_buf):
                    await self._dispatch(event_type, "\n".join(data_buf))
                    event_type = ""
                    data_buf = []

    async def _dispatch(self, event_type: str, raw_data: str) -> None:
        if not raw_data:
            return
        try:
            payload = json.loads(raw_data)
        except json.JSONDecodeError:
            _logger.warning("Invalid JSON in SSE event %s: %s", event_type, raw_data[:200])
            return

        model_cls = _EVENT_MODELS.get(event_type)
        if model_cls:
            try:
                evt = model_cls.model_validate(payload)
            except Exception:
                _logger.warning("Validation failed for SSE event %s", event_type, exc_info=True)
                return
            await self._handle_typed_event(event_type, evt)
            return

        await self._handle_signal_event(event_type, payload)

    async def _handle_typed_event(self, event_type: str, evt: Any) -> None:
        if isinstance(evt, ConnectedEvent):
            if self.on_connected:
                await self.on_connected(evt)
        elif isinstance(evt, MessageEvent):
            if self._dedup.is_duplicate(evt.message_id):
                return
            if evt.text.startswith(_LOCK_EMOJI):
                return
            if self.on_message:
                await self.on_message(evt)
        elif isinstance(evt, TypingEvent):
            if self.on_typing:
                await self.on_typing(evt)
        elif isinstance(evt, PeerFindingEvent):
            if self.on_peer_finding:
                await self.on_peer_finding(evt)
        elif isinstance(evt, PeerAskEvent):
            if self.on_peer_ask:
                await self.on_peer_ask(evt)
        elif isinstance(evt, PeerReplyEvent):
            if self.on_peer_reply:
                await self.on_peer_reply(evt)
        elif isinstance(evt, AgentPresenceEvent):
            if self.on_agent_presence:
                await self.on_agent_presence(evt)

    async def _handle_signal_event(self, event_type: str, payload: dict) -> None:
        from .models import InvestigationSignalEvent

        try:
            signal = InvestigationSignalEvent.model_validate(payload)
        except Exception:
            return

        cb = {
            "investigation_cancel": self.on_investigation_cancel,
            "investigation_pause": self.on_investigation_pause,
            "investigation_resume": self.on_investigation_resume,
        }.get(event_type)
        if cb:
            await cb(signal)

    async def _heartbeat_loop(self) -> None:
        while not self._stopped:
            try:
                await asyncio.sleep(self._heartbeat_interval)
                if self._stopped or not self._client:
                    return
                resp = await self._client.post("/api/v1/agent/heartbeat")
                if resp.status_code >= 400:
                    _logger.warning("Heartbeat failed: %d", resp.status_code)
            except asyncio.CancelledError:
                return
            except Exception as exc:
                _logger.warning("Heartbeat error: %s", exc)

    async def _sleep_with_jitter(self) -> None:
        jitter = self._backoff * _JITTER_FACTOR * random.random()
        await asyncio.sleep(self._backoff + jitter)
