use serde::{Deserialize, Serialize};
use std::collections::HashMap;

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
    pub channel_name: String,
    pub post_id: String,
}

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
    pub starts_at: Option<String>,
    pub ends_at: Option<String>,
    pub generator_url: String,
    pub events: Vec<AlertEvent>,
    pub delivery_targets: Vec<DeliveryTarget>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct CoordinationTask {
    pub task_id: String,
    pub incident_number: i64,
    pub kind: String,
    pub goal: String,
    pub assignee_role: String,
    pub assignee_agent_id: String,
    pub status: String,
    pub result: Option<serde_json::Value>,
    pub input_context: Option<serde_json::Value>,
    pub parent_task_id: String,
    pub created_at: Option<String>,
    pub completed_at: Option<String>,
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
    pub expires_at: Option<String>,
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
    pub expires_at: Option<String>,
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
    pub answered_at: Option<String>,
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
    pub incident_number: i64,
    pub title: String,
    pub description: String,
    pub summary: String,
    pub status: String,
    pub severity: String,
    pub priority: String,
    pub commander_id: String,
    pub service_id: String,
    pub team_id: String,
    pub sla_target_respond_at: Option<String>,
    pub sla_target_resolve_at: Option<String>,
    pub created_at: String,
    pub updated_at: String,
    pub resolved_at: Option<String>,
    pub closed_at: Option<String>,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct IncidentRole {
    pub role_type: String,
    pub assignee_type: String,
    pub agent_token_id: String,
    pub agent_name: String,
    pub user_id: String,
    pub user_name: String,
    pub status: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct IncidentContext {
    pub incident: Incident,
    pub roles: Vec<IncidentRole>,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct OnCallEntry {
    pub schedule_id: String,
    pub schedule_name: String,
    pub user_id: String,
    pub user_name: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct SecretValue {
    pub secret_id: String,
    pub name: String,
    pub value: String,
    pub fetched_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct PlaybookStep {
    pub id: String,
    pub step_number: i32,
    pub title: String,
    pub description: String,
    pub expected_duration: String,
    pub command: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct Playbook {
    pub id: String,
    pub title: String,
    pub kind: String,
    pub summary: String,
    pub service_id: String,
    pub label_selectors: Vec<HashMap<String, String>>,
    pub tags: Vec<String>,
    pub steps: Vec<PlaybookStep>,
    pub created_by: String,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct ConnectedEvent {
    pub client_id: String,
    pub agent_id: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct MessageEvent {
    #[serde(rename = "type")]
    pub event_type: String,
    pub chat_id: String,
    pub text: String,
    pub sender_id: String,
    pub sender_name: String,
    pub message_id: String,
    pub trigger: String,
    pub reply_to_message_id: String,
    pub reply_to_text: String,
    pub mentions: Vec<String>,
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
    pub alert_investigation_id: String,
    pub reason: String,
    pub actor: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct PeerFindingEvent {
    #[serde(rename = "type")]
    pub event_type: String,
    pub investigation_id: String,
    pub peer_agent_id: String,
    pub peer_agent_type: String,
    pub text: String,
    pub labels: HashMap<String, String>,
    pub created_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct PeerAskEvent {
    #[serde(rename = "type")]
    pub event_type: String,
    pub ask_id: String,
    pub from_agent_id: String,
    pub from_agent_name: String,
    pub from_agent_type: String,
    pub investigation_id: String,
    pub question: String,
    pub expires_at: String,
    pub created_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct PeerReplyEvent {
    #[serde(rename = "type")]
    pub event_type: String,
    pub ask_id: String,
    pub investigation_id: String,
    pub reply: String,
    pub replied_by_agent_id: String,
    pub replied_by_agent_name: String,
    pub answered_at: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct CoordinationTaskEvent {
    #[serde(rename = "type")]
    pub event_type: String,
    pub chat_id: String,
    pub task_id: String,
    pub incident_number: i64,
    pub kind: String,
    pub goal: String,
    pub assignee_role: String,
    pub assignee_agent_id: String,
    pub input_context: Option<serde_json::Value>,
    pub parent_task_id: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct SummarizeIncidentEvent {
    pub incident_number: i64,
    pub chat_id: String,
    pub incident: Option<serde_json::Value>,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct AlertAutoResolvedEvent {
    pub investigation_id: String,
    pub fingerprint: String,
    pub alert_name: String,
}

#[derive(Clone, Debug, Default, Deserialize, Serialize)]
#[serde(default)]
pub struct IncidentCommsStaleEvent {
    pub incident_number: i64,
    pub trigger: String,
    pub reason: String,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct KnowledgeListResponse {
    #[serde(default)]
    pub items: Vec<KnowledgeNote>,
    #[serde(default)]
    pub total: i64,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct MemoryListResponse {
    #[serde(default)]
    pub items: Vec<Memory>,
    #[serde(default)]
    pub total: i64,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct PeerAskListResponse {
    #[serde(default)]
    pub items: Vec<PeerAsk>,
    #[serde(default)]
    pub total: i64,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct ServiceListResponse {
    #[serde(default)]
    pub items: Vec<Service>,
    #[serde(default)]
    pub total: i64,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct SendMessageResponse {
    #[serde(default)]
    pub status: String,
    #[serde(default)]
    pub message_id: String,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct CommandResponse {
    #[serde(default)]
    pub ok: bool,
    #[serde(default)]
    pub op: String,
    #[serde(default)]
    pub chat_id: String,
    #[serde(default)]
    pub investigation_id: String,
    #[serde(default)]
    pub incident_number: i64,
    #[serde(default)]
    pub error: String,
}
