from __future__ import annotations

import mimetypes
from pathlib import Path
from typing import Any
from urllib.parse import quote

import httpx

from .commands import InvestigationCommand
from .dedup import MessageDedup
from .errors import AlgaAPIError, AlgaAuthError, AlgaConnectionError
from .models import (
    Alert,
    AlertListResponse,
    Capability,
    CommandResponse,
    Incident,
    Investigation,
    InvestigationListResponse,
    KnowledgeListResponse,
    KnowledgeNote,
    Memory,
    MemoryListResponse,
    PeerAsk,
    PeerAskListResponse,
    Playbook,
    SendMessageResponse,
    Service,
)
from .sse import EventHandler, SSEClient


class AlgaClient:
    def __init__(
        self,
        server_url: str,
        token: str,
        heartbeat_interval: float = 30.0,
        dedup: MessageDedup | None = None,
    ):
        self._server_url = server_url.rstrip("/")
        self._token = token
        self._heartbeat_interval = heartbeat_interval
        self._dedup = dedup
        self._http: httpx.AsyncClient | None = None
        self._sse: SSEClient | None = None

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

    async def connect(self) -> None:
        self._http = httpx.AsyncClient(
            base_url=self._server_url,
            headers={"Authorization": f"Bearer {self._token}"},
            timeout=httpx.Timeout(30.0),
        )
        self._sse = SSEClient(
            http_base=self._server_url,
            token=self._token,
            dedup=self._dedup,
            heartbeat_interval=self._heartbeat_interval,
        )
        self._sse.on_connected = self.on_connected
        self._sse.on_message = self.on_message
        self._sse.on_typing = self.on_typing
        self._sse.on_investigation_cancel = self.on_investigation_cancel
        self._sse.on_investigation_pause = self.on_investigation_pause
        self._sse.on_investigation_resume = self.on_investigation_resume
        self._sse.on_peer_finding = self.on_peer_finding
        self._sse.on_peer_ask = self.on_peer_ask
        self._sse.on_peer_reply = self.on_peer_reply
        self._sse.on_agent_presence = self.on_agent_presence
        await self._sse.start()

    async def disconnect(self) -> None:
        if self._sse:
            await self._sse.stop()
            self._sse = None
        if self._http:
            await self._http.aclose()
            self._http = None

    async def wait(self) -> None:
        if self._sse:
            await self._sse.wait()

    def _ensure_client(self) -> httpx.AsyncClient:
        if self._http is None:
            raise AlgaConnectionError("Not connected — call connect() first")
        return self._http

    async def _request(self, method: str, path: str, **kwargs: Any) -> httpx.Response:
        client = self._ensure_client()
        try:
            resp = await client.request(method, path, **kwargs)
        except httpx.HTTPError as exc:
            raise AlgaConnectionError(str(exc)) from exc
        if resp.status_code in (401, 403):
            raise AlgaAuthError(resp.status_code, resp.text)
        if resp.status_code >= 400:
            raise AlgaAPIError(resp.status_code, resp.text)
        return resp

    async def _get_json(self, path: str, **kwargs: Any) -> Any:
        resp = await self._request("GET", path, **kwargs)
        return resp.json()

    async def _post_json(self, path: str, **kwargs: Any) -> Any:
        resp = await self._request("POST", path, **kwargs)
        return resp.json()

    async def _put_json(self, path: str, **kwargs: Any) -> Any:
        resp = await self._request("PUT", path, **kwargs)
        return resp.json()

    async def _delete_json(self, path: str, **kwargs: Any) -> Any:
        resp = await self._request("DELETE", path, **kwargs)
        if resp.status_code == 204 or not resp.content:
            return {}
        return resp.json()

    async def list_alerts(
        self,
        *,
        status: str | None = None,
        severity: str | None = None,
        search: str | None = None,
        start_date: str | None = None,
        end_date: str | None = None,
        limit: int | None = None,
        skip: int | None = None,
    ) -> AlertListResponse:
        params = self._build_params(
            status=status, severity=severity, search=search,
            start_date=start_date, end_date=end_date,
            limit=limit, skip=skip,
        )
        data = await self._get_json("/api/v1/agent/alerts", params=params)
        return AlertListResponse.model_validate(data)

    async def get_alert(self, fingerprint: str) -> Alert:
        data = await self._get_json(f"/api/v1/agent/alerts/{quote(fingerprint, safe='')}")
        return Alert.model_validate(data)

    async def resolve_alert(self, fingerprint: str) -> Alert:
        data = await self._post_json(f"/api/v1/agent/alerts/{quote(fingerprint, safe='')}/resolve")
        return Alert.model_validate(data)

    async def reopen_alert(self, fingerprint: str) -> Alert:
        data = await self._post_json(f"/api/v1/agent/alerts/{quote(fingerprint, safe='')}/reopen")
        return Alert.model_validate(data)

    async def list_investigations(
        self,
        *,
        status: str | None = None,
        severity: str | None = None,
        search: str | None = None,
        limit: int | None = None,
        skip: int | None = None,
    ) -> InvestigationListResponse:
        params = self._build_params(
            status=status, severity=severity, search=search,
            limit=limit, skip=skip,
        )
        data = await self._get_json("/api/v1/agent/investigations", params=params)
        return InvestigationListResponse.model_validate(data)

    async def get_investigation(self, investigation_id: int) -> Investigation:
        data = await self._get_json(f"/api/v1/agent/investigations/{investigation_id}")
        return Investigation.model_validate(data)

    async def post_update(
        self,
        investigation_id: int,
        update_type: str,
        message: str,
    ) -> Investigation:
        await self._post_json(
            f"/api/v1/agent/investigations/{investigation_id}/updates",
            json={"type": update_type, "message": message},
        )
        return await self.get_investigation(investigation_id)

    async def send_message(
        self,
        chat_id: str,
        text: str,
        *,
        mentions: list[str] | None = None,
    ) -> SendMessageResponse:
        body: dict[str, Any] = {"chat_id": chat_id, "kind": "text", "text": text}
        if mentions:
            body["mentions"] = mentions
        data = await self._post_json("/api/v1/agent/messages", json=body)
        return SendMessageResponse.model_validate(data)

    async def send_command(
        self,
        chat_id: str,
        command: InvestigationCommand,
    ) -> CommandResponse:
        body: dict[str, Any] = {
            "chat_id": chat_id,
            "kind": "inv_tool",
            "command": command.to_dict(),
        }
        data = await self._post_json("/api/v1/agent/messages", json=body)
        return CommandResponse.model_validate(data)

    async def edit_message(
        self,
        message_id: str,
        chat_id: str,
        text: str,
    ) -> None:
        await self._put_json(
            f"/api/v1/agent/messages/{message_id}",
            json={"chat_id": chat_id, "kind": "text", "text": text},
        )

    async def delete_message(self, message_id: str, chat_id: str) -> None:
        await self._delete_json(
            f"/api/v1/agent/messages/{message_id}",
            json={"chat_id": chat_id},
        )

    async def send_typing(self, chat_id: str, active: bool = True) -> None:
        await self._post_json("/api/v1/agent/typing", json={"chat_id": chat_id, "active": active})

    async def send_heartbeat(self) -> None:
        await self._post_json("/api/v1/agent/heartbeat")

    async def list_knowledge(
        self,
        *,
        query: str | None = None,
        kind: str | None = None,
        tag: str | None = None,
        limit: int | None = None,
        skip: int | None = None,
    ) -> KnowledgeListResponse:
        params = self._build_params(query=query, kind=kind, tag=tag, limit=limit, skip=skip)
        data = await self._get_json("/api/v1/agent/knowledge", params=params)
        return KnowledgeListResponse.model_validate(data)

    async def create_knowledge(
        self,
        kind: str,
        title: str,
        body_markdown: str,
        *,
        tags: list[str] | None = None,
        source_investigation_id: int | None = None,
        confidence: float | None = None,
    ) -> KnowledgeNote:
        body: dict[str, Any] = {
            "kind": kind,
            "title": title,
            "body_markdown": body_markdown,
        }
        if tags is not None:
            body["tags"] = tags
        if source_investigation_id is not None:
            body["source_investigation_id"] = source_investigation_id
        if confidence is not None:
            body["confidence"] = confidence
        data = await self._post_json("/api/v1/agent/knowledge", json=body)
        return KnowledgeNote.model_validate(data)

    async def list_memories(
        self,
        *,
        query: str | None = None,
        limit: int | None = None,
        skip: int | None = None,
    ) -> MemoryListResponse:
        params = self._build_params(query=query, limit=limit, skip=skip)
        data = await self._get_json("/api/v1/agent/memories", params=params)
        return MemoryListResponse.model_validate(data)

    async def create_memory(
        self,
        content: str,
        *,
        memory_type: str | None = None,
        investigation_id: int | None = None,
        correlation_key: str | None = None,
        labels: dict[str, str] | None = None,
        confidence: float | None = None,
        expires_at: str | None = None,
    ) -> Memory:
        body: dict[str, Any] = {"content": content}
        if memory_type is not None:
            body["memory_type"] = memory_type
        if investigation_id is not None:
            body["investigation_id"] = investigation_id
        if correlation_key is not None:
            body["correlation_key"] = correlation_key
        if labels is not None:
            body["labels"] = labels
        if confidence is not None:
            body["confidence"] = confidence
        if expires_at is not None:
            body["expires_at"] = expires_at
        data = await self._post_json("/api/v1/agent/memories", json=body)
        return Memory.model_validate(data)

    async def get_memory(self, memory_id: int) -> Memory:
        data = await self._get_json(f"/api/v1/agent/memories/{memory_id}")
        return Memory.model_validate(data)

    async def delete_memory(self, memory_id: int) -> None:
        await self._delete_json(f"/api/v1/agent/memories/{memory_id}")

    async def list_peer_asks(
        self,
        *,
        role: str = "inbox",
        status: str | None = None,
        limit: int | None = None,
        skip: int | None = None,
    ) -> PeerAskListResponse:
        params = self._build_params(role=role, status=status, limit=limit, skip=skip)
        data = await self._get_json("/api/v1/agent/peer-ask", params=params)
        return PeerAskListResponse.model_validate(data)

    async def create_peer_ask(
        self,
        question: str,
        *,
        to_agent_id: str | None = None,
        to_agent_type: str | None = None,
        investigation_id: int | None = None,
        timeout_seconds: int = 600,
    ) -> PeerAsk:
        body: dict[str, Any] = {
            "question": question,
            "timeout_seconds": timeout_seconds,
        }
        if to_agent_id is not None:
            body["to_agent_id"] = to_agent_id
        if to_agent_type is not None:
            body["to_agent_type"] = to_agent_type
        if investigation_id is not None:
            body["investigation_id"] = investigation_id
        data = await self._post_json("/api/v1/agent/peer-ask", json=body)
        return PeerAsk.model_validate(data)

    async def get_peer_ask(self, ask_id: int) -> PeerAsk:
        data = await self._get_json(f"/api/v1/agent/peer-ask/{ask_id}")
        return PeerAsk.model_validate(data)

    async def reply_peer_ask(self, ask_id: int, reply: str) -> PeerAsk:
        data = await self._post_json(
            f"/api/v1/agent/peer-ask/{ask_id}/reply",
            json={"reply": reply},
        )
        return PeerAsk.model_validate(data)

    async def cancel_peer_ask(self, ask_id: int) -> None:
        await self._post_json(f"/api/v1/agent/peer-ask/{ask_id}/cancel")

    async def get_incident(self, incident_id: int) -> Incident:
        data = await self._get_json(f"/api/v1/agent/incidents/{incident_id}")
        return Incident.model_validate(data)

    async def add_incident_timeline(
        self,
        incident_id: int,
        message: str,
        event_type: str | None = None,
    ) -> None:
        body: dict[str, Any] = {"message": message}
        if event_type is not None:
            body["event_type"] = event_type
        await self._post_json(f"/api/v1/agent/incidents/{incident_id}/timeline", json=body)

    async def send_incident_summary(self, incident_id: str, text: str) -> None:
        await self._post_json(
            "/api/v1/agent/messages",
            json={
                "chat_id": f"incident_coord_{incident_id}",
                "kind": "incident_summary",
                "text": text,
            },
        )

    async def list_services(self) -> list[Service]:
        data = await self._get_json("/api/v1/agent/services")
        if isinstance(data, list):
            return [Service.model_validate(s) for s in data]
        items = data.get("services", data.get("items", []))
        return [Service.model_validate(s) for s in items]

    async def who_is_on_call(self) -> list[dict[str, Any]]:
        data = await self._get_json("/api/v1/agent/on-call/current")
        if isinstance(data, list):
            return data
        return data.get("schedules", data.get("items", []))

    async def get_capabilities(self) -> list[Capability]:
        data = await self._get_json("/api/v1/agent/capabilities")
        if isinstance(data, list):
            return [Capability.model_validate(c) for c in data]
        return [Capability.model_validate(c) for c in data.get("items", data.get("capabilities", []))]

    async def get_playbooks(self, alert_fingerprint: str | None = None) -> list[Playbook]:
        params = self._build_params(alert_fingerprint=alert_fingerprint)
        data = await self._get_json("/api/v1/agent/playbooks", params)
        if isinstance(data, list):
            return [Playbook.model_validate(p) for p in data]
        return [Playbook.model_validate(p) for p in data.get("items", [])]

    async def upload_media(self, file_path: str) -> dict[str, Any]:
        client = self._ensure_client()
        path = Path(file_path)
        mime = mimetypes.guess_type(file_path)[0] or "application/octet-stream"
        try:
            with open(file_path, "rb") as f:
                files = {"file": (path.name, f, mime)}
                resp = await client.post("/api/v1/agent/media", files=files)
        except httpx.HTTPError as exc:
            raise AlgaConnectionError(str(exc)) from exc
        if resp.status_code in (401, 403):
            raise AlgaAuthError(resp.status_code, resp.text)
        if resp.status_code >= 400:
            raise AlgaAPIError(resp.status_code, resp.text)
        return resp.json()

    @staticmethod
    def _build_params(**kwargs: Any) -> dict[str, Any]:
        return {k: v for k, v in kwargs.items() if v is not None}
