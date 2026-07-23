use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "op")]
pub enum InvestigationCommand {
    // --- Alert investigation tools ---
    #[serde(rename = "resolve_alert")]
    ResolveAlert {
        fingerprint: String,
        #[serde(skip_serializing_if = "Option::is_none")]
        root_cause: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        resolution: Option<String>,
    },
    #[serde(rename = "reopen_alert")]
    ReopenAlert {
        #[serde(skip_serializing_if = "Option::is_none")]
        fingerprint: Option<String>,
    },
    #[serde(rename = "set_outcome")]
    SetOutcome {
        #[serde(skip_serializing_if = "Option::is_none")]
        root_cause: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        resolution: Option<String>,
    },
    #[serde(rename = "cancel_investigation")]
    CancelInvestigation {
        #[serde(skip_serializing_if = "Option::is_none")]
        reason: Option<String>,
    },
    #[serde(rename = "pause_investigation")]
    PauseInvestigation {
        #[serde(skip_serializing_if = "Option::is_none")]
        reason: Option<String>,
    },
    #[serde(rename = "triage_feedback")]
    TriageFeedback {
        triage_result_id: String,
        agreed: bool,
        #[serde(skip_serializing_if = "Option::is_none")]
        correct_decision: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        correct_severity: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        note: Option<String>,
    },
    #[serde(rename = "assign_investigation")]
    AssignInvestigation { target_agent_id: String },
    #[serde(rename = "promote_to_incident")]
    PromoteToIncident {
        #[serde(skip_serializing_if = "Option::is_none")]
        title: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        severity: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        priority: Option<String>,
    },

    // --- Incident tools ---
    #[serde(rename = "set_incident_priority")]
    SetIncidentPriority {
        #[serde(rename = "incident_number")]
        incident_number: i64,
        priority: String,
    },
    #[serde(rename = "set_incident_severity")]
    SetIncidentSeverity {
        #[serde(rename = "incident_number")]
        incident_number: i64,
        severity: String,
    },
    #[serde(rename = "trigger_escalation")]
    TriggerEscalation {
        #[serde(rename = "incident_number")]
        incident_number: i64,
    },
    #[serde(rename = "request_status_update")]
    RequestStatusUpdate {
        #[serde(rename = "incident_number")]
        incident_number: i64,
    },
    #[serde(rename = "mitigate_incident")]
    MitigateIncident {
        #[serde(rename = "incident_number")]
        incident_number: i64,
        #[serde(skip_serializing_if = "Option::is_none")]
        reason: Option<String>,
    },
    /// ResolveIncident is for commander-capable agents only. Investigator agents
    /// should record outcomes and ask the commander to verify resolution through
    /// coordination.
    #[serde(rename = "resolve_incident")]
    ResolveIncident {
        #[serde(rename = "incident_number")]
        incident_number: i64,
        #[serde(skip_serializing_if = "Option::is_none")]
        reason: Option<String>,
    },
    #[serde(rename = "begin_triage")]
    BeginTriage {
        #[serde(rename = "incident_number")]
        incident_number: i64,
    },
    #[serde(rename = "promote_incident")]
    PromoteIncident {
        #[serde(rename = "incident_number")]
        incident_number: i64,
    },
    #[serde(rename = "assign_incident_role")]
    AssignIncidentRole {
        #[serde(rename = "incident_number")]
        incident_number: i64,
        role_type: String,
        #[serde(skip_serializing_if = "Option::is_none")]
        user_id: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        agent_token_id: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        scope_description: Option<String>,
    },

    // --- Coordination / status tools ---
    /// Post a coordination handoff message. `message`, `audience`, and `urgency`
    /// are required by the backend. Do NOT pass `status_level` here — status
    /// milestones must be published via `PublishStatusUpdate`.
    #[serde(rename = "post_handoff")]
    PostHandoff {
        #[serde(rename = "incident_number")]
        incident_number: i64,
        message: String,
        audience: String,
        urgency: String,
    },
    /// Publish a status update. `message` and `status_level` are required.
    #[serde(rename = "publish_status_update")]
    PublishStatusUpdate {
        #[serde(rename = "incident_number")]
        incident_number: i64,
        message: String,
        status_level: String,
        #[serde(skip_serializing_if = "Option::is_none")]
        impact_assessment: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        actions_taken: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        eta: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        source_coordination_message_id: Option<String>,
    },
    /// Set incident resolution documents. At least one of the optional fields
    /// must be present.
    #[serde(rename = "set_incident_resolution_docs")]
    SetIncidentResolutionDocs {
        #[serde(rename = "incident_number")]
        incident_number: i64,
        #[serde(skip_serializing_if = "Option::is_none")]
        summary: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        impact_assessment: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        actions_taken: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        root_cause: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        resolution: Option<String>,
    },
}
