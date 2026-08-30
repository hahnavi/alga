pub mod client;
pub mod commands;
pub mod dedup;
pub mod error;
pub mod models;
pub mod sse;

pub use client::{AlgaClient, AlgaClientOptions};
pub use commands::{
    assign_incident_role_to_agent, assign_incident_role_to_user, assign_investigation,
    begin_triage, cancel_investigation, mitigate_incident, pause_investigation, post_handoff,
    promote_incident, promote_to_incident, publish_status_update, reopen_alert, resolve_alert,
    resolve_incident, set_incident_priority, set_incident_resolution_docs, set_incident_severity,
    set_outcome, triage_feedback, trigger_escalation, InvestigationCommand,
};
pub use dedup::MessageDedup;
pub use error::{is_auth_error, is_retryable_error, AlgaError};
pub use models::*;
pub use sse::{EventHandler, SSEClient};
