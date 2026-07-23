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

export interface CorrelatedAlert {
  fingerprint?: string;
  alert_number?: number;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  status?: string;
  starts_at?: string;
  values?: Record<string, number>;
  generator_url?: string;
}

export interface InvestigationResult {
  status?: string;
  root_cause?: string;
  resolution?: string;
  summary?: string;
  evidence?: string[];
  recommended_actions?: string[];
  severity_assessment?: string;
  escalation_level?: string;
  raw_response?: string;
}

export interface InvestigationUpdate {
  id?: string;
  type?: string;
  message?: string;
  source?: string;
  internal?: boolean;
  edited?: boolean;
  user_id?: string;
  username?: string;
  mm_post_id?: string;
  slack_message_ts?: string;
  quoted_update_id?: string;
  mentions?: string[];
  created_at?: string;
}

export interface Investigation {
  id?: string;
  investigation_id?: string;
  investigation_number?: number;
  alerts?: CorrelatedAlert[];
  severity?: string;
  correlation_key?: string;
  status?: string;
  result?: InvestigationResult;
  mm_post_id?: string;
  mm_thread_id?: string;
  primary_thread_id?: string;
  slack_channel_id?: string;
  slack_thread_ts?: string;
  twilio_call_sid?: string;
  agent_id?: string;
  agent_name?: string;
  agent_type?: string;
  escalation_level?: string;
  updates?: InvestigationUpdate[];
  created_at?: string;
  updated_at?: string;
  completed_at?: string;
  started_at?: string;
  investigating_duration_ms?: number;
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
  similarity?: number;
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
  title?: string;
  description?: string;
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

export interface ConnectedEvent {
  agent_id?: string;
  client_id?: string;
}

export interface MessageEvent {
  id?: string;
  type?: string;
  chat_id?: string;
  kind?: string;
  text?: string;
  sender_type?: string;
  sender_id?: string;
  sender_name?: string;
  message_id?: string;
  created_at?: string;
}

export interface TypingEvent {
  type?: string;
  chat_id?: string;
  active?: boolean;
}

export interface InvestigationSignalEvent {
  investigation_id?: string;
  signal?: string;
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

export interface AlertListResponse {
  alerts?: Alert[];
  items?: Alert[];
  total?: number;
  limit?: number;
  skip?: number;
}

export interface InvestigationListResponse {
  investigations?: Investigation[];
  items?: Investigation[];
  total?: number;
  limit?: number;
  skip?: number;
}

export interface KnowledgeListResponse {
  notes?: KnowledgeNote[];
  items?: KnowledgeNote[];
  total?: number;
  limit?: number;
  skip?: number;
}

export interface MemoryListResponse {
  memories?: Memory[];
  total?: number;
}

export interface PeerAskListResponse {
  asks?: PeerAsk[];
  total?: number;
}

export interface SendMessageResponse {
  message_id?: string;
  chat_id?: string;
  created_at?: string;
}

export interface CommandResponse {
  ok?: boolean;
  op?: string;
  investigation_id?: string;
  error?: string;
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
  label_selectors?: unknown[];
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
