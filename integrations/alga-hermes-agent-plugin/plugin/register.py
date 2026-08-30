"""
Alga Platform Plugin for Hermes Agent.

  Self-contained plugin that registers:
  - Platform adapter (SSE + REST) for Alga alert and incident threads
  - 21 agent tools (acknowledge, resolve, knowledge, incident lifecycle, etc.)

Installation: drop into ``~/.hermes/plugins/alga-platform/`` and enable with
``hermes plugins enable alga-platform``.
"""

from __future__ import annotations

import asyncio
import json
import logging
import os
import random
import stat
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional
from urllib.parse import urlsplit, urlunsplit

from gateway.config import Platform, PlatformConfig
from gateway.platforms.base import (
    BasePlatformAdapter,
    MessageEvent,
    MessageType,
    SendResult,
)
from gateway.platforms.helpers import MessageDeduplicator

logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

MAX_MESSAGE_LENGTH = 4000

_RECONNECT_BASE_DELAY = 2.0
_RECONNECT_MAX_DELAY = 60.0
_RECONNECT_JITTER = 0.2

_SSE_PATH = "/api/v1/agent/events"
_HEARTBEAT_PATH = "/api/v1/agent/heartbeat"
_MESSAGES_PATH = "/api/v1/agent/messages"
_DRAFTS_PATH = "/api/v1/agent/drafts"
_TYPING_PATH = "/api/v1/agent/typing"
_ALERTS_PATH = "/api/v1/agent/alerts"
_KNOWLEDGE_PATH = "/api/v1/agent/knowledge"

_HEARTBEAT_INTERVAL = 30.0
_HEARTBEAT_FAIL_THRESHOLD = 5

_ALGA_TOOLSET = "alga"

_SSE_EVENTS_FOR_AGENT = frozenset({
    "message",
    "investigation_dispatch",
    "investigation_status_changed",
    "investigation_patch",
})


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _normalize_alga_url(raw: str) -> str:
    raw = (raw or "").strip()
    if not raw:
        return ""
    if "://" not in raw:
        raw = "https://" + raw
    u = urlsplit(raw)
    scheme = (u.scheme or "http").lower()
    netloc = u.netloc
    if not netloc:
        return ""
    if scheme in ("ws", "wss"):
        scheme = "https" if scheme == "wss" else "http"
    if scheme not in ("http", "https"):
        return ""
    return urlunsplit((scheme, netloc, "", "", "")).rstrip("/")


def _resolve_server_url(extra: Optional[Dict[str, Any]] = None) -> str:
    ex = dict(extra or {})
    raw = (
        str(ex.get("server_url") or "").strip()
        or os.getenv("ALGA_SERVER_URL", "").strip()
    )
    return _normalize_alga_url(raw) if raw else ""


def resolve_alga_endpoints(extra: Optional[Dict[str, Any]] = None) -> tuple[str, str]:
    h = _resolve_server_url(extra)
    return h, h


def resolve_alga_http_base(extra: Optional[Dict[str, Any]] = None) -> str:
    return resolve_alga_endpoints(extra)[1]


def _strip_chat_prefix(chat_id: str) -> str:
    chat_id = (chat_id or "").strip()
    if chat_id.startswith("alga:"):
        return chat_id[len("alga:"):]
    return chat_id


def _normalize_chat_id(chat_id: str) -> str:
    return _strip_chat_prefix(chat_id)


def _extract_reply_to_context(data: dict) -> tuple[Optional[str], Optional[str]]:
    """Extract (reply_to_message_id, reply_to_text) from an inbound SSE event.

    The Alga backend persists ``reply_to_message_id`` on thread messages. When
    the backend forwards a reply to the agent over SSE it is expected to
    include ``reply_to_message_id`` plus the replied-to message's text.

    Returns (None, None) when no reply-to is present, so callers can populate
    MessageEvent fields unconditionally.
    """
    if not isinstance(data, dict):
        return None, None
    reply_to_id = data.get("reply_to_message_id") or ""
    reply_to_id = str(reply_to_id).strip() if reply_to_id else ""
    reply_to_text = data.get("reply_to_text") or ""
    reply_to_text = str(reply_to_text).strip() if reply_to_text else ""
    if not reply_to_id:
        return None, None
    return (reply_to_id or None), (reply_to_text or None)


_INCIDENT_ROLE_LABELS = {
    "incident_commander": "Incident Commander",
    "communications_lead": "Communications Lead",
    "responder": "Responder",
    "communicator": "Communications Lead",
}

_INCIDENT_ROLE_INSTRUCTIONS = {
    "incident_commander": (
        "You own incident direction, escalation decisions, final calls, and documentation quality. "
        "Commander tools: alga_set_incident_priority, alga_set_incident_severity, alga_trigger_escalation, "
        "alga_publish_status_update, alga_mitigate_incident, alga_resolve_incident, alga_resolve_alert, alga_reopen_alert, alga_begin_triage, "
        "alga_promote_incident, alga_assign_incident_role. "
        "You are the orchestrator. Do not investigate directly. Delegate technical work by @mentioning responder agents in "
        "the coordination thread with a bounded, specific goal (one goal per mention — a mention activates the agent), and "
        "assign child incident investigations when parallel technical work is needed. Track progress through the coordination "
        "thread, investigation summaries, and the Status Updates card. "
        "You are strictly forbidden from performing technical validation, running commands against the environment, "
        "or inspecting environment/service health. All technical work, commands, and validation must be handled by the Responder. "
        "Alert state checks (alga_list_alerts) and alert closure (alga_resolve_alert, alga_reopen_alert) are part of incident closure, not technical actions, and are owned by you. "
        "Directly publish public status updates via alga_publish_status_update at key milestones "
        "(investigating, identified, mitigated, monitoring, or resolved) instead of delegating to a communicator. "
        "Final resolution is blocked until a public status update with status_level='resolved' is published. "
        "Verify responder recovery and ensure the resolved status update is posted before calling alga_resolve_incident. "
        "When resolving the incident, you MUST fill all five resolution fields: a SHORT executive summary (summary field — 3-6 sentences of plain narrative prose for non-technical stakeholders, NO labels or section headings like \"Root Cause:\"/\"Trigger:\"/\"Recovery:\", and NO duplication of technical detail from the other sections); a dedicated root_cause field describing WHY the incident happened; a resolution field describing the concrete remediation/recovery applied; an impact_assessment field; and an actions_taken field of environment actions as markdown bullets. root_cause and resolution are mandatory incident document sections — resolution will fail with 422 if either is empty. Stage them early with alga_set_incident_resolution_docs while still mitigated. "
        "Do NOT @mention the Responder or any other teammate in appreciation, acknowledgement, sign-off, or recap. An @mention activates them and forces a ping-pong reply. When you have accepted the Responder's handoff, either act on it (verify and close the linked alert with alga_resolve_alert, publish the resolved status update, call alga_resolve_incident, set alga_set_incident_resolution_docs) or post a brief coordination reply without any mentions."
    ),
    "communications_lead": (
        "You publish public status updates when asked. You are activated by @mentions in the incident coordination thread and "
        "by comms_stale nudges (the SLA sweep nudging you that a status update is overdue). When activated, publish a "
        "public-facing status update with alga_publish_status_update using the requested status_level (investigating, "
        "identified, mitigated, monitoring, resolved) and reply in the thread confirming what was published. Do not mention "
        "internal alert numbers, agent names, investigation IDs, UUIDs, or placeholder/test labels in public status text. "
        "Communication tools: alga_publish_status_update. "
        "Do NOT @mention other agents or teammates unless absolutely necessary."
    ),
    "communicator": (
        "You publish public status updates when asked. You are activated by @mentions in the incident coordination thread and "
        "by comms_stale nudges (the SLA sweep nudging you that a status update is overdue). When activated, publish a "
        "public-facing status update with alga_publish_status_update using the requested status_level (investigating, "
        "identified, mitigated, monitoring, resolved) and reply in the thread confirming what was published. Do not mention "
        "internal alert numbers, agent names, investigation IDs, UUIDs, or placeholder/test labels in public status text. "
        "Communication tools: alga_publish_status_update. "
        "Do NOT @mention other agents or teammates unless absolutely necessary."
    ),
    "responder": (
        "You focus on technical recovery, safe mitigation, evidence, and readiness for commander verification. "
        "You are the sole owner of technical validation and environment investigation. "
        "You are activated when the scheduler dispatches an incident investigation to you and when the commander @mentions you "
        "in the coordination thread. Investigate the assigned goal and publish milestone progress via "
        "alga_publish_status_update. Do not delegate work yourself — delegation belongs to the commander. "
        "Responder tools: alga_publish_status_update, alga_post_handoff, alga_pause_investigation, alga_cancel_investigation. "
        "You are FORBIDDEN from calling alga_resolve_alert or alga_reopen_alert on alerts linked to an active incident — "
        "alert closure is part of incident closure and is owned by the incident commander. The server will reject "
        "your call with a role-guard error if you try; do not retry or attempt the call from another thread. "
        "You are also FORBIDDEN from calling alga_who_is_on_call to identify who to hand off to. You do not need to "
        "look up who is on call; the handoff in the investigation thread (using alga_post_handoff with audience='commander') "
        "is always directly to the incident commander. "
        "This is the incident COORDINATION chat — do NOT post technical investigation logs, terminal output, "
        "or step-by-step debugging here. Post SRE investigation details and terminal outputs in the alert investigation chat. "
        "Coordination-tool discipline (NON-NEGOTIABLE, violations break the incident): "
        "alga_post_handoff ACTIVATES other agents (commander, communicator) on every call — "
        "it interrupts their current work and triggers ping-pong message loops. You are FORBIDDEN from calling "
        "alga_post_handoff during investigation, identification, mitigation, or verification. "
        "ALL status communication while you are still working MUST go through alga_publish_status_update, which "
        "posts to the Status Updates card WITHOUT activating any other agent. The commander monitors the Status "
        "Updates card and will act on your 'mitigated' or 'monitoring' update. You may call alga_post_handoff at most ONCE "
        "— only for the single final commander handoff after ALL recovery and verification is complete and you "
        "have already published status_level='identified' AND (status_level='mitigated' OR status_level='monitoring'). "
        "Never call it to post findings, progress notes, or interim summaries. "
        "Status-level discipline: you may publish status_level='identified', 'mitigated', and 'monitoring' ONLY. "
        "You are FORBIDDEN from publishing status_level='resolved' (commander-only) or status_level='investigating' (system-only). "
        "Each milestone is published EXACTLY ONCE per incident — do not re-publish 'mitigated' or 'monitoring' as a verification update; the commander will publish 'resolved' once they verify and resolve. "
        "Workflow: Interleave technical steps and milestone status updates. "
        "First, investigate the environment. As soon as you identify the root cause, you MUST immediately call alga_publish_status_update with status_level='identified' to post to the 'Status Updates' card/feed before performing mitigation. "
        "Second, apply the recovery/mitigation steps. Once the fix is applied and impact is reduced, you MUST immediately call alga_publish_status_update with status_level='mitigated' to post to the 'Status Updates' card/feed. "
        "Third, if the fix needs time to confirm full recovery (e.g. waiting on replication to settle or a multi-hour soak test), call alga_publish_status_update with status_level='monitoring' to mark the verification phase. SKIP monitoring entirely if the fix is fully verified — 'mitigated' alone is sufficient for handoff. "
        "Do NOT publish a 'resolved' or 'investigating' status update — the initial 'investigating' update is posted by the system, and the 'resolved' update is published by the commander when resolving the incident. "
        "Do NOT post milestone updates as free-text coordination-thread messages; "
        "you must publish status updates to the 'Status Updates' card yourself using alga_publish_status_update immediately. "
        "Ensure status update messages are public-facing, describing user-visible impact and current service status while "
        "avoiding internal alert numbers, agent names, investigation IDs, UUIDs, or placeholders. "
        "Do NOT list technical validation, log checks, or recovery steps under 'Required Actions (for incident commander)' in handoff summaries or coordination updates. All technical tasks must be directed to the Responder; the Commander coordinates and verifies but never runs commands. "
        "Do NOT @mention other agents or teammates unless absolutely necessary to request an action or handoff. Too many unnecessary real agent mentions activate the agent."
    ),
}

_ALERT_INVESTIGATION_TOOLS = frozenset({
    "alga_search_knowledge",
    "alga_get_knowledge",
    "alga_create_knowledge",
    "alga_list_alerts",
    "alga_list_services",
    "alga_set_outcome",
    "alga_resolve_alert",
    "alga_reopen_alert",
    "alga_promote_to_incident",
    "alga_pause_investigation",
    "alga_cancel_investigation",
})

_INCIDENT_TOOLS = frozenset({
    "alga_set_incident_priority",
    "alga_set_incident_severity",
    "alga_trigger_escalation",
    "alga_mitigate_incident",
    "alga_resolve_incident",
    "alga_set_incident_resolution_docs",
    "alga_begin_triage",
    "alga_promote_incident",
    "alga_add_incident_timeline",
    "alga_assign_incident_role",
    "alga_get_incident_context",
    "alga_get_incident_timeline",
    "alga_post_handoff",
    "alga_publish_status_update",
})


def _incident_role_context_prefix(incident_role: str, incident_id: str, incident_status: str, chat_id: str = "") -> str:
    incident_role = (incident_role or "").strip()
    if not incident_role:
        return ""
    label = _INCIDENT_ROLE_LABELS.get(incident_role, incident_role)
    instructions = _INCIDENT_ROLE_INSTRUCTIONS.get(incident_role, "")
    chat_target = chat_id if chat_id else (f"incident_coord_{incident_id}" if incident_id else "this incident chat")
    parts = [
        f"[ALGA INCIDENT CONTEXT — Incident #{incident_id or '?'} — Status: {incident_status or 'unknown'}]",
        f"Your incident role: {label}.",
    ]
    if instructions:
        parts.append(instructions)
    # In incident scope, advertise the role-filtered tool list so the agent does
    # not waste a round-trip calling tools it is not authorized to use
    # (e.g. alga_resolve_alert for the responder — server-side role guard will
    # reject anyway, but listing only the allowed tools here avoids the
    # confusion and keeps the LLM's tool picker honest).
    if chat_id and (chat_id.startswith("incident_coord_") or chat_id.startswith("incident_inv_") or chat_id.startswith("incident_")):
        allowed = sorted(_allowed_alga_tools_for_chat(chat_id, incident_role))
        parts.append("Allowed Alga tools in this incident thread: " + ", ".join(f"`{name}`" for name in allowed) + ".")
        parts.append("Do NOT call any other Alga tool from this incident thread; even if the global Hermes tool list exposes them, calls outside the allowed set will be rejected.")
    parts.append(f"Reply in this chat ({chat_target}). Use your incident tools to act.")
    parts.append("Do NOT reply to this message in any other channel or platform.")
    parts.append(
        "Refer to the incident and alerts by NUMBER only (e.g. \"Incident #" + str(incident_id or "?") +
        "\", \"Alert #42\"); never mention or surface investigation IDs or UUIDs. "
        "Do NOT @mention another agent or teammate unless absolutely necessary to request an action or handoff. "
        "Do NOT mention them in status updates, replies, findings, or handoffs. Mentioning other agents activates their models, which causes unnecessary message loops. A simple reply without a mention is preferred when checking in or closing, or no reply at all. "
        "To address a teammate, reuse the exact mention form you saw in-thread, a \"[@Name](agent:UUID)\" link "
        "where UUID looks like \"123e4567-e89b-12d3-a456-426614174000\" (never wrap the UUID in angle brackets or quotes). "
        "Do NOT invent teammate names, roles, or UUIDs. Never use template/placeholder names or hypothetical UUIDs like \"123e4567-e89b-12d3-a456-426614174000\" in mentions. Only mention a teammate if you have retrieved their exact Agent ID or User ID from the active roles list (via alga_get_incident_context or previous in-thread messages). If a teammate is not in the active roles list, do NOT reference or mention them; "
        "do NOT invent role abbreviations like @ic, @comms, or @cmd — they are not valid mentions and won't resolve."
    )
    parts.append("[END ALGA INCIDENT CONTEXT]")
    return "\n".join(parts)


def _allowed_alga_tools_for_chat(chat_id: str, incident_role: str = "") -> set[str]:
    chat_id = _normalize_chat_id(chat_id)
    if chat_id.startswith("alert_"):
        return set(_ALERT_INVESTIGATION_TOOLS)
    # In incident scope, alert closure is commander-only. Filter alga_resolve_alert,
    # alga_reopen_alert, and alga_who_is_on_call out of the responder's allowed list so the agent is
    # not tempted to call them (the server-side role guard will reject anyway,
    # but listing them here keeps the agent from wasting a round-trip).
    tools = {tool["name"] for tool in _ALGA_TOOLS}
    if chat_id.startswith("incident_") and incident_role == "responder":
        tools = tools - {"alga_resolve_alert", "alga_reopen_alert", "alga_who_is_on_call"}
    return tools


def _alert_investigation_context_prefix(chat_id: str) -> str:
    chat_id = _normalize_chat_id(chat_id)
    if not chat_id.startswith("alert_"):
        return ""
    allowed = sorted(_allowed_alga_tools_for_chat(chat_id))
    return "\n".join([
        f"[ALGA ALERT INVESTIGATION CONTEXT — {chat_id}]",
        "This is an alert investigation thread, not an incident coordination thread.",
        "Allowed Alga tools in this thread: " + ", ".join(f"`{name}`" for name in allowed) + ".",
        "Do NOT call incident tools in this alert thread, even if the global Hermes tool list exposes them.",
        "If you promote the alert to an incident, report the incident number and stop; do not assign incident roles, read incident context, post incident coordination updates, or resolve the incident from this alert thread.",
        "Verify the alert before promoting — do not blindly promote. Once you have confirmed the alert is genuine and still firing, promote when EITHER (a) the alert runbook (retrieved via `alga_get_knowledge`) specifies promotion criteria — including mandatory/immediate promotion (e.g. \"if any node is down\") — OR (b) there is real, current user-facing impact (failing requests, error budget burn, customer-visible errors, blocked deploys). A runbook is not required: when no runbook matches, promote on confirmed user-facing impact. Call `alga_promote_to_incident` before or alongside mitigation in both cases.",
        "Do NOT list technical validation, log checks, or recovery steps under \"Required Actions (for incident commander)\" in handoff summaries or coordination updates. All technical tasks must be directed to the Responder; the Commander coordinates and verifies but never runs commands.",
        "[END ALGA ALERT INVESTIGATION CONTEXT]",
    ])


def check_requirements() -> bool:
    api = _resolve_server_url({})
    token = os.getenv("ALGA_AGENT_TOKEN", "")
    if not api:
        return False
    if not token:
        return False
    try:
        import httpx  # noqa: F401
        return True
    except ImportError:
        return False


def validate_config(config) -> bool:
    extra = getattr(config, "extra", {}) or {}
    token = getattr(config, "token", "") or os.getenv("ALGA_AGENT_TOKEN", "")
    server_url = extra.get("server_url", "") or os.getenv("ALGA_SERVER_URL", "")
    return bool(server_url and token)


def is_connected(config) -> bool:
    return validate_config(config)


# ---------------------------------------------------------------------------
# Shared tool HTTP client
# ---------------------------------------------------------------------------

_shared_client: Any = None


def _get_shared_client():
    global _shared_client
    import httpx
    base = _tool_http_base()
    if not base:
        return None
    if _shared_client is not None:
        old_base = getattr(_shared_client, "_alga_base", "")
        if old_base == base:
            return _shared_client
        try:
            import asyncio
            loop = asyncio.get_event_loop()
            if loop.is_running():
                loop.create_task(_shared_client.aclose())
            else:
                loop.run_until_complete(_shared_client.aclose())
        except Exception:
            pass
    _shared_client = httpx.AsyncClient(
        base_url=base,
        headers=_rest_headers(),
        timeout=httpx.Timeout(30.0, connect=10.0),
    )
    _shared_client._alga_base = base
    return _shared_client


def _tool_http_base() -> str:
    return _resolve_server_url({})


def _token() -> str:
    return os.getenv("ALGA_AGENT_TOKEN", "").strip()


def _rest_headers() -> dict:
    return {"Authorization": f"Bearer {_token()}", "Content-Type": "application/json"}


async def _agent_get(path: str, params: Optional[Dict[str, Any]] = None) -> Any:
    client = _get_shared_client()
    if client is None:
        return {"ok": False, "error": "Alga server URL not configured"}
    resp = await client.get(path, params=params or {})
    if resp.status_code >= 400:
        return {"ok": False, "error": f"HTTP {resp.status_code}: {resp.text[:200]}"}
    return resp.json()


async def _agent_post(path: str, body: Dict[str, Any]) -> Any:
    client = _get_shared_client()
    if client is None:
        return {"ok": False, "error": "Alga server URL not configured"}
    resp = await client.post(path, json=body)
    if resp.status_code >= 400:
        return {"ok": False, "error": f"HTTP {resp.status_code}: {resp.text[:200]}"}
    return resp.json()


async def _inv_tool(chat_id: str, command: Dict[str, Any]) -> str:
    result = await _agent_post(
        _MESSAGES_PATH,
        {"chat_id": chat_id, "kind": "inv_tool", "command": command},
    )
    if result.get("ok") is False:
        return json.dumps({"error": f"{command.get('op', 'unknown')}: {result.get('error', 'unknown')}"})
    out: Dict[str, Any] = {"success": True, "op": command.get("op"), "chat_id": chat_id}
    for key in ("incident_id", "incident_number", "task_id"):
        if key in result:
            out[key] = result[key]
    return json.dumps(out)


def _check_tool_availability() -> bool:
    return bool(_tool_http_base()) and bool(_token())


# ---------------------------------------------------------------------------
# AlgaAdapter
# ---------------------------------------------------------------------------

class AlgaAdapter(BasePlatformAdapter):

    def __init__(self, config: PlatformConfig):
        platform = Platform("alga")
        super().__init__(config, platform)

        extra = dict(config.extra) if config.extra else {}
        http_base = _resolve_server_url(extra)
        if not http_base:
            http_base = "http://localhost:8080"
            logger.warning("Alga: no server URL configured, falling back to %s", http_base)
        self._http_base = http_base

        self._token: str = config.token or os.getenv("ALGA_AGENT_TOKEN", "")

        self._client = None
        self._sse_task: Optional[asyncio.Task] = None
        self._heartbeat_task: Optional[asyncio.Task] = None
        self._closing = False

        self._dedup = MessageDeduplicator(max_size=2000, ttl_seconds=300)

        self._heartbeat_failures = 0

        self._stopped_chats: set[str] = set()

        if not os.getenv("ALGA_HOME_CHANNEL"):
            os.environ["ALGA_HOME_CHANNEL"] = "alga"

        if not os.getenv("ALGA_ALLOW_ALL_USERS"):
            os.environ["ALGA_ALLOW_ALL_USERS"] = "true"

    def _mark_chat_stopped(self, chat_id: str) -> None:
        normalized = _normalize_chat_id(chat_id) if chat_id != "alga_dm" else chat_id
        self._stopped_chats.add(normalized)

    def _clear_chat_stopped(self, chat_id: str) -> None:
        normalized = _normalize_chat_id(chat_id) if chat_id != "alga_dm" else chat_id
        self._stopped_chats.discard(normalized)

    def _is_chat_stopped(self, chat_id: str) -> bool:
        normalized = _normalize_chat_id(chat_id) if chat_id != "alga_dm" else chat_id
        return normalized in self._stopped_chats

    def _headers(self) -> dict[str, str]:
        return {
            "Authorization": f"Bearer {self._token}",
            "Content-Type": "application/json",
        }

    async def connect(self) -> bool:
        import httpx

        if not self._http_base:
            self._set_fatal_error("config_missing", "Alga server URL not configured", retryable=False)
            return False

        if not self._token:
            self._set_fatal_error("config_missing", "ALGA_AGENT_TOKEN not configured", retryable=False)
            return False

        self._closing = False
        self._client = httpx.AsyncClient(
            base_url=self._http_base,
            headers=self._headers(),
            timeout=httpx.Timeout(30.0, connect=10.0),
        )
        self._sse_task = asyncio.create_task(self._sse_loop())
        self._heartbeat_task = asyncio.create_task(self._heartbeat_loop())
        self._mark_connected()
        logger.info("Alga: connecting to %s", self._http_base)
        return True

    async def disconnect(self) -> None:
        self._closing = True

        if self._heartbeat_task and not self._heartbeat_task.done():
            self._heartbeat_task.cancel()
            try:
                await self._heartbeat_task
            except (asyncio.CancelledError, Exception):
                pass
            self._heartbeat_task = None

        if self._sse_task and not self._sse_task.done():
            self._sse_task.cancel()
            try:
                await self._sse_task
            except (asyncio.CancelledError, Exception):
                pass
            self._sse_task = None

        if self._client:
            await self._client.aclose()
            self._client = None

        self._mark_disconnected()
        logger.info("Alga: disconnected")

    async def send(
        self,
        chat_id: str,
        content: str,
        reply_to: Optional[str] = None,
        metadata: Optional[Dict[str, Any]] = None,
    ) -> SendResult:
        if not content:
            return SendResult(success=True)
        if not self._client or not self.is_connected:
            return SendResult(success=False, error="Not connected to Alga", retryable=True)

        normalized_id = _strip_chat_prefix(chat_id)
        if normalized_id in self._stopped_chats:
            logger.info("Alga: suppressing send to stopped chat %s", normalized_id)
            return SendResult(success=True)
        chunks = self.truncate_message(content, MAX_MESSAGE_LENGTH)
        last_message_id: Optional[str] = None

        for idx, chunk in enumerate(chunks):
            # reply_to_message_id maps to the DB column; the backend persists
            # it on the resulting investigation_thread_messages row.
            payload: Dict[str, Any] = {"chat_id": normalized_id, "kind": "text", "text": chunk}
            if reply_to and idx == 0:
                payload["reply_to_message_id"] = str(reply_to)
            try:
                resp = await self._client.post(_MESSAGES_PATH, json=payload)
                if resp.status_code >= 400:
                    logger.error("Alga: send failed %d: %s", resp.status_code, resp.text[:200])
                    return SendResult(success=False, error=f"HTTP {resp.status_code}", retryable=resp.status_code >= 500)
                body = resp.json()
                last_message_id = body.get("message_id") or body.get("id")
            except Exception as exc:
                logger.error("Alga: send error: %s", exc)
                return SendResult(success=False, error=str(exc), retryable=True)

        return SendResult(success=True, message_id=last_message_id)

    def supports_draft_streaming(
        self,
        chat_type: Optional[str] = None,
        metadata: Optional[Dict[str, Any]] = None,
    ) -> bool:
        return bool(self._client and self.is_connected)

    async def send_draft(
        self,
        chat_id: str,
        draft_id: int,
        content: str,
        metadata: Optional[Dict[str, Any]] = None,
    ) -> SendResult:
        if not self._client or not self.is_connected:
            return SendResult(success=False, error="Not connected to Alga", retryable=True)

        normalized_id = _strip_chat_prefix(chat_id)
        payload = {"chat_id": normalized_id, "draft_id": str(draft_id), "text": content}
        try:
            resp = await self._client.post(_DRAFTS_PATH, json=payload)
            if resp.status_code >= 400:
                logger.debug("Alga: draft failed %d: %s", resp.status_code, resp.text[:200])
                return SendResult(success=False, error=f"HTTP {resp.status_code}", retryable=resp.status_code >= 500)
            return SendResult(success=True)
        except Exception as exc:
            logger.debug("Alga: draft error: %s", exc)
            return SendResult(success=False, error=str(exc), retryable=True)

    async def edit_message(self, chat_id: str, message_id: str, content: str) -> SendResult:
        if not message_id:
            return SendResult(success=False, error="No message_id for edit")
        if not self._client or not self.is_connected:
            return SendResult(success=False, error="Not connected to Alga", retryable=True)

        normalized_id = _strip_chat_prefix(chat_id)
        payload = {"chat_id": normalized_id, "message_id": message_id, "kind": "text", "text": content}
        try:
            resp = await self._client.put(f"{_MESSAGES_PATH}/{message_id}", json=payload)
            if resp.status_code >= 400:
                logger.error("Alga: edit failed %d: %s", resp.status_code, resp.text[:200])
                return SendResult(success=False, error=f"HTTP {resp.status_code}", retryable=resp.status_code >= 500)
            return SendResult(success=True, message_id=message_id)
        except Exception as exc:
            logger.error("Alga: edit error: %s", exc)
            return SendResult(success=False, error=str(exc), retryable=True)

    async def send_typing(self, chat_id: str, metadata: Optional[Dict[str, Any]] = None) -> None:
        if not self._client or not self.is_connected:
            return
        normalized_id = _strip_chat_prefix(chat_id)
        try:
            await self._client.post(_TYPING_PATH, json={"chat_id": normalized_id, "active": True})
        except Exception as exc:
            logger.debug("Alga: send_typing error: %s", exc)

    async def stop_typing(self, chat_id: str) -> None:
        if not self._client or not self.is_connected:
            return
        normalized_id = _strip_chat_prefix(chat_id)
        try:
            await self._client.post(_TYPING_PATH, json={"chat_id": normalized_id, "active": False})
        except Exception as exc:
            logger.debug("Alga: stop_typing error: %s", exc)

    async def get_chat_info(self, chat_id: str) -> Dict[str, Any]:
        normalized_id = _strip_chat_prefix(chat_id)
        return {
            "name": f"Thread {normalized_id}",
            "type": "group",
            "chat_id": chat_id,
        }

    async def _sse_loop(self) -> None:
        import httpx

        delay = _RECONNECT_BASE_DELAY
        sse_url = _SSE_PATH

        while not self._closing:
            try:
                async with self._client.stream("GET", sse_url) as resp:
                    if resp.status_code in (401, 403):
                        logger.error("Alga: auth failed (%d) — stopping reconnect", resp.status_code)
                        self._set_fatal_error("auth_failed", f"Auth failed (HTTP {resp.status_code})", retryable=False)
                        await self._notify_fatal_error()
                        return

                    if resp.status_code >= 400:
                        raise httpx.HTTPStatusError(f"SSE HTTP {resp.status_code}", request=resp.request, response=resp)

                    self._mark_connected()
                    self._heartbeat_failures = 0
                    logger.info("Alga: SSE connected to %s", self._http_base)
                    delay = _RECONNECT_BASE_DELAY

                    event_type = ""
                    event_data_lines: List[str] = []

                    async for line in resp.aiter_lines():
                        if self._closing:
                            return
                        if line.startswith("event:"):
                            event_type = line[len("event:"):].strip()
                        elif line.startswith("data:"):
                            event_data_lines.append(line[len("data:"):].strip())
                        elif line == "":
                            if event_data_lines:
                                raw_data = "\n".join(event_data_lines)
                                await self._handle_sse_event(event_type, raw_data)
                            event_type = ""
                            event_data_lines = []

            except asyncio.CancelledError:
                return
            except Exception as exc:
                if self._closing:
                    return
                err_str = str(exc).lower()
                if "401" in err_str or "403" in err_str or "unauthorized" in err_str:
                    logger.error("Alga: auth failed — stopping reconnect: %s", exc)
                    self._set_fatal_error("auth_failed", f"Auth failed: {exc}", retryable=False)
                    await self._notify_fatal_error()
                    return
                logger.warning("Alga: SSE error: %s — reconnecting in %.0fs", exc, delay)

            if self._closing:
                return
            if self.has_fatal_error:
                return

            jitter = delay * _RECONNECT_JITTER * random.random()
            await asyncio.sleep(delay + jitter)
            delay = min(delay * 2, _RECONNECT_MAX_DELAY)

    async def _handle_sse_event(self, event_type: str, raw_data: str) -> None:
        if not event_type:
            return

        try:
            data = json.loads(raw_data)
        except (json.JSONDecodeError, TypeError):
            return

        if event_type == "message":
            trigger = data.get("trigger", "mention")
            text = data.get("text", "")
            chat_id = data.get("chat_id", "")
            is_stop = isinstance(text, str) and text.strip() == "/stop"

            if is_stop:
                self._mark_chat_stopped(chat_id)
                await self._handle_incoming(data)
            elif trigger == "observe":
                await self._observe_message(data)
            else:
                self._clear_chat_stopped(chat_id)
                await self._handle_incoming(data)
        elif event_type in _SSE_EVENTS_FOR_AGENT:
            logger.debug("Alga: SSE event %s received", event_type)
        else:
            logger.debug("Alga: SSE event %s ignored", event_type)

    async def _heartbeat_loop(self) -> None:
        while not self._closing:
            try:
                await asyncio.sleep(_HEARTBEAT_INTERVAL)
                if self._closing or not self._client:
                    return
                resp = await self._client.post(_HEARTBEAT_PATH)
                if resp.status_code >= 400:
                    self._heartbeat_failures += 1
                    if self._heartbeat_failures >= _HEARTBEAT_FAIL_THRESHOLD:
                        logger.warning(
                            "Alga: heartbeat failed %d consecutive times (last HTTP %d) — server-side presence may have expired",
                            self._heartbeat_failures, resp.status_code,
                        )
                    else:
                        logger.debug("Alga: heartbeat failed HTTP %d (%d/%d)", resp.status_code, self._heartbeat_failures, _HEARTBEAT_FAIL_THRESHOLD)
                else:
                    if self._heartbeat_failures > 0:
                        logger.info("Alga: heartbeat recovered after %d failures", self._heartbeat_failures)
                    self._heartbeat_failures = 0
            except asyncio.CancelledError:
                return
            except Exception as exc:
                self._heartbeat_failures += 1
                if self._heartbeat_failures >= _HEARTBEAT_FAIL_THRESHOLD:
                    logger.warning("Alga: heartbeat error (%d consecutive): %s", self._heartbeat_failures, exc)
                else:
                    logger.debug("Alga: heartbeat error (%d/%d): %s", self._heartbeat_failures, _HEARTBEAT_FAIL_THRESHOLD, exc)

    async def _handle_incoming(self, data: dict) -> None:
        text = data.get("text", "")
        chat_id = data.get("chat_id", "")

        if not text or not chat_id:
            return

        is_dm = chat_id == "alga_dm"
        if not is_dm:
            chat_id = _normalize_chat_id(chat_id)

        msg_id = (
            data.get("message_id")
            or data.get("id")
        )
        if not msg_id:
            return
        if self._dedup.is_duplicate(msg_id):
            return

        if text.startswith("\U0001f512"):
            return

        sender_id = data.get("sender_id", "")
        sender_name = data.get("sender_name", "User")
        reply_to_id, reply_to_text = _extract_reply_to_context(data)

        alert_scope_prefix = _alert_investigation_context_prefix(chat_id)
        if alert_scope_prefix:
            text = alert_scope_prefix + "\n\n" + text

        incident_role = data.get("incident_role", "")
        if incident_role:
            inc_id = data.get("incident_id") or data.get("incident_number") or ""
            context_prefix = _incident_role_context_prefix(
                incident_role,
                str(inc_id),
                data.get("incident_status", ""),
                chat_id=chat_id,
            )
            if context_prefix:
                text = context_prefix + "\n\n" + text

        chat_name = data.get("chat_name") or (
            "Direct Message" if is_dm else f"Thread {chat_id}"
        )

        source = self.build_source(
            chat_id=chat_id,
            chat_name=chat_name,
            chat_type="direct" if is_dm else "group",
            user_id=sender_id or None,
            user_name=sender_name,
            thread_id=chat_id,
        )

        event = MessageEvent(
            text=text,
            message_type=MessageType.TEXT,
            source=source,
            raw_message=data,
            message_id=msg_id,
            reply_to_message_id=reply_to_id,
            reply_to_text=reply_to_text,
        )

        await self.handle_message(event)

    async def _observe_message(self, data: dict) -> None:
        text = data.get("text", "")
        chat_id = data.get("chat_id", "")
        if not text or not chat_id:
            return

        msg_id = data.get("message_id") or data.get("id")
        if not msg_id:
            return
        if self._dedup.is_duplicate(msg_id):
            return

        if text.startswith("\U0001f512"):
            return

        sender_name = data.get("sender_name", "User")
        sender_id = data.get("sender_id", "")
        reply_to_id, reply_to_text = _extract_reply_to_context(data)

        is_dm = chat_id == "alga_dm"
        if not is_dm:
            chat_id = _normalize_chat_id(chat_id)

        store = getattr(self, "_session_store", None)
        if store is not None:
            try:
                source = self.build_source(
                    chat_id=chat_id,
                    chat_name=data.get("chat_name") or ("Direct Message" if is_dm else f"Thread {chat_id}"),
                    chat_type="direct" if is_dm else "group",
                    user_id=sender_id or None,
                    user_name=sender_name,
                    thread_id=chat_id,
                )
                session_entry = store.get_or_create_session(source)
                observed_content = f"[{sender_name}|{sender_id}]\n{text}"
                if reply_to_text:
                    observed_content = f'[Replying to: "{reply_to_text[:500]}"]\n\n{observed_content}'
                entry = {
                    "role": "user",
                    "content": observed_content,
                    "timestamp": datetime.now(tz=timezone.utc).isoformat(),
                    "observed": True,
                }
                store.append_to_transcript(session_entry.session_id, entry)
                logger.debug("Alga: observed message in %s from %s", chat_id, sender_name)
            except Exception as exc:
                logger.debug("Alga: observe failed, dropping: %s", exc)
        else:
            logger.debug("Alga: observe skipped, no session store available")


# ---------------------------------------------------------------------------
# Tool definitions
# ---------------------------------------------------------------------------

_ALGA_TOOLS = [
    {
        "name": "alga_resolve_alert",
        "schema": {
            "name": "alga_resolve_alert",
            "description": "Resolve an alert with optional root cause and resolution. Investigation completes when all alerts are resolved.",
            "parameters": {
                "type": "object",
                "properties": {
                    "chat_id": {"type": "string", "description": "Owner-scoped chat ID (e.g. alert_42, incident_coord_12, incident_inv_12)."},
                    "fingerprint": {"type": "string", "description": "Alert fingerprint (defaults to primary)."},
                    "root_cause": {"type": "string", "description": "Root cause."},
                    "resolution": {"type": "string", "description": "Remediation steps."},
                },
                "required": ["chat_id"],
            },
        },
    },
    {
        "name": "alga_reopen_alert",
        "schema": {
            "name": "alga_reopen_alert",
            "description": "Reopen a resolved alert and resume investigation.",
            "parameters": {
                "type": "object",
                "properties": {
                    "chat_id": {"type": "string", "description": "Owner-scoped chat ID (e.g. alert_42, incident_coord_12, incident_inv_12)."},
                    "fingerprint": {"type": "string", "description": "Alert fingerprint."},
                },
                "required": ["chat_id"],
            },
        },
    },
    {
        "name": "alga_promote_to_incident",
        "schema": {
            "name": "alga_promote_to_incident",
            "description": "Promote an alert to an incident from alert investigation. Borrow the investigation summary as the incident description.",
            "parameters": {
                "type": "object",
                "properties": {
                    "chat_id": {"type": "string", "description": "Investigation chat ID (e.g. alert_42)."},
                    "title": {"type": "string", "description": "Optional custom title for the created incident."},
                    "severity": {"type": "string", "description": "Optional severity (critical, high, warning, info; defaults to warning)."},
                    "priority": {"type": "string", "description": "Optional priority (P1, P2, P3, P4, P5; computed automatically if omitted)."},
                },
                "required": ["chat_id"],
            },
        },
    },
    {
        "name": "alga_set_outcome",
        "schema": {
            "name": "alga_set_outcome",
            "description": "Responder or assigned investigator only (requires investigate capability). The incident commander MUST NOT use this tool — it will be rejected. Records root cause and/or resolution without resolving alerts/incidents.",
            "parameters": {
                "type": "object",
                "properties": {
                    "chat_id": {"type": "string", "description": "Owner-scoped chat ID (e.g. alert_42, incident_coord_12, incident_inv_12)."},
                    "root_cause": {"type": "string", "description": "Root cause analysis."},
                    "resolution": {"type": "string", "description": "Remediation steps."},
                },
                "required": ["chat_id"],
            },
        },
    },
    {
        "name": "alga_cancel_investigation",
        "schema": {
            "name": "alga_cancel_investigation",
            "description": "Responder or assigned investigator only: cancel the investigation with a reason.",
            "parameters": {
                "type": "object",
                "properties": {
                    "chat_id": {"type": "string", "description": "Owner-scoped chat ID (e.g. alert_42, incident_coord_12, incident_inv_12)."},
                    "reason": {"type": "string", "description": "Cancellation reason."},
                },
                "required": ["chat_id", "reason"],
            },
        },
    },
    {
        "name": "alga_pause_investigation",
        "schema": {
            "name": "alga_pause_investigation",
            "description": "Responder or assigned investigator only: pause the investigation with a reason.",
            "parameters": {
                "type": "object",
                "properties": {
                    "chat_id": {"type": "string", "description": "Owner-scoped chat ID (e.g. alert_42, incident_coord_12, incident_inv_12)."},
                    "reason": {"type": "string", "description": "Pause reason."},
                },
                "required": ["chat_id", "reason"],
            },
        },
    },
    {
        "name": "alga_search_knowledge",
        "schema": {
            "name": "alga_search_knowledge",
            "description": "Search knowledge notes (runbooks, known issues, service docs). Returns short previews with each note's id; call alga_get_knowledge with an id to read the full body.",
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {"type": "string", "description": "Search text."},
                    "kind": {"type": "string", "enum": ["runbook", "known_issue", "service_owner", "fact"]},
                    "tag": {"type": "string", "description": "Filter by tag."},
                    "limit": {"type": "number", "description": "Max results (default 10)."},
                },
            },
        },
    },
    {
        "name": "alga_get_knowledge",
        "schema": {
            "name": "alga_get_knowledge",
            "description": "Fetch the full body of a single knowledge note by its id. Use this after alga_search_knowledge when a note looks relevant and you need the complete runbook or known-issue content (search only returns a 200-char preview).",
            "parameters": {
                "type": "object",
                "properties": {
                    "id": {"type": "string", "description": "Knowledge note id (UUID) from alga_search_knowledge."},
                },
                "required": ["id"],
            },
        },
    },
    {
        "name": "alga_create_knowledge",
        "schema": {
            "name": "alga_create_knowledge",
            "description": "Create a knowledge note (runbook, known issue, service doc, or fact) from analysis findings.",
            "parameters": {
                "type": "object",
                "properties": {
                    "kind": {"type": "string", "enum": ["runbook", "known_issue", "service_owner", "fact"]},
                    "title": {"type": "string", "description": "Note title."},
                    "body_markdown": {"type": "string", "description": "Note content in markdown."},
                    "tags": {"type": "array", "items": {"type": "string"}},
                    "source_investigation_id": {"type": "string", "description": "Source Alga investigation ID that produced this note."},
                    "confidence": {"type": "number", "description": "Confidence score 0-1."},
                },
                "required": ["title", "body_markdown", "source_investigation_id", "confidence"],
            },
        },
    },
    {
        "name": "alga_list_alerts",
        "schema": {
            "name": "alga_list_alerts",
            "description": "List alerts with optional filters. Requires investigate capability (responder/investigator role only); a pure incident commander token will be denied (403) and should instead ask the responder for alert details.",
            "parameters": {
                "type": "object",
                "properties": {
                    "status": {"type": "string", "description": "Filter by status (firing, resolved, acknowledged)."},
                    "severity": {"type": "string", "description": "Filter by severity."},
                    "search": {"type": "string", "description": "Search term."},
                    "limit": {"type": "number", "description": "Max results (default 20)."},
                },
            },
        },
    },
    {
        "name": "alga_triage_feedback",
        "schema": {
            "name": "alga_triage_feedback",
            "description": "Provide feedback on a triage decision.",
            "parameters": {
                "type": "object",
                "properties": {
                    "triage_result_id": {"type": "string", "description": "Triage result ID."},
                    "agreed": {"type": "boolean", "description": "Whether you agree with the triage decision."},
                    "correct_decision": {"type": "string", "description": "Correct decision if disagreed."},
                    "correct_severity": {"type": "string", "description": "Correct severity if disagreed."},
                    "note": {"type": "string", "description": "Additional notes."},
                },
                "required": ["triage_result_id", "agreed"],
            },
        },
    },
    {
        "name": "alga_set_incident_priority",
        "schema": {
            "name": "alga_set_incident_priority",
            "description": "Incident commander only: set incident priority (P1-P5).",
            "parameters": {
                "type": "object",
                "properties": {
                    "incident_number": {"type": "string", "description": "Incident ID."},
                    "priority": {"type": "string", "description": "Priority: P1, P2, P3, P4, or P5."},
                },
                "required": ["incident_number", "priority"],
            },
        },
    },
    {
        "name": "alga_set_incident_severity",
        "schema": {
            "name": "alga_set_incident_severity",
            "description": "Responder or assigned investigator only: change incident severity: critical, high, warning, or info.",
            "parameters": {
                "type": "object",
                "properties": {
                    "incident_number": {"type": "string", "description": "Incident ID (e.g. 42)."},
                    "severity": {"type": "string", "enum": ["critical", "high", "warning", "info"]},
                },
                "required": ["incident_number", "severity"],
            },
        },
    },
    {
        "name": "alga_trigger_escalation",
        "schema": {
            "name": "alga_trigger_escalation",
            "description": "Incident commander only: trigger escalation for an incident.",
            "parameters": {
                "type": "object",
                "properties": {
                    "incident_number": {"type": "string", "description": "Incident ID."},
                },
                "required": ["incident_number"],
            },
        },
    },
    {
        "name": "alga_mitigate_incident",
        "schema": {
            "name": "alga_mitigate_incident",
            "description": "Incident commander only: mark an incident as mitigated after responder investigation is complete.",
            "parameters": {
                "type": "object",
                "properties": {
                    "incident_number": {"type": "string", "description": "Incident ID."},
                    "reason": {"type": "string", "description": "Mitigation reason."},
                },
                "required": ["incident_number"],
            },
        },
    },
    {
        "name": "alga_resolve_incident",
        "schema": {
            "name": "alga_resolve_incident",
            "description": "Incident commander only: resolve an incident. Resolution REQUIRES five structured fields (summary, impact_assessment, actions_taken, root_cause, resolution) — supply them inline here and Alga records them, then resolves. If any are missing AND not already on file, resolution fails with 422 listing what is required. You can also stage them first with alga_set_incident_resolution_docs. root_cause is the underlying cause of the incident and resolution is the concrete remediation/recovery applied; both are mandatory document sections. Resolution is ALSO blocked until a resolved status update has been published (via alga_publish_status_update with status_level=resolved).",
            "parameters": {
                "type": "object",
                "properties": {
                    "incident_number": {"type": "string", "description": "Incident ID."},
                    "reason": {"type": "string", "description": "Resolution reason."},
                    "summary": {"type": "string", "description": "Detailed executive resolution summary including the cause, why it started, what it did, and status until recovery (required if not already set)."},
                    "impact_assessment": {"type": "string", "description": "Customer/business impact assessment (required if not already set)."},
                    "actions_taken": {"type": "string", "description": "Concrete actions taken against the environment to mitigate and resolve (required if not already set). MUST be valid markdown with each action on its own line as a `- ` bullet item (or `1.` numbered list). Describe what was DONE TO the system — commands run, services restarted, deployments rolled back, configs changed, hosts cordoned, traffic shifted, etc. Do NOT describe which agent or role did what; that is implied by role assignment and is not useful. Omit @mentions of agents. If no environment action was taken (e.g. alert was already resolved, no impact), state that explicitly in one line."},
                    "root_cause": {"type": "string", "description": "Underlying root cause of the incident (required if not already set). Describe WHY the incident happened — the technical or process failure that triggered it. Distinguish from `resolution` (what fixed it) and `summary` (executive narrative)."},
                    "resolution": {"type": "string", "description": "Concrete remediation/recovery applied to resolve the incident (required if not already set). Describe the fix, rollback, or configuration change that restored service. Distinguish from `actions_taken` (environment commands) and `root_cause` (why it happened)."},
                },
                "required": ["incident_number"],
            },
        },
    },
    {
        "name": "alga_set_incident_resolution_docs",
        "schema": {
            "name": "alga_set_incident_resolution_docs",
            "description": "Incident commander only (requires command capability): record the structured resolution documents — summary, impact_assessment, actions_taken, root_cause, and resolution — without resolving. Use this to stage resolution artifacts while the incident is still active/mitigated, then call alga_resolve_incident. At least one field is required. These fields are mandatory before an incident can be resolved; root_cause and resolution are incident document sections shown on the incident page.",
            "parameters": {
                "type": "object",
                "properties": {
                    "incident_number": {"type": "string", "description": "Incident ID (e.g. 42)."},
                    "summary": {"type": "string", "description": "Detailed executive resolution summary including the cause, why it started, what it did, and status until recovery."},
                    "impact_assessment": {"type": "string", "description": "Customer/business impact assessment."},
                    "actions_taken": {"type": "string", "description": "Concrete actions taken against the environment to mitigate and resolve. MUST be valid markdown with each action on its own line as a `- ` bullet item (or `1.` numbered list). Describe what was DONE TO the system — commands run, services restarted, deployments rolled back, configs changed, hosts cordoned, traffic shifted, etc. Do NOT describe which agent or role did what; that is implied by role assignment and is not useful. Omit @mentions of agents. If no environment action was taken (e.g. alert was already resolved, no impact), state that explicitly in one line."},
                    "root_cause": {"type": "string", "description": "Underlying root cause of the incident. Describe WHY the incident happened — the technical or process failure that triggered it. Distinguish from `resolution` (what fixed it) and `summary` (executive narrative)."},
                    "resolution": {"type": "string", "description": "Concrete remediation/recovery applied to resolve the incident. Describe the fix, rollback, or configuration change that restored service. Distinguish from `actions_taken` (environment commands) and `root_cause` (why it happened)."},
                },
                "required": ["incident_number"],
            },
        },
    },
    {
        "name": "alga_begin_triage",
        "schema": {
            "name": "alga_begin_triage",
            "description": "Incident commander only: transition a newly detected incident into 'triaging' status.",
            "parameters": {
                "type": "object",
                "properties": {
                    "incident_number": {"type": "string", "description": "Incident ID (e.g. 42)."},
                },
                "required": ["incident_number"],
            },
        },
    },
    {
        "name": "alga_promote_incident",
        "schema": {
            "name": "alga_promote_incident",
            "description": "Incident commander only: promote a triaged incident to 'active' status.",
            "parameters": {
                "type": "object",
                "properties": {
                    "incident_number": {"type": "string", "description": "Incident ID (e.g. 42)."},
                },
                "required": ["incident_number"],
            },
        },
    },
    {
        "name": "alga_add_incident_timeline",
        "schema": {
            "name": "alga_add_incident_timeline",
            "description": "Add a timeline entry to an incident. Requires investigate capability (responder/investigator role only); a pure incident commander token will be denied (403).",
            "parameters": {
                "type": "object",
                "properties": {
                    "incident_number": {"type": "string", "description": "Incident ID (e.g. 42)."},
                    "message": {"type": "string", "description": "Timeline message content."},
                    "event_type": {"type": "string", "description": "Optional event type classification (defaults to agent_note)."},
                },
                "required": ["incident_number", "message"],
            },
        },
    },
    {
        "name": "alga_assign_incident_role",
        "schema": {
            "name": "alga_assign_incident_role",
            "description": "Incident commander only: assign an ICS role to a user or agent for an incident.",
            "parameters": {
                "type": "object",
                "properties": {
                    "incident_number": {"type": "string", "description": "Incident ID (e.g. 42)."},
                    "role_type": {"type": "string", "enum": ["incident_commander", "communications_lead", "responder"], "description": "The ICS role type."},
                    "user_id": {"type": "string", "description": "User UUID (provide user_id or agent_token_id)."},
                    "agent_token_id": {"type": "string", "description": "Agent token UUID (provide user_id or agent_token_id)."},
                    "scope_description": {"type": "string", "description": "Optional scope description for role responsibilities."},
                },
                "required": ["incident_number", "role_type"],
            },
        },
    },
    {
        "name": "alga_get_incident_context",
        "schema": {
            "name": "alga_get_incident_context",
            "description": "Get incident context including status, severity, timeline, roles, and linked alerts/investigations. Available to assigned incident roles and investigate-capable responders; use this for internal coordination context, not public status wording.",
            "parameters": {
                "type": "object",
                "properties": {
                    "incident_number": {"type": "string", "description": "Incident ID (e.g. 42)."},
                },
                "required": ["incident_number"],
            },
        },
    },
    {
        "name": "alga_get_incident_timeline",
        "schema": {
            "name": "alga_get_incident_timeline",
            "description": "Get the timeline of events for an incident.",
            "parameters": {
                "type": "object",
                "properties": {
                    "incident_number": {"type": "string", "description": "Incident ID (e.g. 42)."},
                },
                "required": ["incident_number"],
            },
        },
    },
    {
        "name": "alga_post_handoff",
        "schema": {
            "name": "alga_post_handoff",
            "description": "Commander-facing coordination tool for the FINAL handoff after all recovery and verification work is complete. Set audience='commander' or audience='command' when commander review, approval, or a decision is needed. WARNING: calling this tool ACTIVATES other agents (commander, communicator) by forwarding the message to them — every call wakes up teammate agents and can interrupt their current work, causing ping-pong loops. Do NOT call this tool during investigation, identification, mitigation, or verification phases. For status milestones during active work (identified, monitoring, resolved), use alga_publish_status_update instead — it does NOT activate other agents and is the only path that creates a Status Updates card entry. Do NOT use this tool to publish milestone updates, post investigation findings, send progress notes, or share interim summaries; those belong in alga_publish_status_update (for status) or the alert investigation thread (for technical findings). Reserve this tool for the single structured commander handoff that happens AFTER recovery is verified AND a status_level='monitoring' update has already been published via alga_publish_status_update.",
            "parameters": {
                "type": "object",
                "properties": {
                    "chat_id": {"type": "string", "description": "Incident coordination chat ID, for example incident_42."},
                    "message": {"type": "string", "description": "Short commander-facing coordination update, decision request, summary, or handoff."},
                    "audience": {"type": "string", "enum": ["none", "commander", "communicator", "command"], "description": "Role intent for backend mention resolution."},
                    "urgency": {"type": "string", "enum": ["info", "needs_attention", "decision_needed"], "description": "Update urgency metadata."},
                },
                "required": ["chat_id", "message"],
            },
        },
    },
    {
        "name": "alga_publish_status_update",
        "schema": {
            "name": "alga_publish_status_update",
            "description": "Publish a public-facing status update for an incident (can be called directly by the incident commander, responder, or communicator). This is the PREFERRED tool for all status communication during investigation, identification, mitigation, and monitoring — it does NOT activate or notify other agents, so it cannot cause ping-pong loops. Pick the status_level that reflects current state: investigating, identified, mitigated, monitoring, resolved. Do not mention internal alert numbers, investigation IDs, UUIDs, agent names, or placeholder/test labels; describe user-visible impact and current service state. At least one public status update must be published before the incident commander can resolve the incident. Responders MUST NOT publish status_level='resolved' — that is commander-only.",
            "parameters": {
                "type": "object",
                "properties": {
                    "incident_number": {"type": "string", "description": "Incident ID (e.g. 42)."},
                    "message": {"type": "string", "description": "Public-facing status update text."},
                    "status_level": {"type": "string", "enum": ["investigating", "identified", "mitigated", "monitoring", "resolved"], "description": "Public status level reflected by this update. Responders must use 'identified', 'mitigated', or 'monitoring' only — never 'resolved' (commander-only) or 'investigating' (system-only)."},
                    "source_coordination_message_id": {"type": "string", "description": "Optional coordination message id this update responds to."},
                },
                "required": ["incident_number", "message"],
            },
        },
    },
    {
        "name": "alga_list_services",
        "schema": {
            "name": "alga_list_services",
            "description": "List all registered services with their current status.",
            "parameters": {
                "type": "object",
                "properties": {},
            },
        },
    },
    {
        "name": "alga_who_is_on_call",
        "schema": {
            "name": "alga_who_is_on_call",
            "description": "Get the current on-call person for each schedule.",
            "parameters": {
                "type": "object",
                "properties": {},
            },
        },
    },
]


# ---------------------------------------------------------------------------
# Tool handlers
# ---------------------------------------------------------------------------

async def _alga_resolve_alert(args: dict, **kw) -> str:
    chat_id = args.get("chat_id", "").strip()
    if not chat_id:
        return json.dumps({"error": "chat_id is required"})
    cmd: Dict[str, Any] = {"op": "resolve_alert"}
    for key in ("fingerprint", "root_cause", "resolution"):
        val = args.get(key, "").strip()
        if val:
            cmd[key] = val
    return await _inv_tool(chat_id, cmd)


async def _alga_reopen_alert(args: dict, **kw) -> str:
    chat_id = args.get("chat_id", "").strip()
    if not chat_id:
        return json.dumps({"error": "chat_id is required"})
    cmd: Dict[str, Any] = {"op": "reopen_alert"}
    fp = args.get("fingerprint", "").strip()
    if fp:
        cmd["fingerprint"] = fp
    return await _inv_tool(chat_id, cmd)


async def _alga_promote_to_incident(args: dict, **kw) -> str:
    chat_id = args.get("chat_id", "").strip()
    if not chat_id:
        return json.dumps({"error": "chat_id is required"})
    cmd: Dict[str, Any] = {"op": "promote_to_incident"}
    if "title" in args:
        cmd["title"] = args["title"]
    if "severity" in args:
        cmd["severity"] = args["severity"]
    if "priority" in args:
        cmd["priority"] = args["priority"]
    return await _inv_tool(chat_id, cmd)


async def _alga_set_incident_severity(args: dict, **kw) -> str:
    incident_id = args.get("incident_number", "").strip()
    severity = args.get("severity", "").strip()
    if not incident_id or not severity:
        return json.dumps({"error": "incident_number and severity are required"})
    if severity not in ("critical", "high", "warning", "info"):
        return json.dumps({"error": "severity must be one of: critical, high, warning, info"})
    cmd = {"op": "set_incident_severity", "incident_number": int(incident_id), "severity": severity}
    return await _inv_tool(f"incident_coord_{incident_id}", cmd)


async def _alga_set_outcome(args: dict, **kw) -> str:
    chat_id = args.get("chat_id", "").strip()
    if not chat_id:
        return json.dumps({"error": "chat_id is required"})
    cmd: Dict[str, Any] = {"op": "set_outcome"}
    for key in ("root_cause", "resolution"):
        val = args.get(key, "").strip()
        if val:
            cmd[key] = val
    return await _inv_tool(chat_id, cmd)


async def _alga_cancel_investigation(args: dict, **kw) -> str:
    chat_id = args.get("chat_id", "").strip()
    if not chat_id:
        return json.dumps({"error": "chat_id is required"})
    return await _inv_tool(chat_id, {"op": "cancel_investigation", "reason": args.get("reason", "").strip()})


async def _alga_pause_investigation(args: dict, **kw) -> str:
    chat_id = args.get("chat_id", "").strip()
    if not chat_id:
        return json.dumps({"error": "chat_id is required"})
    return await _inv_tool(chat_id, {"op": "pause_investigation", "reason": args.get("reason", "").strip()})


async def _alga_search_knowledge(args: dict, **kw) -> str:
    params: Dict[str, Any] = {}
    for key, arg_key in [("q", "query"), ("kind", "kind"), ("tag", "tag")]:
        val = args.get(arg_key, "").strip()
        if val:
            params[key] = val
    limit = args.get("limit")
    if limit:
        params["limit"] = int(limit)
    result = await _agent_get(_KNOWLEDGE_PATH, params)
    if isinstance(result, dict) and result.get("ok") is False:
        return json.dumps({"error": result.get("error", "unknown error")})
    notes = result.get("items", []) if isinstance(result, dict) else (result if isinstance(result, list) else [])
    if not notes:
        return json.dumps({"success": True, "count": 0, "notes": []})
    lines = []
    for i, n in enumerate(notes):
        note_id = n.get("id", "")
        title = n.get("title", "untitled")
        kind = n.get("kind", "note")
        full_body = n.get("body_markdown") or ""
        preview = full_body[:200]
        truncated = len(full_body) > 200
        suffix = " …[truncated]" if truncated else ""
        lines.append(f"{i + 1}. [{kind}] {title}\nid: {note_id}\n{preview}{suffix}")
    hint = (
        "Previews are truncated to 200 chars. For the full body of a note, "
        "call alga_get_knowledge with its id."
    )
    return json.dumps(
        {"success": True, "count": len(notes), "notes": "\n\n".join(lines), "hint": hint},
        ensure_ascii=False,
    )


async def _alga_get_knowledge(args: dict, **kw) -> str:
    note_id = str(args.get("id", "")).strip()
    if not note_id:
        return json.dumps({"error": "id is required"})
    result = await _agent_get(f"{_KNOWLEDGE_PATH}/{note_id}")
    if isinstance(result, dict) and result.get("ok") is False:
        return json.dumps({"error": result.get("error", "unknown error")})
    if not isinstance(result, dict):
        return json.dumps({"error": "unexpected response shape from knowledge API"})
    return json.dumps({"success": True, "note": result}, ensure_ascii=False)


async def _alga_create_knowledge(args: dict, **kw) -> str:
    title = args.get("title", "").strip()
    body_markdown = args.get("body_markdown", "").strip()
    if not title or not body_markdown:
        return json.dumps({"error": "title and body_markdown are required"})
    source_investigation_id = args.get("source_investigation_id", "").strip()
    if not source_investigation_id:
        return json.dumps({"error": "source_investigation_id is required"})
    confidence = args.get("confidence")
    if confidence is None:
        return json.dumps({"error": "confidence is required"})
    note: Dict[str, Any] = {
        "kind": args.get("kind", "fact").strip(),
        "title": title,
        "body_markdown": body_markdown,
        "source_investigation_id": source_investigation_id,
        "confidence": float(confidence),
    }
    tags = args.get("tags")
    if isinstance(tags, list) and tags:
        note["tags"] = tags
    result = await _agent_post(_KNOWLEDGE_PATH, note)
    if isinstance(result, dict) and result.get("ok") is False:
        return json.dumps({"error": result.get("error", "unknown error")})
    return json.dumps({"success": True, "title": title})


async def _alga_list_alerts(args: dict, **kw) -> str:
    params: Dict[str, Any] = {}
    for key in ("status", "severity", "search"):
        val = args.get(key, "").strip()
        if val:
            params[key] = val
    limit = args.get("limit")
    if limit:
        params["limit"] = int(limit)
    result = await _agent_get(_ALERTS_PATH, params)
    if isinstance(result, dict) and result.get("ok") is False:
        return json.dumps({"error": result.get("error", "unknown error")})
    alerts = result if isinstance(result, list) else result.get("alerts", [])
    if not alerts:
        return json.dumps({"success": True, "count": 0, "alerts": []})
    lines = []
    for i, a in enumerate(alerts):
        labels = a.get("labels", {})
        name = labels.get("alertname", a.get("fingerprint", "?"))
        status = a.get("status", "unknown")
        lines.append(f"{i + 1}. {name} [{status}]")
    return json.dumps({"success": True, "count": len(alerts), "alerts": "\n".join(lines)}, ensure_ascii=False)


async def _alga_triage_feedback(args: dict, **kw) -> str:
    triage_result_id = args.get("triage_result_id", "").strip()
    if not triage_result_id:
        return json.dumps({"error": "triage_result_id is required"})
    cmd: Dict[str, Any] = {
        "op": "triage_feedback",
        "triage_result_id": triage_result_id,
        "agreed": args.get("agreed", True),
    }
    for key in ("correct_decision", "correct_severity", "note"):
        val = args.get(key, "").strip()
        if val:
            cmd[key] = val
    result = await _agent_post(_MESSAGES_PATH, {
        "chat_id": "",
        "kind": "inv_tool",
        "command": cmd,
    })
    if isinstance(result, dict) and result.get("ok") is False:
        return json.dumps({"error": result.get("error", "unknown error")})
    return json.dumps({"success": True})


async def _alga_set_incident_priority(args: dict, **kw) -> str:
    incident_id = args.get("incident_number", "").strip()
    priority = args.get("priority", "").strip()
    if not incident_id or not priority:
        return json.dumps({"error": "incident_number and priority are required"})
    cmd = {"op": "set_incident_priority", "incident_number": int(incident_id), "priority": priority}
    return await _inv_tool(f"incident_coord_{incident_id}", cmd)


async def _alga_trigger_escalation(args: dict, **kw) -> str:
    incident_id = args.get("incident_number", "").strip()
    if not incident_id:
        return json.dumps({"error": "incident_number is required"})
    cmd = {"op": "trigger_escalation", "incident_number": int(incident_id)}
    return await _inv_tool(f"incident_coord_{incident_id}", cmd)










async def _alga_mitigate_incident(args: dict, **kw) -> str:
    incident_id = args.get("incident_number", "").strip()
    if not incident_id:
        return json.dumps({"error": "incident_number is required"})
    cmd: Dict[str, Any] = {"op": "mitigate_incident", "incident_number": int(incident_id)}
    reason = args.get("reason", "").strip()
    if reason:
        cmd["reason"] = reason
    return await _inv_tool(f"incident_coord_{incident_id}", cmd)


async def _alga_resolve_incident(args: dict, **kw) -> str:
    incident_id = args.get("incident_number", "").strip()
    if not incident_id:
        return json.dumps({"error": "incident_number is required"})
    cmd: Dict[str, Any] = {"op": "resolve_incident", "incident_number": int(incident_id)}
    reason = args.get("reason", "").strip()
    if reason:
        cmd["reason"] = reason
    for key in ("summary", "impact_assessment", "actions_taken"):
        val = args.get(key, "").strip()
        if val:
            cmd[key] = val
    for key in ("root_cause", "resolution"):
        val = args.get(key, "").strip()
        if val:
            cmd[key] = val
    return await _inv_tool(f"incident_coord_{incident_id}", cmd)


async def _alga_set_incident_resolution_docs(args: dict, **kw) -> str:
    incident_id = args.get("incident_number", "").strip()
    if not incident_id:
        return json.dumps({"error": "incident_number is required"})
    cmd: Dict[str, Any] = {"op": "set_incident_resolution_docs", "incident_number": int(incident_id)}
    provided = False
    for key in ("summary", "impact_assessment", "actions_taken"):
        val = args.get(key, "").strip()
        if val:
            cmd[key] = val
            provided = True
    for key in ("root_cause", "resolution"):
        val = args.get(key, "").strip()
        if val:
            cmd[key] = val
            provided = True
    if not provided:
        return json.dumps({"error": "at least one of summary, impact_assessment, actions_taken, root_cause, or resolution is required"})
    return await _inv_tool(f"incident_coord_{incident_id}", cmd)


async def _alga_get_incident_context(args: dict, **kw) -> str:
    incident_id = args.get("incident_number", "").strip()
    if not incident_id:
        return json.dumps({"error": "incident_number is required"})
    result = await _agent_get(f"/api/v1/agent/incidents/{incident_id}")
    return json.dumps(result)


async def _alga_get_incident_timeline(args: dict, **kw) -> str:
    incident_id = args.get("incident_number", "").strip()
    if not incident_id:
        return json.dumps({"error": "incident_number is required"})
    result = await _agent_get(f"/api/v1/agent/incidents/{incident_id}/timeline")
    return json.dumps(result)


async def _alga_list_services(args: dict, **kw) -> str:
    result = await _agent_get("/api/v1/agent/services")
    return json.dumps(result)


async def _alga_who_is_on_call(args: dict, **kw) -> str:
    result = await _agent_get("/api/v1/agent/on-call/current")
    return json.dumps(result)


async def _alga_begin_triage(args: dict, **kw) -> str:
    incident_id = args.get("incident_number", "").strip()
    if not incident_id:
        return json.dumps({"error": "incident_number is required"})
    cmd = {"op": "begin_triage", "incident_number": int(incident_id)}
    return await _inv_tool(f"incident_coord_{incident_id}", cmd)


async def _alga_promote_incident(args: dict, **kw) -> str:
    incident_id = args.get("incident_number", "").strip()
    if not incident_id:
        return json.dumps({"error": "incident_number is required"})
    cmd = {"op": "promote_incident", "incident_number": int(incident_id)}
    return await _inv_tool(f"incident_coord_{incident_id}", cmd)


async def _alga_add_incident_timeline(args: dict, **kw) -> str:
    incident_id = args.get("incident_number", "").strip()
    message = args.get("message", "").strip()
    if not incident_id or not message:
        return json.dumps({"error": "incident_number and message are required"})
    event_type = args.get("event_type", "agent_note").strip()
    payload = {"message": message}
    if event_type:
        payload["event_type"] = event_type
    result = await _agent_post(f"/api/v1/agent/incidents/{incident_id}/timeline", payload)
    if isinstance(result, dict) and result.get("ok") is False:
        return json.dumps({"error": result.get("error", "unknown error")})
    return json.dumps({"success": True, "incident_number": int(incident_id)})


async def _alga_assign_incident_role(args: dict, **kw) -> str:
    incident_id = args.get("incident_number", "").strip()
    role_type = args.get("role_type", "").strip()
    if not incident_id or not role_type:
        return json.dumps({"error": "incident_number and role_type are required"})
    if role_type not in ("incident_commander", "communications_lead", "responder"):
        return json.dumps({"error": "role_type must be one of: incident_commander, communications_lead, responder"})
    user_id = args.get("user_id", "").strip()
    agent_token_id = args.get("agent_token_id", "").strip()
    if not user_id and not agent_token_id:
        return json.dumps({"error": "user_id or agent_token_id is required"})
    if user_id and agent_token_id:
        return json.dumps({"error": "provide either user_id or agent_token_id, not both"})
    cmd = {
        "op": "assign_incident_role",
        "incident_number": int(incident_id),
        "role_type": role_type,
    }
    if user_id:
        cmd["user_id"] = user_id
    if agent_token_id:
        cmd["agent_token_id"] = agent_token_id
    scope_description = args.get("scope_description", "").strip()
    if scope_description:
        cmd["scope_description"] = scope_description
    return await _inv_tool(f"incident_coord_{incident_id}", cmd)


async def _alga_post_handoff(args: dict, **kw) -> str:
    chat_id = args.get("chat_id", "").strip()
    message = args.get("message", "").strip()
    if not chat_id or not message:
        return json.dumps({"error": "chat_id and message are required"})
    normalized_id = _normalize_chat_id(chat_id)
    if not (normalized_id.startswith("incident_coord_") or normalized_id.startswith("incident_inv_")):
        return json.dumps({"error": "alga_post_handoff requires an incident coordination or investigation chat_id starting with 'incident_coord_' or 'incident_inv_'; alert threads only support alert investigation tools"})
    audience = args.get("audience", "none").strip() or "none"
    if audience not in ("none", "commander", "communicator", "command"):
        return json.dumps({"error": "audience must be one of: none, commander, communicator, command"})
    urgency = args.get("urgency", "info").strip() or "info"
    if urgency not in ("info", "needs_attention", "decision_needed"):
        return json.dumps({"error": "urgency must be one of: info, needs_attention, decision_needed"})
    # status_level is intentionally NOT accepted on alga_post_handoff. Status
    # milestones go through alga_publish_status_update (which writes to the
    # Status Updates card); passing status_level here would be rejected by the
    # backend with a 422. Some LLMs that were trained on earlier versions of
    # this tool still emit it; we silently drop it here so the call succeeds
    # instead of producing a confusing error. The schema also no longer
    # declares it (see the alga_post_handoff tool entry above), so new agents
    # that respect the schema will not pass it.
    return await _inv_tool(chat_id, {"op": "post_handoff", "message": message, "audience": audience, "urgency": urgency})


def _validate_status_level(value: str) -> str:
    value = (value or "investigating").strip() or "investigating"
    if value not in ("investigating", "identified", "mitigated", "monitoring", "resolved"):
        raise ValueError("status_level must be one of: investigating, identified, mitigated, monitoring, resolved")
    return value


async def _alga_publish_status_update(args: dict, **kw) -> str:
    incident_id = args.get("incident_number", "").strip()
    if not incident_id:
        return json.dumps({"error": "incident_number is required"})
    message = args.get("message", "").strip()
    if not message:
        return json.dumps({"error": "message is required"})
    try:
        status_level = _validate_status_level(args.get("status_level", "investigating"))
    except ValueError as exc:
        return json.dumps({"error": str(exc)})
    cmd: Dict[str, Any] = {
        "op": "publish_status_update",
        "incident_number": int(incident_id),
        "message": message,
        "status_level": status_level,
    }
    src = args.get("source_coordination_message_id", "").strip()
    if src:
        cmd["source_coordination_message_id"] = src
    if args.get("internal"):
        cmd["internal"] = True
    return await _inv_tool(f"incident_coord_{incident_id}", cmd)


_TOOL_HANDLERS = {
    "alga_resolve_alert": _alga_resolve_alert,
    "alga_reopen_alert": _alga_reopen_alert,
    "alga_promote_to_incident": _alga_promote_to_incident,
    "alga_set_outcome": _alga_set_outcome,
    "alga_cancel_investigation": _alga_cancel_investigation,
    "alga_pause_investigation": _alga_pause_investigation,
    "alga_search_knowledge": _alga_search_knowledge,
    "alga_get_knowledge": _alga_get_knowledge,
    "alga_create_knowledge": _alga_create_knowledge,
    "alga_list_alerts": _alga_list_alerts,
    "alga_triage_feedback": _alga_triage_feedback,
    "alga_set_incident_priority": _alga_set_incident_priority,
    "alga_set_incident_severity": _alga_set_incident_severity,
    "alga_trigger_escalation": _alga_trigger_escalation,
    "alga_mitigate_incident": _alga_mitigate_incident,
    "alga_resolve_incident": _alga_resolve_incident,
    "alga_set_incident_resolution_docs": _alga_set_incident_resolution_docs,
    "alga_begin_triage": _alga_begin_triage,
    "alga_promote_incident": _alga_promote_incident,
    "alga_add_incident_timeline": _alga_add_incident_timeline,
    "alga_assign_incident_role": _alga_assign_incident_role,
    "alga_get_incident_context": _alga_get_incident_context,
    "alga_get_incident_timeline": _alga_get_incident_timeline,
    "alga_list_services": _alga_list_services,
    "alga_who_is_on_call": _alga_who_is_on_call,
    "alga_post_handoff": _alga_post_handoff,
    "alga_publish_status_update": _alga_publish_status_update,
}


# ---------------------------------------------------------------------------
# Plugin entry point
# ---------------------------------------------------------------------------

def interactive_setup():
    """Interactive setup for Alga credentials."""
    try:
        from hermes_cli.console import get_console
        console = get_console()
    except ImportError:
        import sys as _sys
        console = None

    def _print(msg: str = ""):
        if console:
            console.print(msg)
        else:
            _sys.stdout.write(msg + "\n")

    def _input(prompt: str) -> str:
        if console:
            return console.input(prompt)
        return input(prompt)

    _print("\n[bold]Alga Platform Setup[/bold]")
    _print("Configure the Alga investigation gateway for Hermes.\n")

    server_url = _input("Alga server URL (e.g. http://alga:8080): ").strip()
    if server_url:
        hermes_home = os.path.expanduser("~/.hermes")
        env_path = os.path.join(hermes_home, ".env")
        lines = []
        if os.path.exists(env_path):
            with open(env_path) as f:
                lines = f.readlines()
        updated = {"ALGA_SERVER_URL": False, "ALGA_AGENT_TOKEN": False}
        new_lines = []
        for line in lines:
            stripped = line.strip()
            if stripped.startswith("ALGA_SERVER_URL="):
                new_lines.append(f"ALGA_SERVER_URL={server_url}\n")
                updated["ALGA_SERVER_URL"] = True
            elif stripped.startswith("ALGA_AGENT_TOKEN="):
                token = _input("Alga agent token: ").strip()
                new_lines.append(f"ALGA_AGENT_TOKEN={token}\n")
                updated["ALGA_AGENT_TOKEN"] = True
            else:
                new_lines.append(line)
        if not updated["ALGA_SERVER_URL"]:
            new_lines.append(f"ALGA_SERVER_URL={server_url}\n")
        if not updated["ALGA_AGENT_TOKEN"]:
            token = _input("Alga agent token: ").strip()
            new_lines.append(f"ALGA_AGENT_TOKEN={token}\n")
        with open(env_path, "w") as f:
            f.writelines(new_lines)
        os.chmod(env_path, stat.S_IRUSR | stat.S_IWUSR)
        _print(f"\nConfiguration saved to {env_path}")
    _print("Restart the gateway for changes to take effect: hermes gateway restart")


def register(ctx):
    """Plugin entry point — called by the Hermes plugin system."""

    ctx.register_platform(
        name="alga",
        label="Alga",
        adapter_factory=lambda cfg: AlgaAdapter(cfg),
        check_fn=check_requirements,
        validate_config=validate_config,
        is_connected=is_connected,
        required_env=["ALGA_SERVER_URL", "ALGA_AGENT_TOKEN"],
        install_hint="pip install httpx>=0.27",
        setup_fn=interactive_setup,
        allowed_users_env="ALGA_ALLOWED_USERS",
        allow_all_env="ALGA_ALLOW_ALL_USERS",
        max_message_length=MAX_MESSAGE_LENGTH,
        emoji="🧩",
        pii_safe=True,
        allow_update_command=True,
        platform_hint=(
            "Alga alert and incident threads. Alerts are pre-acknowledged. "
            "When done investigating, resolve the owning alert or incident with the appropriate Alga tool. "
            "Use alga_search_knowledge to find runbooks and known issues; when a note looks relevant, call alga_get_knowledge with its id to read the full body. "
            "Chat IDs are owner-scoped: alert_<number> or incident_<id>."
        ),
    )

    for tool_def in _ALGA_TOOLS:
        handler = _TOOL_HANDLERS.get(tool_def["name"])
        if not handler:
            logger.warning("Alga plugin: no handler for tool %s", tool_def["name"])
            continue
        ctx.register_tool(
            name=tool_def["name"],
            toolset=_ALGA_TOOLSET,
            schema=tool_def["schema"],
            handler=handler,
            check_fn=_check_tool_availability,
            requires_env=["ALGA_SERVER_URL", "ALGA_AGENT_TOKEN"],
            is_async=True,
            emoji=tool_def.get("emoji", "🧩"),
        )

    logger.info("Alga plugin: registered platform adapter + %d tools", len(_ALGA_TOOLS))
