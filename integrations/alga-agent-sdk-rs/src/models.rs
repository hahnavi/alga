use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct Alert {
    pub fingerprint: String,
    pub alert_number: i64,
    pub status: String,
    pub acknowledged: bool,
    pub silenced: bool,
    pub labels: HashMap<String, String>,
    pub annotations: HashMap<String, String>,
    pub values: HashMap<String, f64>,
    pub starts_at: String,
    pub ends_at: String,
    pub generator_url: String,
    pub events: Vec<AlertEvent>,
    pub delivery_targets: Vec<DeliveryTarget>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct AlertEvent {
    #[serde(rename = "type")]
    pub event_type: String,
    pub timestamp: String,
    pub source: String,
    pub actor_user_id: String,
    pub actor_username: String,
    pub actor_display_name: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct DeliveryTarget {
    pub provider: String,
    pub channel: String,
    #[serde(default)]
    pub channel_name: String,
    #[serde(default)]
    pub post_id: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct CorrelatedAlert {
    pub fingerprint: String,
    pub alert_number: i64,
    pub labels: HashMap<String, String>,
    pub annotations: HashMap<String, String>,
    pub status: String,
    pub starts_at: String,
    pub values: HashMap<String, f64>,
    pub generator_url: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct InvestigationResult {
    pub status: String,
    pub root_cause: String,
    pub resolution: String,
    pub summary: String,
    pub evidence: Vec<String>,
    pub recommended_actions: Vec<String>,
    pub severity_assessment: String,
    pub escalation_level: String,
    pub raw_response: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct InvestigationUpdate {
    pub id: String,
    #[serde(rename = "type")]
    pub update_type: String,
    pub message: String,
    pub source: String,
    pub internal: bool,
    pub edited: bool,
    pub user_id: String,
    pub username: String,
    pub mm_post_id: String,
    pub slack_message_ts: String,
    pub quoted_update_id: String,
    pub mentions: Vec<String>,
    pub created_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct Investigation {
    pub id: String,
    pub investigation_id: String,
    pub investigation_number: i64,
    pub alerts: Vec<CorrelatedAlert>,
    pub severity: String,
    pub correlation_key: String,
    pub status: String,
    pub result: Option<InvestigationResult>,
    pub mm_post_id: String,
    pub mm_thread_id: String,
    pub primary_thread_id: String,
    pub slack_channel_id: String,
    pub slack_thread_ts: String,
    pub twilio_call_sid: String,
    pub agent_id: String,
    pub agent_name: String,
    pub agent_type: String,
    pub escalation_level: String,
    pub updates: Vec<InvestigationUpdate>,
    pub created_at: String,
    pub updated_at: String,
    pub completed_at: String,
    pub started_at: String,
    pub investigating_duration_ms: i64,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct KnowledgeNote {
    pub id: String,
    pub kind: String,
    pub title: String,
    pub body_markdown: String,
    pub tags: Vec<String>,
    pub selectors: HashMap<String, String>,
    pub source_investigation_id: String,
    pub confidence: Option<f64>,
    pub expires_at: String,
    pub author_type: String,
    pub author_name: String,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct Memory {
    pub id: String,
    pub content: String,
    pub memory_type: String,
    pub hash: String,
    pub agent_id: String,
    pub agent_name: String,
    pub agent_type: String,
    pub investigation_id: String,
    pub correlation_key: String,
    pub labels: HashMap<String, String>,
    pub entities: Vec<String>,
    pub metadata: HashMap<String, serde_json::Value>,
    pub confidence: Option<f64>,
    pub access_count: i64,
    pub score: f64,
    pub expires_at: String,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct PeerAsk {
    pub id: String,
    pub from_agent_id: String,
    pub from_agent_name: String,
    pub from_agent_type: String,
    pub investigation_id: String,
    pub to_agent_id: String,
    pub to_agent_type: String,
    pub question: String,
    pub reply: String,
    pub replied_by_agent_id: String,
    pub replied_by_agent_name: String,
    pub status: String,
    pub expires_at: String,
    pub created_at: String,
    pub answered_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct Service {
    pub id: String,
    pub name: String,
    pub description: String,
    pub status: String,
    pub labels: HashMap<String, String>,
    pub team_id: String,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct Incident {
    pub id: String,
    pub title: String,
    pub description: String,
    pub severity: String,
    pub priority: String,
    pub status: String,
    pub commander_id: String,
    pub service_id: String,
    pub team_id: String,
    pub sla_target_respond_at: String,
    pub sla_target_resolve_at: String,
    pub created_at: String,
    pub updated_at: String,
    pub resolved_at: String,
    pub closed_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct ConnectedEvent {
    pub agent_id: String,
    pub client_id: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct MessageEvent {
    pub message_id: String,
    pub chat_id: String,
    pub text: String,
    pub kind: String,
    pub sender_type: String,
    pub sender_id: String,
    pub sender_name: String,
    pub mentions: Vec<String>,
    pub created_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct TypingEvent {
    #[serde(rename = "type")]
    pub event_type: String,
    pub chat_id: String,
    pub active: bool,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct InvestigationSignalEvent {
    pub investigation_id: String,
    pub reason: String,
    pub actor: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct PeerFindingEvent {
    pub agent_id: String,
    pub agent_name: String,
    pub investigation_id: String,
    pub finding: String,
    pub severity: String,
    pub created_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct PeerAskEvent {
    pub id: String,
    pub question: String,
    pub context: String,
    pub asking_agent_id: String,
    pub asking_agent_name: String,
    pub target_agent_id: Option<String>,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct PeerReplyEvent {
    pub id: String,
    pub reply: String,
    pub replied_by: String,
    pub replied_by_name: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct AgentPresenceEvent {
    pub agent_id: String,
    pub online: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AlertListResponse {
    #[serde(default)]
    pub alerts: Vec<Alert>,
    #[serde(default)]
    pub items: Vec<Alert>,
    #[serde(default)]
    pub total: u64,
    #[serde(default)]
    pub limit: u64,
    #[serde(default)]
    pub skip: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct InvestigationListResponse {
    #[serde(default)]
    pub investigations: Vec<Investigation>,
    #[serde(default)]
    pub items: Vec<Investigation>,
    #[serde(default)]
    pub total: u64,
    #[serde(default)]
    pub limit: u64,
    #[serde(default)]
    pub skip: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct KnowledgeListResponse {
    #[serde(default)]
    pub notes: Vec<KnowledgeNote>,
    #[serde(default)]
    pub items: Vec<KnowledgeNote>,
    #[serde(default)]
    pub total: u64,
    #[serde(default)]
    pub limit: u64,
    #[serde(default)]
    pub skip: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MemoryListResponse {
    #[serde(default)]
    pub memories: Vec<Memory>,
    #[serde(default)]
    pub total: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PeerAskListResponse {
    #[serde(default)]
    pub peer_asks: Vec<PeerAsk>,
    #[serde(default)]
    pub total: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SendMessageResponse {
    #[serde(default)]
    pub message_id: String,
    #[serde(default)]
    pub chat_id: String,
    #[serde(default)]
    pub created_at: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CommandResponse {
    #[serde(default)]
    pub ok: bool,
    #[serde(default)]
    pub op: String,
    #[serde(default)]
    pub investigation_id: Option<String>,
    #[serde(default)]
    pub error: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ListAlertsParams {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub status: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub severity: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub provider: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub search: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub limit: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub skip: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ListInvestigationsParams {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub status: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub severity: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub search: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub incident_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub limit: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub skip: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ListKnowledgeParams {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub search: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tags: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub limit: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub skip: Option<u64>,
}

/// Body for `POST /api/v1/agent/knowledge`. The backend requires
/// `source_investigation_id` and `confidence` for agent-authored notes; both
/// are exposed here as required fields so callers get a compile-time prompt.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct CreateKnowledgeParams {
    pub title: String,
    /// Markdown body of the note; serialized as `body_markdown` to match the
    /// backend's `KnowledgeNote.BodyMarkdown`.
    #[serde(rename = "body_markdown")]
    pub body_markdown: String,
    /// Agent-authored notes must reference the investigation they came from.
    pub source_investigation_id: String,
    /// Required for agent-authored notes (0.0..=1.0).
    pub confidence: f64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub kind: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub tags: Option<Vec<String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub selectors: Option<HashMap<String, String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub expires_at: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ListPeerAsksParams {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub status: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub limit: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub skip: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ListMemoriesParams {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub search: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub limit: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub skip: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct CreateMemoryParams {
    pub content: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub memory_type: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub investigation_id: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct CreatePeerAskParams {
    pub question: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub context: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub target_agent_id: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct AddTimelineEntryParams {
    pub entry_type: String,
    pub message: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub details: Option<HashMap<String, serde_json::Value>>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OnCallEntry {
    #[serde(default)]
    pub schedule_id: String,
    #[serde(default)]
    pub schedule_name: String,
    #[serde(default)]
    pub user_id: String,
    #[serde(default)]
    pub user_name: String,
    #[serde(default)]
    pub user_email: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct OnCallResponse {
    #[serde(default)]
    pub on_call: Vec<OnCallEntry>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PlaybookStep {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub step_number: i32,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub description: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub expected_duration: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub command: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Playbook {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub title: String,
    #[serde(default)]
    pub kind: String,
    #[serde(default)]
    pub summary: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub service_id: Option<String>,
    #[serde(default)]
    pub label_selectors: Vec<serde_json::Value>,
    #[serde(default)]
    pub tags: Vec<String>,
    #[serde(default)]
    pub steps: Vec<PlaybookStep>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub created_by: Option<String>,
    #[serde(default)]
    pub created_at: String,
    #[serde(default)]
    pub updated_at: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Capability {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub name: String,
    #[serde(default)]
    pub description: String,
}
