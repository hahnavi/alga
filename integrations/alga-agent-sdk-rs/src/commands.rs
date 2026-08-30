use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct InvestigationCommand {
    pub op: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub chat_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub fingerprint: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub reason: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub note: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub priority: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub severity: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub root_cause: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub resolution: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub summary: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub impact_assessment: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub actions_taken: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub eta: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub triage_result_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub agreed: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub correct_decision: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub correct_severity: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub target_agent_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub role_type: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub user_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub agent_token_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub scope_description: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub incident_number: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub message: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub audience: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub urgency: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub status_level: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub source_coordination_message_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub internal: Option<bool>,
}

impl InvestigationCommand {
    fn new(op: &str) -> Self {
        Self {
            op: op.to_string(),
            ..Default::default()
        }
    }
}

fn opt(val: &str) -> Option<String> {
    if val.is_empty() {
        None
    } else {
        Some(val.to_string())
    }
}

pub fn resolve_alert(fingerprint: &str) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("resolve_alert");
    cmd.fingerprint = Some(fingerprint.to_string());
    cmd
}

pub fn reopen_alert(fingerprint: &str) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("reopen_alert");
    cmd.fingerprint = Some(fingerprint.to_string());
    cmd
}

pub fn set_outcome(
    root_cause: Option<&str>,
    resolution: Option<&str>,
) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("set_outcome");
    cmd.root_cause = root_cause.map(|s| s.to_string());
    cmd.resolution = resolution.map(|s| s.to_string());
    cmd
}

pub fn cancel_investigation(reason: &str) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("cancel_investigation");
    cmd.reason = opt(reason);
    cmd
}

pub fn pause_investigation(reason: &str) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("pause_investigation");
    cmd.reason = opt(reason);
    cmd
}

pub fn triage_feedback(
    triage_result_id: &str,
    agreed: bool,
    correct_decision: &str,
    correct_severity: &str,
    note: &str,
) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("triage_feedback");
    cmd.triage_result_id = Some(triage_result_id.to_string());
    cmd.agreed = if agreed { Some(true) } else { None };
    cmd.correct_decision = opt(correct_decision);
    cmd.correct_severity = opt(correct_severity);
    cmd.note = opt(note);
    cmd
}

pub fn assign_investigation(target_agent_id: &str) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("assign_investigation");
    cmd.target_agent_id = Some(target_agent_id.to_string());
    cmd
}

pub fn promote_to_incident(
    title: &str,
    severity: &str,
    priority: &str,
) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("promote_to_incident");
    cmd.title = opt(title);
    cmd.severity = opt(severity);
    cmd.priority = opt(priority);
    cmd
}

pub fn set_incident_priority(
    incident_number: i64,
    priority: &str,
) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("set_incident_priority");
    cmd.incident_number = Some(incident_number);
    cmd.priority = Some(priority.to_string());
    cmd
}

pub fn set_incident_severity(
    incident_number: i64,
    severity: &str,
) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("set_incident_severity");
    cmd.incident_number = Some(incident_number);
    cmd.severity = Some(severity.to_string());
    cmd
}

pub fn trigger_escalation(incident_number: i64) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("trigger_escalation");
    cmd.incident_number = Some(incident_number);
    cmd
}

pub fn mitigate_incident(
    incident_number: i64,
    reason: &str,
) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("mitigate_incident");
    cmd.incident_number = Some(incident_number);
    cmd.reason = opt(reason);
    cmd
}

pub fn resolve_incident(
    incident_number: i64,
    reason: &str,
) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("resolve_incident");
    cmd.incident_number = Some(incident_number);
    cmd.reason = opt(reason);
    cmd
}

pub fn begin_triage(incident_number: i64) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("begin_triage");
    cmd.incident_number = Some(incident_number);
    cmd
}

pub fn promote_incident(incident_number: i64) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("promote_incident");
    cmd.incident_number = Some(incident_number);
    cmd
}

pub fn assign_incident_role_to_user(
    incident_number: i64,
    role_type: &str,
    user_id: &str,
    scope_description: &str,
) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("assign_incident_role");
    cmd.incident_number = Some(incident_number);
    cmd.role_type = Some(role_type.to_string());
    cmd.user_id = Some(user_id.to_string());
    if !scope_description.is_empty() {
        cmd.scope_description = Some(scope_description.to_string());
    }
    cmd
}

pub fn assign_incident_role_to_agent(
    incident_number: i64,
    role_type: &str,
    agent_token_id: &str,
    scope_description: &str,
) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("assign_incident_role");
    cmd.incident_number = Some(incident_number);
    cmd.role_type = Some(role_type.to_string());
    cmd.agent_token_id = Some(agent_token_id.to_string());
    if !scope_description.is_empty() {
        cmd.scope_description = Some(scope_description.to_string());
    }
    cmd
}

pub fn post_handoff(
    incident_number: i64,
    message: &str,
    audience: &str,
    urgency: &str,
) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("post_handoff");
    cmd.incident_number = Some(incident_number);
    cmd.message = Some(message.to_string());
    cmd.audience = Some(audience.to_string());
    cmd.urgency = Some(urgency.to_string());
    cmd
}

pub fn publish_status_update(
    incident_number: i64,
    message: &str,
    status_level: &str,
) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("publish_status_update");
    cmd.incident_number = Some(incident_number);
    cmd.message = Some(message.to_string());
    cmd.status_level = Some(status_level.to_string());
    cmd
}

pub fn set_incident_resolution_docs(
    incident_number: i64,
    summary: &str,
    impact_assessment: &str,
    actions_taken: &str,
    root_cause: &str,
    resolution: &str,
) -> InvestigationCommand {
    let mut cmd = InvestigationCommand::new("set_incident_resolution_docs");
    cmd.incident_number = Some(incident_number);
    cmd.summary = opt(summary);
    cmd.impact_assessment = opt(impact_assessment);
    cmd.actions_taken = opt(actions_taken);
    if !root_cause.is_empty() {
        cmd.root_cause = Some(root_cause.to_string());
    }
    if !resolution.is_empty() {
        cmd.resolution = Some(resolution.to_string());
    }
    cmd
}
