from __future__ import annotations

from typing import Any, Optional

from pydantic import BaseModel, ConfigDict, Field


class _AlgaModel(BaseModel):
    model_config = ConfigDict(extra="allow")


class AlertEvent(_AlgaModel):
    type: str = ""
    timestamp: str = ""
    actor_username: str = ""
    actor_display_name: str = ""
    actor_user_id: str = ""
    source: str = ""


class DeliveryTarget(_AlgaModel):
    provider: str = ""
    channel: str = ""
    channel_name: str = ""
    post_id: str = ""


class Alert(_AlgaModel):
    fingerprint: str = ""
    status: str = ""
    acknowledged: bool = False
    silenced: bool = False
    labels: dict[str, str] = Field(default_factory=dict)
    annotations: dict[str, str] = Field(default_factory=dict)
    values: dict[str, float] = Field(default_factory=dict)
    starts_at: Optional[str] = None
    ends_at: Optional[str] = None
    generator_url: str = ""
    events: list[AlertEvent] = Field(default_factory=list)
    delivery_targets: list[DeliveryTarget] = Field(default_factory=list)
    alert_number: int = 0
    created_at: str = ""
    updated_at: str = ""


class KnowledgeNote(_AlgaModel):
    id: str = ""
    kind: str = ""
    title: str = ""
    body_markdown: str = ""
    tags: list[str] = Field(default_factory=list)
    selectors: dict[str, str] = Field(default_factory=dict)
    source_investigation_id: str = ""
    confidence: Optional[float] = None
    expires_at: Optional[str] = None
    author_type: str = ""
    author_name: str = ""
    created_at: str = ""
    updated_at: str = ""


class Memory(_AlgaModel):
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
    confidence: Optional[float] = None
    access_count: int = 0
    score: float = 0.0
    expires_at: Optional[str] = None
    created_at: str = ""
    updated_at: str = ""


class PeerAsk(_AlgaModel):
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
    answered_at: Optional[str] = None


class Service(_AlgaModel):
    id: str = ""
    name: str = ""
    description: str = ""
    status: str = ""
    labels: dict[str, str] = Field(default_factory=dict)
    team_id: str = ""
    created_at: str = ""
    updated_at: str = ""


class Incident(_AlgaModel):
    id: str = ""
    incident_number: int = 0
    title: str = ""
    description: str = ""
    summary: str = ""
    status: str = ""
    severity: str = ""
    priority: str = ""
    commander_id: str = ""
    service_id: str = ""
    team_id: str = ""
    sla_target_respond_at: Optional[str] = None
    sla_target_resolve_at: Optional[str] = None
    created_at: str = ""
    updated_at: str = ""
    resolved_at: Optional[str] = None
    closed_at: Optional[str] = None


class IncidentRole(_AlgaModel):
    role_type: str = ""
    assignee_type: str = ""
    agent_token_id: str = ""
    agent_name: str = ""
    user_id: str = ""
    user_name: str = ""
    status: str = ""


class IncidentContext(_AlgaModel):
    incident: Incident = Field(default_factory=Incident)
    roles: list[IncidentRole] = Field(default_factory=list)


class OnCallEntry(_AlgaModel):
    schedule_id: str = ""
    schedule_name: str = ""
    user_id: str = ""
    user_name: str = ""


class SecretValue(_AlgaModel):
    secret_id: str = ""
    name: str = ""
    value: str = ""
    fetched_at: str = ""


class PlaybookStep(_AlgaModel):
    id: str = ""
    step_number: int = 0
    title: str = ""
    description: str = ""
    expected_duration: str = ""
    command: str = ""


class Playbook(_AlgaModel):
    id: str = ""
    title: str = ""
    kind: str = ""
    summary: str = ""
    service_id: str = ""
    label_selectors: list[dict[str, str]] = Field(default_factory=list)
    tags: list[str] = Field(default_factory=list)
    steps: list[PlaybookStep] = Field(default_factory=list)
    created_by: str = ""
    created_at: str = ""
    updated_at: str = ""


class Capability(_AlgaModel):
    id: str = ""
    name: str = ""
    description: str = ""


class ConnectedEvent(_AlgaModel):
    client_id: str = ""
    agent_id: str = ""


class MessageEvent(_AlgaModel):
    type: str = ""
    chat_id: str = ""
    text: str = ""
    sender_id: str = ""
    sender_name: str = ""
    message_id: str = ""
    trigger: str = ""
    reply_to_message_id: str = ""
    reply_to_text: str = ""
    mentions: list[str] = Field(default_factory=list)


class TypingEvent(_AlgaModel):
    type: str = ""
    chat_id: str = ""
    active: bool = False


class InvestigationSignalEvent(_AlgaModel):
    investigation_id: str = ""
    alert_investigation_id: str = ""
    reason: str = ""
    actor: str = ""


class PeerFindingEvent(_AlgaModel):
    type: str = ""
    investigation_id: str = ""
    peer_agent_id: str = ""
    peer_agent_type: str = ""
    text: str = ""
    labels: dict[str, str] = Field(default_factory=dict)
    created_at: str = ""


class PeerAskEvent(_AlgaModel):
    type: str = ""
    ask_id: str = ""
    from_agent_id: str = ""
    from_agent_name: str = ""
    from_agent_type: str = ""
    investigation_id: str = ""
    question: str = ""
    expires_at: str = ""
    created_at: str = ""


class PeerReplyEvent(_AlgaModel):
    type: str = ""
    ask_id: str = ""
    investigation_id: str = ""
    reply: str = ""
    replied_by_agent_id: str = ""
    replied_by_agent_name: str = ""
    answered_at: str = ""


class AgentPresenceEvent(_AlgaModel):
    agent_id: str = ""
    online: bool = False


class SummarizeIncidentEvent(_AlgaModel):
    incident_number: int = 0
    chat_id: str = ""
    incident: Optional[dict[str, Any]] = None


class AlertAutoResolvedEvent(_AlgaModel):
    investigation_id: str = ""
    fingerprint: str = ""
    alert_name: str = ""


class IncidentCommsStaleEvent(_AlgaModel):
    incident_number: int = 0
    trigger: str = ""
    reason: str = ""


class AlertListResponse(_AlgaModel):
    alerts: list[Alert] = Field(default_factory=list)
    items: list[Alert] = Field(default_factory=list)
    total: int = 0


class KnowledgeListResponse(_AlgaModel):
    items: list[KnowledgeNote] = Field(default_factory=list)
    notes: list[KnowledgeNote] = Field(default_factory=list)
    total: int = 0


class MemoryListResponse(_AlgaModel):
    items: list[Memory] = Field(default_factory=list)
    memories: list[Memory] = Field(default_factory=list)
    total: int = 0


class PeerAskListResponse(_AlgaModel):
    items: list[PeerAsk] = Field(default_factory=list)
    asks: list[PeerAsk] = Field(default_factory=list)
    total: int = 0


class ServiceListResponse(_AlgaModel):
    items: list[Service] = Field(default_factory=list)
    total: int = 0


class SendMessageResponse(_AlgaModel):
    status: str = ""
    message_id: str = ""


class CommandResponse(_AlgaModel):
    ok: bool = False
    op: str = ""
    chat_id: str = ""
    investigation_id: str = ""
    incident_number: int = 0
    error: str = ""
