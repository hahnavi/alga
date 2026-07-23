from __future__ import annotations

from datetime import datetime
from typing import Any, Optional

from pydantic import BaseModel, Field


class AlertEvent(BaseModel):
    type: str = ""
    timestamp: str = ""
    source: str = ""
    actor_user_id: str = ""
    actor_username: str = ""
    actor_display_name: str = ""


class DeliveryTarget(BaseModel):
    provider: str = ""
    channel: str = ""
    channel_name: str = ""
    post_id: str = ""


class Alert(BaseModel):
    fingerprint: str = ""
    alert_number: int = 0
    status: str = ""
    acknowledged: bool = False
    silenced: bool = False
    labels: dict[str, str] = Field(default_factory=dict)
    annotations: dict[str, str] = Field(default_factory=dict)
    values: dict[str, float] = Field(default_factory=dict)
    starts_at: str = ""
    ends_at: str = ""
    generator_url: str = ""
    events: list[AlertEvent] = Field(default_factory=list)
    delivery_targets: list[DeliveryTarget] = Field(default_factory=list)
    created_at: str = ""
    updated_at: str = ""


class CorrelatedAlert(BaseModel):
    fingerprint: str = ""
    alert_number: int = 0
    labels: dict[str, str] = Field(default_factory=dict)
    annotations: dict[str, str] = Field(default_factory=dict)
    status: str = ""
    starts_at: str = ""
    values: dict[str, float] = Field(default_factory=dict)
    generator_url: str = ""


class InvestigationResult(BaseModel):
    status: str = ""
    root_cause: str = ""
    resolution: str = ""
    summary: str = ""
    evidence: list[str] = Field(default_factory=list)
    recommended_actions: list[str] = Field(default_factory=list)
    severity_assessment: str = ""
    escalation_level: str = ""
    raw_response: str = ""


class InvestigationUpdate(BaseModel):
    id: str = ""
    type: str = ""
    message: str = ""
    source: str = ""
    internal: bool = False
    edited: bool = False
    user_id: str = ""
    username: str = ""
    mm_post_id: str = ""
    slack_message_ts: str = ""
    quoted_update_id: str = ""
    mentions: list[str] = Field(default_factory=list)
    created_at: str = ""


class Investigation(BaseModel):
    model_config = {"populate_by_name": True}

    id: str = ""
    investigation_id: str = ""
    investigation_number: int = 0
    alerts: list[CorrelatedAlert] = Field(default_factory=list)
    severity: str = ""
    correlation_key: str = ""
    status: str = ""
    result: Optional[InvestigationResult] = None
    mm_post_id: str = ""
    mm_thread_id: str = ""
    primary_thread_id: str = ""
    slack_channel_id: str = ""
    slack_thread_ts: str = ""
    twilio_call_sid: str = ""
    agent_id: str = ""
    agent_name: str = ""
    agent_type: str = ""
    escalation_level: str = ""
    updates: list[InvestigationUpdate] = Field(default_factory=list)
    created_at: str = ""
    updated_at: str = ""
    completed_at: str = ""
    started_at: str = ""
    investigating_duration_ms: int = 0


class KnowledgeNote(BaseModel):
    id: str = ""
    kind: str = ""
    title: str = ""
    body_markdown: str = ""
    tags: list[str] = Field(default_factory=list)
    selectors: dict[str, str] = Field(default_factory=dict)
    source_investigation_id: str = ""
    confidence: float = 0.0
    expires_at: str = ""
    author_type: str = ""
    author_name: str = ""
    created_at: str = ""
    updated_at: str = ""


class Memory(BaseModel):
    id: str = ""
    content: str = ""
    memory_type: str = ""
    hash: str = ""
    agent_id: str = ""
    agent_name: str = ""
    agent_type: str = ""
    investigation_id: str = ""
    correlation_key: str = ""
    labels: dict[str, str] = Field(default_factory=dict)
    entities: list[str] = Field(default_factory=list)
    metadata: dict[str, Any] = Field(default_factory=dict)
    confidence: float = 0.0
    access_count: int = 0
    score: float = 0.0
    expires_at: str = ""
    created_at: str = ""
    updated_at: str = ""


class PeerAsk(BaseModel):
    id: str = ""
    from_agent_id: str = ""
    from_agent_name: str = ""
    from_agent_type: str = ""
    investigation_id: str = ""
    to_agent_id: str = ""
    to_agent_type: str = ""
    question: str = ""
    reply: str = ""
    replied_by_agent_id: str = ""
    replied_by_agent_name: str = ""
    status: str = ""
    expires_at: str = ""
    created_at: str = ""
    answered_at: str = ""


class Service(BaseModel):
    id: str = ""
    name: str = ""
    description: str = ""
    status: str = ""
    labels: dict[str, str] = Field(default_factory=dict)
    team_id: str = ""
    created_at: str = ""
    updated_at: str = ""


class Incident(BaseModel):
    id: str = ""
    title: str = ""
    description: str = ""
    severity: str = ""
    priority: str = ""
    status: str = ""
    commander_id: str = ""
    service_id: str = ""
    team_id: str = ""
    sla_target_respond_at: str = ""
    sla_target_resolve_at: str = ""
    created_at: str = ""
    updated_at: str = ""
    resolved_at: str = ""
    closed_at: str = ""


class ConnectedEvent(BaseModel):
    client_id: str = ""
    agent_id: str = ""


class MessageEvent(BaseModel):
    type: str = ""
    message_id: str = ""
    chat_id: str = ""
    text: str = ""
    sender_id: str = ""
    sender_name: str = ""


class TypingEvent(BaseModel):
    type: str = ""
    chat_id: str = ""
    active: bool = False


class InvestigationSignalEvent(BaseModel):
    investigation_id: str = ""
    reason: str = ""
    actor: str = ""


class PeerFindingEvent(BaseModel):
    type: str = ""
    investigation_id: str = ""
    peer_agent_id: str = ""
    peer_agent_type: str = ""
    text: str = ""
    labels: dict[str, str] = Field(default_factory=dict)
    created_at: str = ""


class PeerAskEvent(BaseModel):
    type: str = ""
    ask_id: str = ""
    from_agent_id: str = ""
    from_agent_name: str = ""
    from_agent_type: str = ""
    investigation_id: str = ""
    question: str = ""
    expires_at: str = ""
    created_at: str = ""


class PeerReplyEvent(BaseModel):
    type: str = ""
    ask_id: str = ""
    investigation_id: str = ""
    reply: str = ""
    replied_by_agent_id: str = ""
    replied_by_agent_name: str = ""
    answered_at: str = ""


class AgentPresenceEvent(BaseModel):
    agent_id: str = ""
    online: bool = False


class AlertListResponse(BaseModel):
    alerts: list[Alert] = Field(default_factory=list)
    items: list[Alert] = Field(default_factory=list)
    total: int = 0
    limit: int = 0
    skip: int = 0


class InvestigationListResponse(BaseModel):
    investigations: list[Investigation] = Field(default_factory=list)
    items: list[Investigation] = Field(default_factory=list)
    total: int = 0
    limit: int = 0
    skip: int = 0


class KnowledgeListResponse(BaseModel):
    notes: list[KnowledgeNote] = Field(default_factory=list)
    items: list[KnowledgeNote] = Field(default_factory=list)
    total: int = 0
    limit: int = 0
    skip: int = 0


class MemoryListResponse(BaseModel):
    memories: list[Memory] = Field(default_factory=list)
    total: int = 0


class PeerAskListResponse(BaseModel):
    asks: list[PeerAsk] = Field(default_factory=list)
    total: int = 0


class SendMessageResponse(BaseModel):
    status: str = ""
    message_id: str = ""


class CommandResponse(BaseModel):
    ok: bool = False
    op: str = ""
    investigation_id: str | None = None
    error: str = ""


class PlaybookStep(BaseModel):
    id: str = ""
    step_number: int = 0
    title: str = ""
    description: str = ""
    expected_duration: str = ""
    command: str = ""


class Playbook(BaseModel):
    id: str = ""
    title: str = ""
    kind: str = ""
    summary: str = ""
    service_id: str = ""
    label_selectors: list[dict[str, Any]] = Field(default_factory=list)
    tags: list[str] = Field(default_factory=list)
    steps: list[PlaybookStep] = Field(default_factory=list)
    created_by: str = ""
    created_at: str = ""
    updated_at: str = ""


class Capability(BaseModel):
    id: str = ""
    name: str = ""
    description: str = ""
