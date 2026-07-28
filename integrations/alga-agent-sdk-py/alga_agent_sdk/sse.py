from __future__ import annotations

import asyncio
import inspect
import json
import logging
import random
from datetime import datetime, timedelta, timezone
from email.utils import parsedate_to_datetime
from typing import Any, Callable, Optional

import httpx

from .dedup import MessageDedup
from .errors import AlgaAPIError, AlgaAuthError, AlgaConnectionError
from .models import (
    AlertAutoResolvedEvent,
    ConnectedEvent,
    CoordinationTaskEvent,
    IncidentCommsStaleEvent,
    InvestigationSignalEvent,
    MessageEvent,
    PeerAskEvent,
    PeerFindingEvent,
    PeerReplyEvent,
    SummarizeIncidentEvent,
    TypingEvent,
)

ErrHandler = Callable[[Exception], None]
EventHandler = Callable[..., Any]

_RECONNECT_BASE = 2.0
_RECONNECT_MAX = 60.0
_MAX_RETRY_AFTER = timedelta(minutes=10)
_LOCK_EMOJI = "\U0001f512"

_logger = logging.getLogger("alga_agent_sdk.sse")


def parse_retry_after(raw: Optional[str]) -> timedelta:
    if not raw:
        return timedelta(0)
    trimmed = raw.strip()
    if not trimmed:
        return timedelta(0)
    try:
        secs = int(trimmed)
    except ValueError:
        secs = None
    if secs is not None:
        if secs < 0:
            return timedelta(0)
        return min(timedelta(seconds=secs), _MAX_RETRY_AFTER)
    try:
        dt = parsedate_to_datetime(trimmed)
    except (TypeError, ValueError):
        dt = None
    if dt is not None:
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        delta = (dt - datetime.now(timezone.utc)).total_seconds()
        if delta > 0:
            return min(timedelta(seconds=delta), _MAX_RETRY_AFTER)
    return timedelta(0)


class SSEClient:
    def __init__(
        self,
        http_base: str,
        token: str,
        dedup: Optional[MessageDedup] = None,
        heartbeat_interval: float = 30.0,
        user_agent: str = "alga-agent-sdk-py",
    ):
        self._http_base = http_base.rstrip("/")
        self._token = token
        self._dedup = dedup if dedup is not None else MessageDedup()
        self._heartbeat_interval = max(1.0, heartbeat_interval)
        self._user_agent = user_agent
        self._client: Optional[httpx.AsyncClient] = None
        self._tasks: list[asyncio.Task[Any]] = []
        self._stop_event = asyncio.Event()
        self._stopped = False
        self._err_handler: Optional[ErrHandler] = None

        self.on_connected: Optional[EventHandler] = None
        self.on_message: Optional[EventHandler] = None
        self.on_typing: Optional[EventHandler] = None
        self.on_investigation_resume: Optional[EventHandler] = None
        self.on_peer_finding: Optional[EventHandler] = None
        self.on_peer_ask: Optional[EventHandler] = None
        self.on_peer_reply: Optional[EventHandler] = None
        self.on_coordination_task: Optional[EventHandler] = None
        self.on_summarize_incident: Optional[EventHandler] = None
        self.on_alert_auto_resolved: Optional[EventHandler] = None
        self.on_incident_comms_stale: Optional[EventHandler] = None
        self.on_unknown_event: Optional[EventHandler] = None

    def set_err_handler(self, handler: ErrHandler) -> None:
        self._err_handler = handler

    async def start(self) -> None:
        self._stopped = False
        self._stop_event.clear()
        self._client = httpx.AsyncClient(timeout=None)
        self._tasks = [
            asyncio.create_task(self._sse_loop()),
            asyncio.create_task(self._heartbeat_loop()),
        ]

    async def stop(self) -> None:
        self._stopped = True
        self._stop_event.set()
        for task in self._tasks:
            if not task.done():
                task.cancel()
        if self._tasks:
            await asyncio.gather(*self._tasks, return_exceptions=True)
        self._tasks = []
        if self._client is not None:
            await self._client.aclose()
            self._client = None

    async def wait(self) -> None:
        if self._tasks:
            await asyncio.gather(*self._tasks, return_exceptions=True)

    async def _fatal(self, err: Exception) -> None:
        self._stopped = True
        self._stop_event.set()
        if self._err_handler is not None:
            try:
                self._err_handler(err)
            except Exception:
                _logger.exception("error handler raised")
        current = asyncio.current_task()
        for task in self._tasks:
            if task is not current and not task.done():
                task.cancel()

    async def _sse_loop(self) -> None:
        backoff = _RECONNECT_BASE
        while not self._stopped and not self._stop_event.is_set():
            connected, err = await self._connect_and_serve()
            if connected:
                backoff = _RECONNECT_BASE
            if err is not None:
                if isinstance(err, AlgaAuthError):
                    _logger.error(
                        "sse auth error, stopping reconnect loop: %s %s",
                        err.status_code,
                        err.message,
                    )
                    await self._fatal(err)
                    return
                delay = backoff
                if isinstance(err, AlgaAPIError) and err.retry_after > timedelta(0):
                    delay = err.retry_after.total_seconds()
                jitter = delay * (0.9 + random.random() * 0.2)
                _logger.warning("sse reconnecting after error: %s", err)
                try:
                    await asyncio.wait_for(self._stop_event.wait(), timeout=jitter)
                    return
                except asyncio.TimeoutError:
                    pass
                backoff = min(backoff * 2, _RECONNECT_MAX)
            if self._stopped or self._stop_event.is_set():
                return

    async def _connect_and_serve(self) -> tuple[bool, Optional[Exception]]:
        if self._client is None:
            return False, AlgaConnectionError("sse client not started")
        url = f"{self._http_base}/api/v1/agent/events"
        headers = {
            "Accept": "text/event-stream",
            "Authorization": f"Bearer {self._token}",
            "User-Agent": self._user_agent,
        }
        try:
            async with self._client.stream("GET", url, headers=headers) as resp:
                if resp.status_code in (401, 403):
                    return False, AlgaAuthError(
                        resp.status_code, "authentication failed"
                    )
                if resp.status_code != 200:
                    retry_after = parse_retry_after(resp.headers.get("Retry-After"))
                    return False, AlgaAPIError(
                        resp.status_code, "unexpected status code", retry_after
                    )

                event_type = ""
                data_buf: list[str] = []

                async for raw_line in resp.aiter_lines():
                    if self._stopped or self._stop_event.is_set():
                        return True, None
                    line = raw_line.rstrip("\r")

                    if line == "":
                        if data_buf:
                            ev = event_type or "message"
                            await self._dispatch(ev, "\n".join(data_buf))
                        event_type = ""
                        data_buf = []
                        continue

                    if line.startswith(":"):
                        continue

                    if line.startswith("event:"):
                        event_type = line[len("event:"):].lstrip()
                        continue

                    if line.startswith("data:"):
                        rest = line[len("data:"):]
                        if rest.startswith(" "):
                            rest = rest[1:]
                        data_buf.append(rest)
                        continue

                    if line.startswith("id:"):
                        continue

                return True, AlgaConnectionError("sse stream closed")
        except asyncio.CancelledError:
            raise
        except httpx.HTTPError as exc:
            return True, AlgaConnectionError(f"stream failed: {exc}")
        except Exception as exc:
            return True, AlgaConnectionError(f"stream failed: {exc}")

    async def _dispatch(self, event_type: str, data: str) -> None:
        data = data.strip()
        try:
            payload = json.loads(data)
        except (ValueError, TypeError):
            return

        if event_type == "connected":
            await self._invoke(self.on_connected, ConnectedEvent.model_validate(payload))
        elif event_type == "message":
            evt = MessageEvent.model_validate(payload)
            if evt.message_id and await self._dedup.is_duplicate(evt.message_id):
                return
            if evt.text.startswith(_LOCK_EMOJI):
                return
            await self._invoke(self.on_message, evt)
        elif event_type == "typing":
            await self._invoke(self.on_typing, TypingEvent.model_validate(payload))
        elif event_type == "investigation_resume":
            await self._invoke(
                self.on_investigation_resume,
                InvestigationSignalEvent.model_validate(payload),
            )
        elif event_type == "peer_finding":
            await self._invoke(
                self.on_peer_finding, PeerFindingEvent.model_validate(payload)
            )
        elif event_type == "peer_ask":
            await self._invoke(self.on_peer_ask, PeerAskEvent.model_validate(payload))
        elif event_type == "peer_reply":
            await self._invoke(
                self.on_peer_reply, PeerReplyEvent.model_validate(payload)
            )
        elif event_type == "coordination_task_dispatched":
            await self._invoke(
                self.on_coordination_task,
                CoordinationTaskEvent.model_validate(payload),
            )
        elif event_type == "summarize_incident":
            await self._invoke(
                self.on_summarize_incident,
                SummarizeIncidentEvent.model_validate(payload),
            )
        elif event_type == "alert_auto_resolved":
            await self._invoke(
                self.on_alert_auto_resolved,
                AlertAutoResolvedEvent.model_validate(payload),
            )
        elif event_type == "incident_comms_stale":
            await self._invoke(
                self.on_incident_comms_stale,
                IncidentCommsStaleEvent.model_validate(payload),
            )
        else:
            await self._invoke(self.on_unknown_event, event_type, data)

    async def _invoke(self, cb: Optional[EventHandler], *args: Any) -> None:
        if cb is None:
            return
        try:
            result = cb(*args)
            if inspect.isawaitable(result):
                await result
        except Exception:
            _logger.exception("SSE callback raised")

    async def _heartbeat_loop(self) -> None:
        while not self._stopped and not self._stop_event.is_set():
            try:
                await asyncio.wait_for(
                    self._stop_event.wait(), timeout=self._heartbeat_interval
                )
                return
            except asyncio.TimeoutError:
                pass
            if self._stopped or self._stop_event.is_set():
                return
            err = await self._post_heartbeat()
            if isinstance(err, AlgaAuthError):
                _logger.error("heartbeat auth failure, stopping: %s", err.status_code)
                await self._fatal(err)
                return
            if err is not None:
                _logger.warning("heartbeat failed: %s", err)

    async def _post_heartbeat(self) -> Optional[Exception]:
        if self._client is None:
            return None
        url = f"{self._http_base}/api/v1/agent/heartbeat"
        headers = {
            "Authorization": f"Bearer {self._token}",
            "User-Agent": self._user_agent,
        }
        try:
            resp = await self._client.post(url, headers=headers)
        except asyncio.CancelledError:
            raise
        except httpx.HTTPError as exc:
            return AlgaConnectionError(f"heartbeat failed: {exc}")
        except Exception as exc:
            return AlgaConnectionError(f"heartbeat failed: {exc}")
        if resp.status_code in (401, 403):
            return AlgaAuthError(resp.status_code, "heartbeat auth failed")
        if resp.status_code >= 400:
            return AlgaAPIError(resp.status_code, "heartbeat non-ok status")
        return None
