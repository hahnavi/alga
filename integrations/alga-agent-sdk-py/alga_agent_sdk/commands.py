from __future__ import annotations

from typing import Any


class InvestigationCommand:
    def __init__(self, op: str, **fields: Any):
        self.op = op
        self.fields = fields

    def to_dict(self) -> dict[str, Any]:
        result: dict[str, Any] = {"op": self.op}
        for k, v in self.fields.items():
            if v is not None:
                result[k] = v
        return result


# --- Alert investigation tools ---

def resolve_alert(fingerprint: str) -> InvestigationCommand:
    return InvestigationCommand("resolve_alert", fingerprint=fingerprint)


def reopen_alert(fingerprint: str) -> InvestigationCommand:
    return InvestigationCommand("reopen_alert", fingerprint=fingerprint)


def set_outcome(root_cause: str | None = None, resolution: str | None = None) -> InvestigationCommand:
    return InvestigationCommand("set_outcome", root_cause=root_cause, resolution=resolution)


def cancel_investigation(reason: str | None = None) -> InvestigationCommand:
    return InvestigationCommand("cancel_investigation", reason=reason)


def pause_investigation(reason: str | None = None) -> InvestigationCommand:
    return InvestigationCommand("pause_investigation", reason=reason)


def triage_feedback(
    triage_result_id: str,
    agreed: bool,
    correct_decision: str | None = None,
    correct_severity: str | None = None,
    note: str | None = None,
) -> InvestigationCommand:
    return InvestigationCommand(
        "triage_feedback",
        triage_result_id=triage_result_id,
        agreed=agreed,
        correct_decision=correct_decision,
        correct_severity=correct_severity,
        note=note,
    )


def assign_investigation(target_agent_id: str) -> InvestigationCommand:
    return InvestigationCommand("assign_investigation", target_agent_id=target_agent_id)


def promote_to_incident(
    title: str | None = None,
    severity: str | None = None,
    priority: str | None = None,
) -> InvestigationCommand:
    return InvestigationCommand("promote_to_incident", title=title, severity=severity, priority=priority)


# --- Incident tools ---

def set_incident_priority(incident_id: str, priority: str) -> InvestigationCommand:
    return InvestigationCommand("set_incident_priority", incident_id=incident_id, priority=priority)


def set_incident_severity(incident_id: str, severity: str) -> InvestigationCommand:
    return InvestigationCommand("set_incident_severity", incident_id=incident_id, severity=severity)


def trigger_escalation(incident_id: str) -> InvestigationCommand:
    return InvestigationCommand("trigger_escalation", incident_id=incident_id)


def request_status_update(incident_id: str) -> InvestigationCommand:
    return InvestigationCommand("request_status_update", incident_id=incident_id)


def mitigate_incident(incident_id: str, reason: str | None = None) -> InvestigationCommand:
    return InvestigationCommand("mitigate_incident", incident_id=incident_id, reason=reason)


def resolve_incident(incident_id: str, reason: str | None = None) -> InvestigationCommand:
    return InvestigationCommand("resolve_incident", incident_id=incident_id, reason=reason)


def begin_triage(incident_id: str) -> InvestigationCommand:
    return InvestigationCommand("begin_triage", incident_id=incident_id)


def promote_incident(incident_id: str) -> InvestigationCommand:
    return InvestigationCommand("promote_incident", incident_id=incident_id)


def assign_incident_role(
    incident_id: str,
    role_type: str,
    user_id: str | None = None,
    agent_token_id: str | None = None,
    scope_description: str | None = None,
) -> InvestigationCommand:
    return InvestigationCommand(
        "assign_incident_role",
        incident_id=incident_id,
        role_type=role_type,
        user_id=user_id,
        agent_token_id=agent_token_id,
        scope_description=scope_description,
    )
