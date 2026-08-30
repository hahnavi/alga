from __future__ import annotations

from typing import Any, Optional

from pydantic import BaseModel, ConfigDict


class InvestigationCommand(BaseModel):
    model_config = ConfigDict(extra="allow")

    op: str
    chat_id: Optional[str] = None
    fingerprint: Optional[str] = None
    reason: Optional[str] = None
    note: Optional[str] = None
    priority: Optional[str] = None
    severity: Optional[str] = None
    title: Optional[str] = None
    root_cause: Optional[str] = None
    resolution: Optional[str] = None
    summary: Optional[str] = None
    impact_assessment: Optional[str] = None
    actions_taken: Optional[str] = None
    eta: Optional[str] = None
    triage_result_id: Optional[str] = None
    agreed: Optional[bool] = None
    correct_decision: Optional[str] = None
    correct_severity: Optional[str] = None
    target_agent_id: Optional[str] = None
    role_type: Optional[str] = None
    user_id: Optional[str] = None
    agent_token_id: Optional[str] = None
    scope_description: Optional[str] = None
    incident_number: Optional[int] = None
    message: Optional[str] = None
    audience: Optional[str] = None
    urgency: Optional[str] = None
    status_level: Optional[str] = None
    source_coordination_message_id: Optional[str] = None
    internal: Optional[bool] = None


def resolve_alert(fingerprint: str) -> InvestigationCommand:
    return InvestigationCommand(op="resolve_alert", fingerprint=fingerprint)


def reopen_alert(fingerprint: str) -> InvestigationCommand:
    return InvestigationCommand(op="reopen_alert", fingerprint=fingerprint)


def set_outcome(
    root_cause: str | None = None,
    resolution: str | None = None,
) -> InvestigationCommand:
    return InvestigationCommand(op="set_outcome", root_cause=root_cause, resolution=resolution)


def cancel_investigation(reason: str | None = None) -> InvestigationCommand:
    return InvestigationCommand(op="cancel_investigation", reason=reason)


def pause_investigation(reason: str | None = None) -> InvestigationCommand:
    return InvestigationCommand(op="pause_investigation", reason=reason)


def triage_feedback(
    triage_result_id: str,
    agreed: bool = True,
    correct_decision: str | None = None,
    correct_severity: str | None = None,
    note: str | None = None,
) -> InvestigationCommand:
    return InvestigationCommand(
        op="triage_feedback",
        triage_result_id=triage_result_id,
        agreed=agreed,
        correct_decision=correct_decision,
        correct_severity=correct_severity,
        note=note,
    )


def assign_investigation(target_agent_id: str) -> InvestigationCommand:
    return InvestigationCommand(op="assign_investigation", target_agent_id=target_agent_id)


def promote_to_incident(
    title: str | None = None,
    severity: str | None = None,
    priority: str | None = None,
) -> InvestigationCommand:
    return InvestigationCommand(
        op="promote_to_incident", title=title, severity=severity, priority=priority
    )


def set_incident_priority(incident_number: int, priority: str) -> InvestigationCommand:
    return InvestigationCommand(
        op="set_incident_priority", incident_number=incident_number, priority=priority
    )


def set_incident_severity(incident_number: int, severity: str) -> InvestigationCommand:
    return InvestigationCommand(
        op="set_incident_severity", incident_number=incident_number, severity=severity
    )


def trigger_escalation(incident_number: int) -> InvestigationCommand:
    return InvestigationCommand(op="trigger_escalation", incident_number=incident_number)


def mitigate_incident(incident_number: int, reason: str | None = None) -> InvestigationCommand:
    return InvestigationCommand(
        op="mitigate_incident", incident_number=incident_number, reason=reason
    )


def resolve_incident(incident_number: int, reason: str | None = None) -> InvestigationCommand:
    return InvestigationCommand(
        op="resolve_incident", incident_number=incident_number, reason=reason
    )


def begin_triage(incident_number: int) -> InvestigationCommand:
    return InvestigationCommand(op="begin_triage", incident_number=incident_number)


def promote_incident(incident_number: int) -> InvestigationCommand:
    return InvestigationCommand(op="promote_incident", incident_number=incident_number)


def assign_incident_role_to_user(
    incident_number: int,
    role_type: str,
    user_id: str,
    scope_description: str | None = None,
) -> InvestigationCommand:
    return InvestigationCommand(
        op="assign_incident_role",
        incident_number=incident_number,
        role_type=role_type,
        user_id=user_id,
        scope_description=scope_description,
    )


def assign_incident_role_to_agent(
    incident_number: int,
    role_type: str,
    agent_token_id: str,
    scope_description: str | None = None,
) -> InvestigationCommand:
    return InvestigationCommand(
        op="assign_incident_role",
        incident_number=incident_number,
        role_type=role_type,
        agent_token_id=agent_token_id,
        scope_description=scope_description,
    )


def post_handoff(
    incident_number: int, message: str, audience: str, urgency: str
) -> InvestigationCommand:
    return InvestigationCommand(
        op="post_handoff",
        incident_number=incident_number,
        message=message,
        audience=audience,
        urgency=urgency,
    )


def publish_status_update(
    incident_number: int, message: str, status_level: str
) -> InvestigationCommand:
    return InvestigationCommand(
        op="publish_status_update",
        incident_number=incident_number,
        message=message,
        status_level=status_level,
    )


def set_incident_resolution_docs(
    incident_number: int,
    summary: str | None = None,
    impact_assessment: str | None = None,
    actions_taken: str | None = None,
    root_cause: str | None = None,
    resolution: str | None = None,
) -> InvestigationCommand:
    return InvestigationCommand(
        op="set_incident_resolution_docs",
        incident_number=incident_number,
        summary=summary,
        impact_assessment=impact_assessment,
        actions_taken=actions_taken,
        root_cause=root_cause,
        resolution=resolution,
    )
