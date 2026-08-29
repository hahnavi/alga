from __future__ import annotations

import asyncio
import json
import random
import secrets
from datetime import timedelta
from typing import Any, Callable, Optional
from urllib.parse import quote, urlencode

import httpx

from .commands import InvestigationCommand
from .dedup import MessageDedup
from .errors import AlgaAPIError, AlgaAuthError, AlgaConnectionError
from .models import (
    Alert,
    CommandResponse,
    Incident,
    IncidentContext,
    KnowledgeListResponse,
    KnowledgeNote,
    Memory,
    MemoryListResponse,
    OnCallEntry,
    PeerAsk,
    PeerAskListResponse,
    Playbook,
    SecretValue,
    SendMessageResponse,
    ServiceListResponse,
)
from .sse import EventHandler, SSEClient, parse_retry_after

AGENT_MESSAGES_PATH = "/api/v1/agent/messages"
IDEMPOTENCY_KEY_HEADER = "Idempotency-Key"
MAX_RESPONSE_BYTES = 8 * 1024 * 1024
MAX_ERROR_MESSAGE_BYTES = 4 * 1024


class AlgaClient:
    def __init__(
        self,
        server_url: str,
        token: str,
        *,
        heartbeat_interval: float = 30.0,
        dedup: Optional[MessageDedup] = None,
        max_rest_retries: int = 2,
        user_agent: str = "alga-agent-sdk-py",
    ):
        self._server_url = server_url.rstrip("/")
        self._token = token
        self._heartbeat_interval = max(1.0, heartbeat_interval)
        self._dedup = dedup if dedup is not None else MessageDedup()
        self._max_rest_retries = max(0, max_rest_retries)
        self._user_agent = user_agent
        self._http: Optional[httpx.AsyncClient] = None
        self._sse: Optional[SSEClient] = None
        self._err_handler: Optional[Callable[[Exception], None]] = None

        self.on_connected: Optional[EventHandler] = None
        self.on_message: Optional[EventHandler] = None
        self.on_typing: Optional[EventHandler] = None
        self.on_investigation_resume: Optional[EventHandler] = None
        self.on_peer_finding: Optional[EventHandler] = None
        self.on_peer_ask: Optional[EventHandler] = None
        self.on_peer_reply: Optional[EventHandler] = None
        self.on_summarize_incident: Optional[EventHandler] = None
        self.on_alert_auto_resolved: Optional[EventHandler] = None
        self.on_incident_comms_stale: Optional[EventHandler] = None
        self.on_unknown_event: Optional[EventHandler] = None

    @property
    def server_url(self) -> str:
        return self._server_url

    def on_err(self, handler: Callable[[Exception], None]) -> None:
        self._err_handler = handler
        if self._sse is not None:
            self._sse.set_err_handler(handler)

    async def connect(self) -> None:
        self._sse = SSEClient(
            http_base=self._server_url,
            token=self._token,
            dedup=self._dedup,
            heartbeat_interval=self._heartbeat_interval,
            user_agent=self._user_agent,
        )
        self._sse.on_connected = self.on_connected
        self._sse.on_message = self.on_message
        self._sse.on_typing = self.on_typing
        self._sse.on_investigation_resume = self.on_investigation_resume
        self._sse.on_peer_finding = self.on_peer_finding
        self._sse.on_peer_ask = self.on_peer_ask
        self._sse.on_peer_reply = self.on_peer_reply
        self._sse.on_summarize_incident = self.on_summarize_incident
        self._sse.on_alert_auto_resolved = self.on_alert_auto_resolved
        self._sse.on_incident_comms_stale = self.on_incident_comms_stale
        self._sse.on_unknown_event = self.on_unknown_event
        if self._err_handler is not None:
            self._sse.set_err_handler(self._err_handler)
        await self._sse.start()

    async def disconnect(self) -> None:
        if self._sse is not None:
            await self._sse.stop()
            self._sse = None
        if self._http is not None:
            await self._http.aclose()
            self._http = None

    async def wait(self) -> None:
        if self._sse is not None:
            await self._sse.wait()

    def _rest_client(self) -> httpx.AsyncClient:
        if self._http is None:
            self._http = httpx.AsyncClient(timeout=httpx.Timeout(30.0))
        return self._http

    async def _do_json(
        self,
        method: str,
        path: str,
        payload: Any = None,
        idempotency_key: str = "",
    ) -> Any:
        body_data: Optional[str] = None
        if payload is not None:
            body_data = json.dumps(payload)

        mutating = method not in ("GET", "HEAD")
        if mutating and not idempotency_key and path == AGENT_MESSAGES_PATH:
            idempotency_key = _new_idempotency_key()

        attempts = self._max_rest_retries
        if mutating and not idempotency_key:
            attempts = 0

        last_err: Optional[Exception] = None
        for attempt in range(attempts + 1):
            outcome = await self._raw_request(method, path, body_data, idempotency_key)
            if isinstance(outcome, AlgaConnectionError):
                last_err = outcome
                if not outcome.is_retryable() or attempt == attempts:
                    raise outcome
                await self._sleep(_backoff_for(attempt, timedelta(0)))
                continue

            status, text, retry_after_header = outcome
            if status in (401, 403):
                raise AlgaAuthError(status, _truncate(text, MAX_ERROR_MESSAGE_BYTES))
            if status >= 400:
                api_err = AlgaAPIError(
                    status,
                    _truncate(text, MAX_ERROR_MESSAGE_BYTES),
                    parse_retry_after(retry_after_header),
                )
                if not api_err.is_retryable() or attempt == attempts:
                    raise api_err
                last_err = api_err
                await self._sleep(_backoff_for(attempt, api_err.retry_after))
                continue

            if not text:
                return None
            return _unwrap_envelope(text)

        raise last_err or AlgaConnectionError("exhausted retries")

    async def _raw_request(
        self,
        method: str,
        path: str,
        body_data: Optional[str],
        idempotency_key: str,
    ) -> Any:
        client = self._rest_client()
        headers = {
            "Authorization": f"Bearer {self._token}",
            "User-Agent": self._user_agent,
        }
        if body_data is not None:
            headers["Content-Type"] = "application/json"
        if idempotency_key:
            headers[IDEMPOTENCY_KEY_HEADER] = idempotency_key
        url = f"{self._server_url}{path}"
        try:
            resp = await client.request(method, url, content=body_data, headers=headers)
        except httpx.HTTPError as exc:
            return AlgaConnectionError(f"request failed: {exc}")
        text = resp.text
        if len(text) > MAX_RESPONSE_BYTES:
            text = text[:MAX_RESPONSE_BYTES]
        return (resp.status_code, text, resp.headers.get("Retry-After"))

    async def _sleep(self, seconds: float) -> None:
        if seconds <= 0:
            return
        await asyncio.sleep(seconds)

    async def _get(self, path: str, params: Optional[dict[str, str]] = None) -> Any:
        return await self._do_json("GET", _with_query(path, params), None, "")

    async def _post(self, path: str, payload: Any = None) -> Any:
        return await self._do_json("POST", path, payload, "")

    async def _post_idem(self, path: str, payload: Any, idempotency_key: str) -> Any:
        return await self._do_json("POST", path, payload, idempotency_key)

    async def _put(self, path: str, payload: Any = None) -> Any:
        return await self._do_json("PUT", path, payload, "")

    async def _patch(self, path: str, payload: Any = None) -> Any:
        return await self._do_json("PATCH", path, payload, "")

    async def _delete(self, path: str, payload: Any = None) -> Any:
        return await self._do_json("DELETE", path, payload, "")

    async def list_alerts(self, params: Optional[dict[str, str]] = None) -> list[Alert]:
        path = _with_query("/api/v1/agent/alerts", params)
        data = await self._get(path)
        return [Alert.model_validate(d) for d in _as_list(data)]

    async def get_alert(self, fingerprint: str) -> Alert:
        data = await self._get(f"/api/v1/agent/alerts/{_escape(fingerprint)}")
        return Alert.model_validate(data)

    async def resolve_alert(self, fingerprint: str) -> None:
        await self._post(f"/api/v1/agent/alerts/{_escape(fingerprint)}/resolve")

    async def reopen_alert(self, fingerprint: str) -> None:
        await self._post(f"/api/v1/agent/alerts/{_escape(fingerprint)}/reopen")

    async def get_incident(self, incident_number: int) -> IncidentContext:
        data = await self._get(f"/api/v1/agent/incidents/{incident_number}")
        return IncidentContext.model_validate(data)

    async def get_incident_timeline(self, incident_number: int) -> list[dict[str, Any]]:
        data = await self._get(f"/api/v1/agent/incidents/{incident_number}/timeline")
        return [d for d in _as_list(data) if isinstance(d, dict)]

    async def add_incident_timeline(
        self, incident_number: int, message: str, event_type: str
    ) -> None:
        await self._post(
            f"/api/v1/agent/incidents/{incident_number}/timeline",
            {"message": message, "event_type": event_type},
        )

    async def update_incident_summary(self, incident_number: int, summary: str) -> Incident:
        data = await self._patch(
            f"/api/v1/agent/incidents/{incident_number}", {"summary": summary}
        )
        return Incident.model_validate(data)

    async def send_message(
        self,
        chat_id: str,
        text: str,
        mentions: Optional[list[str]] = None,
    ) -> SendMessageResponse:
        return await self.send_message_with_key(chat_id, text, mentions, "")

    async def send_message_with_key(
        self,
        chat_id: str,
        text: str,
        mentions: Optional[list[str]] = None,
        idempotency_key: str = "",
    ) -> SendMessageResponse:
        payload: dict[str, Any] = {"chat_id": chat_id, "kind": "text", "text": text}
        if mentions:
            payload["mentions"] = mentions
        data = await self._post_idem(AGENT_MESSAGES_PATH, payload, idempotency_key)
        return SendMessageResponse.model_validate(data or {})

    async def send_command(
        self, chat_id: str, command: InvestigationCommand
    ) -> CommandResponse:
        return await self.send_command_with_key(chat_id, command, "")

    async def send_command_with_key(
        self,
        chat_id: str,
        command: InvestigationCommand,
        idempotency_key: str = "",
    ) -> CommandResponse:
        payload = {
            "chat_id": chat_id,
            "kind": "inv_tool",
            "command": command.model_dump(exclude_none=True),
        }
        data = await self._post_idem(AGENT_MESSAGES_PATH, payload, idempotency_key)
        return CommandResponse.model_validate(data or {})

    async def send_incident_summary(self, incident_number: int, text: str) -> None:
        await self._post(
            AGENT_MESSAGES_PATH,
            {
                "chat_id": f"incident_coord_{incident_number}",
                "kind": "incident_summary",
                "text": text,
            },
        )

    async def send_draft(self, chat_id: str, draft_id: str, text: str) -> None:
        await self._post(
            "/api/v1/agent/drafts",
            {"chat_id": chat_id, "draft_id": draft_id, "text": text},
        )

    async def edit_message(self, message_id: str, chat_id: str, text: str) -> None:
        await self._put(
            f"{AGENT_MESSAGES_PATH}/{_escape(message_id)}",
            {"chat_id": chat_id, "kind": "text", "text": text},
        )

    async def delete_message(self, message_id: str, chat_id: str) -> None:
        await self._delete(
            f"{AGENT_MESSAGES_PATH}/{_escape(message_id)}", {"chat_id": chat_id}
        )

    async def send_typing(self, chat_id: str, active: bool = True) -> None:
        await self._post("/api/v1/agent/typing", {"chat_id": chat_id, "active": active})

    async def send_heartbeat(self) -> None:
        await self._post("/api/v1/agent/heartbeat")

    async def list_knowledge(
        self, params: Optional[dict[str, str]] = None
    ) -> KnowledgeListResponse:
        path = _with_query("/api/v1/agent/knowledge", params)
        data = await self._get(path)
        return KnowledgeListResponse.model_validate(data or {})

    async def get_knowledge(self, id: str) -> KnowledgeNote:
        data = await self._get(f"/api/v1/agent/knowledge/{_escape(id)}")
        return KnowledgeNote.model_validate(data)

    async def create_knowledge(self, params: dict[str, Any]) -> KnowledgeNote:
        data = await self._post("/api/v1/agent/knowledge", params)
        return KnowledgeNote.model_validate(data)

    async def list_memories(
        self, params: Optional[dict[str, str]] = None
    ) -> MemoryListResponse:
        path = _with_query("/api/v1/agent/memories", params)
        data = await self._get(path)
        return MemoryListResponse.model_validate(data or {})

    async def create_memory(self, params: dict[str, Any]) -> Memory:
        data = await self._post("/api/v1/agent/memories", params)
        return Memory.model_validate(data)

    async def get_memory(self, id: str) -> Memory:
        data = await self._get(f"/api/v1/agent/memories/{_escape(id)}")
        return Memory.model_validate(data)

    async def delete_memory(self, id: str) -> None:
        await self._delete(f"/api/v1/agent/memories/{_escape(id)}")

    async def list_peer_asks(
        self, params: Optional[dict[str, str]] = None
    ) -> PeerAskListResponse:
        path = _with_query("/api/v1/agent/peer-ask", params)
        data = await self._get(path)
        return PeerAskListResponse.model_validate(data or {})

    async def create_peer_ask(self, params: dict[str, Any]) -> PeerAsk:
        data = await self._post("/api/v1/agent/peer-ask", params)
        return PeerAsk.model_validate(data)

    async def get_peer_ask(self, id: str) -> PeerAsk:
        data = await self._get(f"/api/v1/agent/peer-ask/{_escape(id)}")
        return PeerAsk.model_validate(data)

    async def reply_peer_ask(self, id: str, reply: str) -> PeerAsk:
        data = await self._post(
            f"/api/v1/agent/peer-ask/{_escape(id)}/reply", {"reply": reply}
        )
        return PeerAsk.model_validate(data)

    async def cancel_peer_ask(self, id: str) -> None:
        await self._post(f"/api/v1/agent/peer-ask/{_escape(id)}/cancel")

    async def list_services(
        self, params: Optional[dict[str, str]] = None
    ) -> ServiceListResponse:
        path = _with_query("/api/v1/agent/services", params)
        data = await self._get(path)
        return ServiceListResponse.model_validate(data or {})

    async def who_is_on_call(self) -> list[OnCallEntry]:
        data = await self._get("/api/v1/agent/on-call/current")
        return [OnCallEntry.model_validate(d) for d in _as_list(data)]

    async def get_playbooks(self, alert_fingerprint: str) -> list[Playbook]:
        data = await self._get(
            "/api/v1/agent/playbooks", {"alert_fingerprint": alert_fingerprint}
        )
        return [Playbook.model_validate(d) for d in _as_list(data)]

    async def get_secret(self, secret_id: str) -> SecretValue:
        data = await self._get(f"/api/v1/agent/secrets/{_escape(secret_id)}")
        return SecretValue.model_validate(data)


def _with_query(path: str, params: Optional[dict[str, str]]) -> str:
    if not params:
        return path
    pairs: list[tuple[str, str]] = []
    for k, v in params.items():
        if v is None or v == "":
            continue
        pairs.append((k, str(v)))
    if not pairs:
        return path
    return f"{path}?{urlencode(pairs)}"


def _unwrap_envelope(body: str) -> Any:
    try:
        parsed = json.loads(body)
    except (ValueError, TypeError):
        return None
    if isinstance(parsed, dict) and "data" in parsed and parsed["data"] is not None:
        return parsed["data"]
    return parsed


def _truncate(s: str, n: int) -> str:
    if len(s) <= n:
        return s
    return s[:n]


def _escape(segment: str) -> str:
    return quote(segment, safe="")


def _as_list(data: Any) -> list[Any]:
    if isinstance(data, list):
        return data
    if isinstance(data, dict):
        for key in ("items", "alerts", "tasks", "services", "schedules"):
            value = data.get(key)
            if isinstance(value, list):
                return value
    return []


def _new_idempotency_key() -> str:
    return "alga-" + secrets.token_hex(16)


def _backoff_for(attempt: int, retry_after: timedelta) -> float:
    if retry_after.total_seconds() > 0:
        return min(retry_after.total_seconds(), 600.0)
    base = min(float(1 << attempt), 30.0)
    jitter = random.random() * base * 0.2
    return base + jitter
