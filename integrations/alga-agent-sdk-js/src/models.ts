export interface AlertEvent {
  type?: string;
  timestamp?: string;
  source?: string;
  actor_user_id?: string;
  actor_username?: string;
  actor_display_name?: string;
}

export interface DeliveryTarget {
  provider?: string;
  channel?: string;
  channel_name?: string;
  post_id?: string;
}

export interface Alert {
  fingerprint?: string;
  alert_number?: number;
  status?: string;
  acknowledged?: boolean;
  silenced?: boolean;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  values?: Record<string, number>;
  starts_at?: string;
  ends_at?: string;
  generator_url?: string;
  events?: AlertEvent[];
  delivery_targets?: DeliveryTarget[];
  created_at?: string;
  updated_at?: string;
}

export interface KnowledgeNote {
  id?: string;
  kind?: string;
  title?: string;
  body_markdown?: string;
  tags?: string[];
  selectors?: Record<string, string>;
  source_investigation_id?: string;
  confidence?: number;
  expires_at?: string;
  author_type?: string;
  author_name?: string;
  created_at?: string;
  updated_at?: string;
}

export interface Memory {
  id?: string;
  content?: string;
  memory_type?: string;
  hash?: string;
  agent_id?: string;
  agent_name?: string;
  agent_type?: string;
  investigation_id?: string;
  correlation_key?: string;
  labels?: Record<string, string>;
  entities?: string[];
  metadata?: Record<string, unknown>;
  confidence?: number;
  access_count?: number;
  score?: number;
  expires_at?: string;
  created_at?: string;
  updated_at?: string;
}

export interface PeerAsk {
  id?: string;
  from_agent_id?: string;
  from_agent_name?: string;
  from_agent_type?: string;
  investigation_id?: string;
  to_agent_id?: string;
  to_agent_type?: string;
  question?: string;
  reply?: string;
  replied_by_agent_id?: string;
  replied_by_agent_name?: string;
  status?: string;
  expires_at?: string;
  created_at?: string;
  answered_at?: string;
}

export interface Service {
  id?: string;
  name?: string;
  description?: string;
  status?: string;
  labels?: Record<string, string>;
  team_id?: string;
  created_at?: string;
  updated_at?: string;
}

export interface Incident {
  id?: string;
  incident_number?: number;
  title?: string;
  description?: string;
  summary?: string;
  severity?: string;
  priority?: string;
  status?: string;
  commander_id?: string;
  service_id?: string;
  team_id?: string;
  sla_target_respond_at?: string;
  sla_target_resolve_at?: string;
  created_at?: string;
  updated_at?: string;
  resolved_at?: string;
  closed_at?: string;
}

// IncidentRole is a role assignment (commander, communications lead,
// responder, ...) attached to an incident.
export interface IncidentRole {
  role_type?: string;
  assignee_type?: string;
  agent_token_id?: string;
  agent_name?: string;
  user_id?: string;
  user_name?: string;
  status?: string;
}

// IncidentContext is the agent-facing incident read model.
export interface IncidentContext {
  incident?: Incident;
  roles?: IncidentRole[];
}

export interface OnCallEntry {
  schedule_id?: string;
  schedule_name?: string;
  user_id?: string;
  user_name?: string;
}

// SecretValue is the payload of GET /api/v1/agent/secrets/{secret_id}. The
// value is plaintext for immediate use; never persist or log it.
export interface SecretValue {
  secret_id?: string;
  name?: string;
  value?: string;
  fetched_at?: string;
}

export interface PlaybookStep {
  id?: string;
  step_number?: number;
  title?: string;
  description?: string;
  expected_duration?: string;
  command?: string;
}

export interface Playbook {
  id?: string;
  title?: string;
  kind?: string;
  summary?: string;
  service_id?: string;
  label_selectors?: Record<string, string>[];
  tags?: string[];
  steps?: PlaybookStep[];
  created_by?: string;
  created_at?: string;
  updated_at?: string;
}

export interface Capability {
  id?: string;
  name?: string;
  description?: string;
}

// --- Server-Sent Events ---

export interface ConnectedEvent {
  client_id?: string;
  agent_id?: string;
}

export interface MessageEvent {
  type?: string;
  chat_id?: string;
  text?: string;
  sender_id?: string;
  sender_name?: string;
  message_id?: string;
  // trigger distinguishes actionable deliveries ("dispatch"/"mention") from
  // passive ones ("observe").
  trigger?: string;
  reply_to_message_id?: string;
  reply_to_text?: string;
  mentions?: string[];
}

export interface TypingEvent {
  type?: string;
  chat_id?: string;
  active?: boolean;
}

// InvestigationSignalEvent is the payload of investigation_resume.
export interface InvestigationSignalEvent {
  investigation_id?: string;
  alert_investigation_id?: string;
  reason?: string;
  actor?: string;
}

export interface PeerFindingEvent {
  type?: string;
  investigation_id?: string;
  peer_agent_id?: string;
  peer_agent_type?: string;
  text?: string;
  labels?: Record<string, string>;
  created_at?: string;
}

export interface PeerAskEvent {
  type?: string;
  ask_id?: string;
  from_agent_id?: string;
  from_agent_name?: string;
  from_agent_type?: string;
  investigation_id?: string;
  question?: string;
  expires_at?: string;
  created_at?: string;
}

export interface PeerReplyEvent {
  type?: string;
  ask_id?: string;
  investigation_id?: string;
  reply?: string;
  replied_by_agent_id?: string;
  replied_by_agent_name?: string;
  answered_at?: string;
}

export interface AgentPresenceEvent {
  agent_id?: string;
  online?: boolean;
}

// SummarizeIncidentEvent asks a communicate-capable agent to produce an
// incident summary (reply with sendIncidentSummary).
export interface SummarizeIncidentEvent {
  incident_number?: number;
  chat_id?: string;
  incident?: Record<string, unknown>;
}

// AlertAutoResolvedEvent notifies the agent that an alert it was investigating
// auto-resolved (e.g. the alert cleared at the source).
export interface AlertAutoResolvedEvent {
  investigation_id?: string;
  fingerprint?: string;
  alert_name?: string;
}

// IncidentCommsStaleEvent nudges the incident-commander agent when incident
// communications have gone quiet past the SLA threshold.
export interface IncidentCommsStaleEvent {
  incident_number?: number;
  trigger?: string;
  reason?: string;
}

// --- List responses ---

export interface AlertListResponse {
  alerts?: Alert[];
  items?: Alert[];
  total?: number;
}

export interface KnowledgeListResponse {
  items?: KnowledgeNote[];
  notes?: KnowledgeNote[];
  total?: number;
}

export interface MemoryListResponse {
  items?: Memory[];
  memories?: Memory[];
  total?: number;
}

export interface PeerAskListResponse {
  items?: PeerAsk[];
  asks?: PeerAsk[];
  total?: number;
}

export interface ServiceListResponse {
  items?: Service[];
  total?: number;
}

export interface SendMessageResponse {
  status?: string;
  message_id?: string;
}

// CommandResponse mirrors the backend inv_tool outcome object.
export interface CommandResponse {
  ok?: boolean;
  op?: string;
  chat_id?: string;
  investigation_id?: string;
  incident_number?: number;
  error?: string;
}
