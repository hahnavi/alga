import {
  validate,
  alertListSchema,
  alertDetailResponseSchema,
  incidentListSchema,
  incidentDetailSchema,
} from "@/lib/validation";
import { z } from "zod";
import { normalizeOwnerThreadResponse, type OwnerThreadWireResponse } from "@/lib/ownerThread";
import { redirectToLogin } from "@/lib/authRedirect";

export type AlertEvent = {
  // `type` and `source` are free-form strings at the DB layer; the backend
  // emits values beyond the historical enum set (e.g. "acknowledged" in
  // addition to "acked", sources like "triage_auto_resolve"). Kept as
  // strings to mirror the runtime validation schema.
  type: string;
  timestamp: string;
  actor_username?: string;
  actor_display_name?: string;
  actor_user_id?: string;
  source?: string;
};

export type DeliveryTarget = {
  provider: string;
  channel: string;
  channel_name?: string;
  post_id: string;
};

export type AlertRecord = {
  fingerprint: string;
  alert_number?: number;
  status: "firing" | "resolved";
  acknowledged?: boolean;
  silenced?: boolean;
  delivery_targets?: DeliveryTarget[];
  labels: Record<string, string>;
  annotations: Record<string, string>;
  // Backend serializes nil as JSON `null` for these (no `omitempty`).
  values?: Record<string, unknown> | null;
  starts_at: string;
  ends_at?: string | null;
  generator_url?: string;
  events?: AlertEvent[];
  updated_at: string;
  created_at: string;
  deleted_at?: string | null;
  investigation?: AlertInvestigationListSummary;
};

type AlertDetailResponse = {
  alert: AlertRecord;
  alert_investigation?: AlertInvestigationRecord;
};

export type RelatedAlert = {
  fingerprint: string;
  alert_number?: number;
  status: string;
  labels: Record<string, string>;
  starts_at: string;
};

export type RelatedIncident = {
  incident_number: number;
  title: string;
  status: string;
  severity: string;
  priority: string;
  deleted_at?: string | null;
};

type AlertRelatedResponse = {
  related_alerts: RelatedAlert[];
  incident: RelatedIncident | null;
};

export type RouteCondition = {
  source: "labels" | "annotations" | "alert";
  field: string;
  operator:
    | "exact"
    | "contains"
    | "prefix"
    | "suffix"
    | "wildcard"
    | "regex"
    | "exists"
    | "not_exists";
  value?: string;
};

export type RouteTarget = {
  provider?: "mattermost" | "slack";
  channel: string;
};

export type RouteConfig = {
  match_mode?: "all" | "any";
  conditions?: RouteCondition[];
  targets?: RouteTarget[];
  silenced?: boolean;
};

export type UserInfo = {
  id: string;
  email: string;
  full_name?: string;
  phone?: string;
  phone_country?: string;
  role: string;
  created_at: string;
  last_login_at?: string;
  slack_linked?: boolean;
  slack_display_name?: string;
  google_linked?: boolean;
  /**
   * Permission strings the user holds, populated from
   * `apps/backend/rbac/roles.go` by both `/auth/login` and `/auth/me`.
   * When missing, `stores/auth.ts` denies every `hasPermission(...)`
   * call (safe default).
   */
  permissions?: string[];
};

export type WebhookTokenRow = {
  id: string;
  name: string;
  token: string;
  created_at: string;
  last_used?: string | null;
  expires_at?: string;
  expired?: boolean;
};

export type AgentType = "alga" | "hermes" | "openclaw" | "other";

// One audit-log row as served by GET /api/v1/audit-events (admin/operator,
// audit:read gated).
export type AuditEventRow = {
  id: string;
  timestamp: string;
  event: string;
  user_id?: string;
  username: string;
  ip?: string;
  user_agent?: string;
  success: boolean;
  details?: Record<string, unknown>;
  request_id?: string;
  entity_type?: string;
  entity_id?: string;
};

// Slim, list-friendly view of the current alert investigation that ships
// inline with alert list/detail payloads so the assigned actor (agent),
// status, and promotion link can be rendered without a separate round-trip.
type AlertInvestigationListSummary = {
  alert_investigation_id: string;
  status: string;
  agent_id?: string;
  agent_name?: string;
  agent_type?: AgentType;
  assignee_type?: "agent" | "user";
  promoted_incident_id?: string;
  promoted_incident_number?: number;
};

export type AgentCapability = "investigate" | "communicate" | "command" | "secrets";

export type AgentTokenRow = {
  id: string;
  name: string;
  agent_type?: AgentType;
  token: string;
  created_at: string;
  last_used?: string | null;
  expires_at?: string;
  expired?: boolean;
  enabled?: boolean;
  online?: boolean;
  scope?: "all" | "labels";
  label_selectors?: RouteCondition[];
  capabilities?: AgentCapability[];
};

export type PATRow = {
  id: string;
  name: string;
  permissions: string[];
  expires_at?: string;
  last_used_at?: string;
  created_at: string;
  revoked: boolean;
  user_id?: string;
};

export type AgentDMMessageRow = {
  id: string;
  agent_token_id: string;
  chat_id: string;
  role: "user" | "agent";
  body: string;
  user_id?: string;
  username?: string;
  edited?: boolean;
  created_at: string;
  updated_at: string;
};

export type IntegrationInfo = {
  mattermost: {
    enabled: boolean;
    /** When false, credentials may still be stored but Mattermost delivery is off. */
    provider_enabled: boolean;
    url: string;
    base_url: string;
    secret_configured: boolean;
    team: string;
    locked: boolean;
    default_channel: string;
  };
  slack: {
    enabled: boolean;
    provider_enabled: boolean;
    token_configured: boolean;
    signing_secret_configured: boolean;
    client_id_configured: boolean;
    workspace_name: string | null;
    workspace_id: string | null;
    locked: boolean;
    default_channel: string;
  };
  twilio: {
    enabled: boolean;
    provider_enabled: boolean;
    active: boolean;
    account_sid_configured: boolean;
    auth_token_configured: boolean;
    from_number: string;
    locked: boolean;
  };
  telnyx: {
    enabled: boolean;
    provider_enabled: boolean;
    active: boolean;
    api_key_configured: boolean;
    connection_id: string;
    from_number: string;
    public_key_configured: boolean;
    tts_voice: string;
    tts_language: string;
    tts_api_key_ref: string;
    locked: boolean;
  };
  google_meet: {
    enabled: boolean;
    auto_create: boolean;
  };
  voice_provider: "twilio" | "telnyx";
  voice_provider_locked: boolean;
};

type OwnerThreadMessageType =
  | "progress"
  | "finding"
  | "action"
  | "resolution"
  | "comment"
  | "tool_call";
export type OwnerThreadMessageSource = "agent" | "user" | "mattermost" | "slack" | "system";

export type OwnerThreadMessage = {
  id: string;
  type: OwnerThreadMessageType;
  source: OwnerThreadMessageSource;
  message: string;
  internal?: boolean;
  edited?: boolean;
  user_id?: string;
  username?: string;
  agent_type?: AgentType;
  mm_post_id?: string;
  slack_message_ts?: string;
  reply_to_message_id?: string;
  mentions?: string[];
  created_at: string;
  updated_at: string;
};

export type OwnerThread = {
  id?: string;
  thread_id?: string;
  owner_type?: "alert" | "incident_inv";
  owner_id?: string;
  messages: OwnerThreadMessage[];
  total?: number;
  created_at?: string;
  updated_at?: string;
};

type CorrelatedAlert = {
  fingerprint: string;
  alert_number?: number;
  labels: Record<string, string>;
  annotations: Record<string, string>;
  status: string;
  starts_at: string;
  values?: Record<string, number>;
  generator_url?: string;
};

type AlertInvestigationSummary = {
  status?: string;
  root_cause?: string;
  resolution?: string;
  summary?: string;
  findings?: string[];
  evidence?: string[];
  recommended_actions?: string[];
  severity_assessment?: string;
  escalation_level?: string;
  raw_response?: string;
};

type InvestigationFinding = {
  title: string;
  severity?: string;
  evidence?: string[];
};

type EvidenceItem = {
  source: string;
  type: string;
  content: string;
  timestamp?: string;
};

export type InvestigationUpdate = {
  id: string;
  type: string;
  message: string;
  source: string;
  internal?: boolean;
  edited?: boolean;
  user_id?: string;
  username?: string;
  mm_post_id?: string;
  slack_message_ts?: string;
  quoted_update_id?: string;
  mentions?: string[];
  created_at: string;
};

type AlertInvestigationLifecycleEvent = {
  id: string;
  event_type: string;
  reason?: string;
  actor_type?: "agent" | "user" | "system" | "grafana" | string;
  actor_id?: string;
  actor_name?: string;
  agent_id?: string;
  agent_name?: string;
  agent_type?: AgentType;
  metadata?: Record<string, unknown>;
  created_at: string;
};

export type AlertInvestigationRecord = {
  id: string;
  alert_investigation_id: string;
  alert_investigation_number?: number;
  primary_alert_fingerprint?: string;
  primary_alert_number?: number;
  alerts: CorrelatedAlert[];
  correlation_key: string;
  status: string;
  agent_id?: string;
  agent_name?: string;
  agent_type?: AgentType;
  assignee_type?: "agent" | "user";
  assignee_id?: string;
  summary?: AlertInvestigationSummary;
  findings?: InvestigationFinding[];
  evidence?: EvidenceItem[];
  updates?: InvestigationUpdate[];
  completed_reason?: string;
  completed_by_type?: "agent" | "user" | "system" | "grafana" | string;
  completed_by_id?: string;
  completed_by_name?: string;
  events?: AlertInvestigationLifecycleEvent[];
  created_at: string;
  updated_at: string;
  completed_at?: string;
  started_at?: string;
  investigating_duration_ms: number;
  promoted_incident_id?: string;
  promoted_incident_investigation_id?: string;
};

export type KnowledgeNote = {
  id: string;
  kind: "runbook" | "known_issue" | "service_owner" | "fact" | string;
  title: string;
  body_markdown: string;
  tags?: string[];
  selectors?: RouteCondition[];
  author_id?: string;
  author_type: "user" | "agent";
  author_name?: string;
  source_investigation_id?: string;
  confidence?: number;
  expires_at?: string;
  created_at: string;
  updated_at: string;
};

export type KnowledgeNoteInput = {
  kind: string;
  title: string;
  body_markdown: string;
  tags?: string[];
  selectors?: RouteCondition[];
  confidence?: number;
  expires_at?: string;
};

export type NotificationRecord = {
  id: string;
  user_id: string;
  type: string;
  title: string;
  message: string;
  read: boolean;
  resource_type: string;
  resource_id: string;
  triggered_by_user_id?: string;
  triggered_by_display_name?: string;
  created_at: string;
};

type DashboardAlertStats = {
  total: number;
  firing: number;
  resolved: number;
  unacknowledged: number;
};

type SeverityBucket = {
  severity: string;
  count: number;
};

type DailyAlertCount = {
  date: string;
  created: number;
  resolved: number;
};

type DashboardInvestigation = {
  total: number;
  pending: number;
  investigating: number;
  complete: number;
  failed: number;
  cancelled: number;
  timed_out: number;
  completion_rate: number;
};

type TopAlertItem = {
  alert_name: string;
  count: number;
  severity: string;
  status: string;
  labels?: Record<string, string>;
};

type RecentInvestigationItem = {
  investigation_id: string;
  investigation_number?: number;
  alert_investigation_id?: string;
  alert_investigation_number?: number;
  status: string;
  severity: string;
  alert_name: string;
  agent_name?: string;
  correlation_key: string;
  created_at: string;
  summary?: string;
};

export type DashboardStats = {
  alerts: DashboardAlertStats;
  alerts_by_severity: SeverityBucket[];
  alert_trend: DailyAlertCount[];
  investigations: DashboardInvestigation;
  top_alerts_24h: TopAlertItem[];
  recent_investigations: RecentInvestigationItem[];
  active_investigations: RecentInvestigationItem[];
  incidents: {
    total: number;
    active: number;
    mitigated: number;
    resolved: number;
    by_severity: Record<string, number>;
    by_priority?: Record<string, number>;
  };
  active_incidents: {
    incident_number: number;
    title: string;
    severity: string;
    priority: string;
    status: string;
    service_name?: string;
    commander_name?: string;
    created_at: string;
  }[];
  services: {
    total: number;
    by_status: Record<string, number>;
  };
  sla_stats: {
    response_breaches: number;
    resolve_breaches: number;
    compliance_pct: number;
  };
};

export type IncidentMetrics = {
  mtta_minutes: number;
  mttr_minutes: number;
  mttm_minutes: number;
  total_created: number;
  total_resolved: number;
  by_severity: Record<string, { count: number; mtta_minutes: number; mttr_minutes: number }>;
  by_priority?: Record<string, { count: number; mtta_minutes: number; mttr_minutes: number }>;
  by_service: Record<string, { count: number; mtta_minutes: number; mttr_minutes: number }>;
  sla_compliance: {
    response_sla_compliance_pct: number;
    resolve_sla_compliance_pct: number;
    response_breaches: number;
    resolve_breaches: number;
    total_with_sla: number;
  };
  trend: {
    date: string;
    created: number;
    resolved: number;
    mtta_minutes: number;
    mttr_minutes: number;
  }[];
};

export type DailySummaryResponse = {
  summary: string;
  generated_at: string;
  period: string;
  available: boolean;
  failed?: boolean;
  error?: string;
};

export type AgentMemoryRecord = {
  id: string;
  content: string;
  memory_type: "fact" | "pattern" | "procedure" | string;
  hash: string;
  agent_id?: string;
  agent_name?: string;
  agent_type?: string;
  investigation_id?: string;
  correlation_key?: string;
  labels?: Record<string, string>;
  entities?: string[];
  metadata?: Record<string, unknown>;
  confidence?: number;
  access_count: number;
  expires_at?: string;
  created_at: string;
  updated_at: string;
};

export type AgentMemoryInput = {
  content: string;
  memory_type?: string;
  investigation_id?: string;
  correlation_key?: string;
  labels?: Record<string, string>;
  confidence?: number;
  expires_at?: string;
};

export type SystemConfigValues = {
  correlation_window: string;
  correlation_cooldown_ttl: string;
  investigation_timeout: string;
  max_concurrent_investigations: number;
  agent_presence_ttl: string;
  agent_disconnect_grace: string;
  scheduler_leader_ttl: string;
  session_expiry_hours: number;
  log_level: string;
  environment?: string;
  slack_incident_channels_enabled: boolean;
  slack_incident_channel_visibility: string;
  slack_incident_channel_trigger_status: string;
  slack_incident_channel_archive_on_close: boolean;
  incident_summary_enabled: boolean;
  incident_summary_interval: string;
  incident_summary_intervals: Record<string, string>;

  // Authentication — Google OAuth login. The client secret is never returned;
  // google_client_secret_set indicates whether one is configured.
  google_oauth_enabled: boolean;
  google_client_id: string;
  google_client_secret_set: boolean;
  google_oauth_redirect_url: string;

  updated_at?: string;
};

export type MaintenanceWindowRecord = {
  id: string;
  name: string;
  start_time: string;
  end_time: string;
  label_matchers: Record<string, string>;
  enabled: boolean;
  created_at: string;
  updated_at: string;
};

type HeartbeatStatus = "healthy" | "expired";

export type StatusPageVisibility = "internal" | "public";

export type ComponentStatus =
  | "operational"
  | "degraded"
  | "partial_outage"
  | "major_outage"
  | "maintenance";

export type StatusPageRecord = {
  id: string;
  name: string;
  slug: string;
  description: string;
  visibility: StatusPageVisibility;
  enabled: boolean;
  owner_team_id?: string;
  created_at: string;
  updated_at: string;
};

export type StatusPageComponentRecord = {
  id: string;
  status_page_id: string;
  name: string;
  description: string;
  service_id?: string;
  display_order: number;
  status: ComponentStatus;
  created_at: string;
  updated_at: string;
};

export type StatusPageViewPage = {
  name: string;
  slug: string;
  description?: string;
};

export type StatusPageViewComponent = {
  name: string;
  description?: string;
  status: ComponentStatus;
  display_order: number;
};

export type StatusPageViewIncident = {
  title: string;
  status: string;
  severity: string;
  started_at?: string;
};

// Allow-listed payload of GET /status-pages/slug/{slug}: no internal
// ids, owner team, Slack/war-room linkage or SLA fields.
export type StatusPageView = {
  page: StatusPageViewPage;
  overall_status: ComponentStatus;
  components: StatusPageViewComponent[];
  incidents: StatusPageViewIncident[];
};

export type OIDCProviderRecord = {
  id: string;
  name: string;
  issuer: string;
  client_id: string;
  client_secret_configured: boolean;
  scopes: string[];
  enabled: boolean;
  created_at: string;
  updated_at: string;
};

export type OIDCProviderPublic = {
  id: string;
  name: string;
};

type CredentialProviderType =
  | "internal"
  | "hashicorp_vault"
  | "aws_secrets_manager"
  | "gcp_secret_manager"
  | "azure_key_vault"
  | string;

export type CredentialProviderRecord = {
  id: string;
  name: string;
  type: CredentialProviderType;
  enabled: boolean;
  system: boolean;
  config_configured: boolean;
  provider_type_name?: string;
  created_at: string;
  updated_at: string;
};

export type SharedSecretRecord = {
  id: string;
  provider_id: string;
  name: string;
  secret_id: string;
  description: string;
  remote_ref: string;
  value_configured: boolean;
  allowed_agent_ids?: string[];
  created_at: string;
  updated_at: string;
  provider?: CredentialProviderRecord;
};

export const CREDENTIAL_PROVIDER_TYPES: {
  value: CredentialProviderType;
  label: string;
  external: boolean;
}[] = [
  { value: "internal", label: "Alga Internal", external: false },
  { value: "hashicorp_vault", label: "HashiCorp Vault", external: true },
  { value: "aws_secrets_manager", label: "AWS Secrets Manager", external: true },
  { value: "gcp_secret_manager", label: "GCP Secret Manager", external: true },
  { value: "azure_key_vault", label: "Azure Key Vault", external: true },
];

export type HeartbeatRecord = {
  id: string;
  name: string;
  description: string;
  interval_seconds: number;
  grace_seconds: number;
  enabled: boolean;
  owner_team_id?: string;
  status: HeartbeatStatus;
  severity: string;
  labels: Record<string, string>;
  ping_token?: string;
  last_ping_at?: string;
  expires_at?: string;
  last_breach_at?: string;
  created_by: string;
  created_at: string;
  updated_at: string;
};

export type IncidentStatus =
  | "detected"
  | "triaging"
  | "active"
  | "mitigated"
  | "resolved"
  | "closed"
  | "cancelled";
export type Severity = "critical" | "high" | "warning" | "info";
export type ImpactLevel = "high" | "medium" | "low";
export type IncidentPriority = "P1" | "P2" | "P3" | "P4" | "P5";

export type ICSRoleType =
  | "incident_commander"
  | "deputy"
  | "communications_lead"
  | "customer_liaison"
  | "internal_liaison"
  | "operations_lead"
  | "planning_lead"
  | "technical_lead"
  | "responder"
  | "subject_matter_expert"
  | "scribe";
type ICSRoleStatus = "active" | "ended";
type ICSEndReason = "replaced" | "incident_resolved" | "assigned" | "agent_offline";

export type ICSRoleRecord = {
  id: string;
  incident_number: number;
  role_type: ICSRoleType;
  assignee_type: "user" | "agent";
  user_id?: string;
  user_name?: string;
  user_email?: string;
  agent_token_id?: string;
  agent_name?: string;
  agent_type?: string;
  agent_revoked?: boolean;
  parent_assignment_id?: string;
  scope_description?: string;
  status: ICSRoleStatus;
  ended_reason?: ICSEndReason;
  started_at: string;
  ended_at?: string;
};

export type DocumentSection =
  | "impact_assessment"
  | "current_status"
  | "actions_taken"
  | "open_questions"
  | "resources"
  | "timeline_summary"
  | "root_cause"
  | "resolution";

export type IncidentDocumentSection = {
  id: string;
  incident_number: number;
  section: DocumentSection;
  content: string;
  version: number;
  updated_by?: string;
  updated_at: string;
};

export type IncidentRecord = {
  id: string;
  incident_number: number;
  title: string;
  description: string;
  summary?: string;
  status: IncidentStatus;
  severity: Severity;
  impact_level: ImpactLevel;
  priority: IncidentPriority;
  incident_type: string;
  commander_id?: string;
  communicator_id?: string;
  on_call_responder_id?: string;
  service_id?: string;
  conference_url?: string;
  slack_channel_id?: string;
  slack_channel_name?: string;
  slack_channel_archived: boolean;
  tags?: string[];
  custom_fields?: Record<string, unknown>;
  sla_target_respond_at?: string;
  sla_target_resolve_at?: string;
  sla_acknowledged_at?: string;
  sla_resolved_at?: string;
  started_at?: string;
  mitigated_at?: string;
  resolved_at?: string;
  closed_at?: string;
  created_at: string;
  updated_at: string;
  timeline?: IncidentTimelineRecord[];
  war_room_channel_id?: string;
  war_room_channel_provider?: "slack" | "mattermost";
  google_meet_space_name?: string;
  ics_roles?: ICSRoleRecord[];
  document_sections?: IncidentDocumentSection[];
  deleted_at?: string | null;
};

export type IncidentCascadeSummary = {
  resolved: number;
  skipped: number;
  failed: number;
};

export type IncidentResolveResponse = {
  incident: IncidentRecord;
  cascade: IncidentCascadeSummary;
};

export type IncidentTimelineRecord = {
  id: string;
  incident_number: number;
  event_type: string;
  actor_id?: string;
  actor_type: string;
  message: string;
  metadata: Record<string, unknown>;
  created_at: string;
};

type PaginatedIncidents = {
  items: IncidentRecord[];
  total: number;
};

type IncidentCoordinationMessageKind =
  | "chat"
  | "system"
  | "decision"
  | "action"
  | "agent_reply"
  | "investigation_summary"
  | "status_update";

export type StatusUpdateStatusLevel = "investigating" | "identified" | "monitoring" | "resolved";

type StatusUpdateCreateRequest = {
  status_level: StatusUpdateStatusLevel;
  body: string;
  internal?: boolean;
};

type IncidentCoordinationActorType = "user" | "agent" | "system" | "external";

type IncidentCoordinationSource = "alga" | "slack" | "mattermost" | "agent" | "system";

export type IncidentCoordinationMessage = {
  id: string;
  incident_number: number;
  kind: IncidentCoordinationMessageKind;
  actor_type: IncidentCoordinationActorType;
  actor_id?: string;
  actor_display_name?: string;
  body: string;
  internal: boolean;
  source: IncidentCoordinationSource;
  slack_channel_id?: string;
  slack_message_ts?: string;
  slack_thread_ts?: string;
  provider_message_id?: string;
  linked_investigation_id?: string;
  parent_message_id?: string;
  metadata?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};

export type ServiceRecord = {
  id: string;
  name: string;
  display_name: string;
  description: string;
  owner_team_id?: string;
  label_matchers: Record<string, unknown>[];
  sla_response_minutes: number;
  sla_resolve_minutes: number;
  status: string;
  dependencies?: ServiceDependencyRecord[];
  dependents?: ServiceDependencyRecord[];
  active_incident_count?: number;
  created_at: string;
  updated_at: string;
};

export type ServiceDependencyRecord = {
  id: string;
  service_id: string;
  dependent_on_service_id: string;
  dependency_type: string;
  created_at: string;
  dependent_on_service_name?: string;
};

export type TeamRecord = {
  id: string;
  name: string;
  description: string;
  created_at: string;
  updated_at: string;
  members?: TeamMemberRecord[];
};

export type TeamMemberRecord = {
  id: string;
  team_id: string;
  user_id: string;
  role: "lead" | "member";
  user_name?: string;
  user_email?: string;
};

export type EscalationPolicyRecord = {
  id: string;
  name: string;
  description: string;
  repeat_count: number;
  created_at: string;
  updated_at: string;
  levels?: EscalationLevelRecord[];
};

type EscalationLevelRecord = {
  level_number: number;
  delay_minutes: number;
  notify_channels: string[];
  targets?: EscalationTargetRecord[];
};

type EscalationTargetRecord = {
  target_type: "user" | "team";
  target_user_id?: string;
  target_team_id?: string;
};

export type OnCallScheduleRecord = {
  id: string;
  // Display name is derived dynamically from the team (team_name). Schedules
  // are auto-created when a team is created and carry no name of their own.
  team_id?: string;
  team_name: string;
  created_at: string;
  updated_at: string;
  layers?: ScheduleLayerRecord[];
};

export type ScheduleLayerRecord = {
  id: string;
  schedule_id: string;
  name: string;
  rotation_type: "hourly" | "daily" | "weekly" | "monthly";
  rotation_interval: number;
  start_date: string;
  end_date?: string;
  timezone: string;
  start_time: string;
  end_time?: string;
  days_of_week: string[];
  priority: number;
  user_ids: string[];
};

export type ScheduleShift = {
  user_id: string;
  user_display_name?: string;
  start: string;
  end: string;
  source: "rotation" | "override";
};

export type ScheduleLayerInput = {
  name: string;
  rotation_type: "hourly" | "daily" | "weekly" | "monthly";
  rotation_interval?: number;
  start_date: string;
  end_date?: string;
  timezone?: string;
  start_time?: string;
  end_time?: string;
  days_of_week?: string[];
  priority?: number;
  user_ids?: string[];
};

export type ScheduleOverrideRecord = {
  id: string;
  schedule_id: string;
  user_id: string;
  start_at: string;
  end_at: string;
  created_by?: string;
};

export type OnCallCurrent = {
  schedule_id: string;
  schedule_name: string;
  user_id: string;
  user_display_name?: string;
  until?: string;
};

export type OnCallSelfEntry = {
  schedule_id: string;
  schedule_name: string;
  layer_name?: string;
  user_id?: string;
  start_at?: string;
  end_at?: string;
};

export type OnCallSelfView = {
  current: OnCallSelfEntry[];
  pending: OnCallSelfEntry[];
};

export type HandoffRecord = {
  id: string;
  schedule_id: string;
  outgoing_user_id: string | null;
  incoming_user_id: string | null;
  handoff_at: string;
  status: "pending" | "acknowledged";
  outgoing_notes: string;
  incoming_notes: string;
  incoming_acknowledged_at: string | null;
  incident_summary: string;
  created_at: string;
  updated_at: string;
};

export type PlaybookStepRecord = {
  id: string;
  playbook_id: string;
  step_number: number;
  title: string;
  description: string;
  expected_duration: string;
  command: string;
  created_at: string;
  updated_at: string;
};

export type PlaybookRecord = {
  id: string;
  title: string;
  kind: "procedure" | "mitigation";
  summary: string;
  service_id: string | null;
  label_selectors: Array<{ key: string; op: string; value: string }>;
  tags: string[];
  created_by: string;
  created_at: string;
  updated_at: string;
  steps?: PlaybookStepRecord[];
};

type PagerLoadShift = {
  user_id: string;
  user_display_name: string;
  shift_start: string;
  shift_end: string;
  alerts_received: number;
  alerts_acknowledged: number;
  alerts_resolved: number;
  alerts_missed: number;
  avg_ack_time_seconds: number;
};

export type PagerLoadData = {
  shifts: PagerLoadShift[];
  summary: {
    total_shifts: number;
    avg_alerts_per_shift?: number;
    avg_ack_rate: number;
    avg_ack_time_seconds: number;
  };
};

export type PostMortemTimelineEntry = {
  timestamp: string;
  event: string;
  description: string;
  actor: string;
};

export type PostMortemRecord = {
  id: string;
  incident_number: number;
  title: string;
  status: "draft" | "in_review" | "approved" | "published";
  summary: string;
  timeline?: PostMortemTimelineEntry[];
  root_cause: string;
  contributing_factors?: string[];
  impact: string;
  lessons_learned: string;
  what_went_well: string;
  what_went_wrong: string;
  blameless_confirmed: boolean;
  blameless_notes: string;
  approved_by_id?: string;
  published_at?: string;
  created_at: string;
  updated_at: string;
  action_items?: ActionItemRecord[];
  incident_title?: string;
  incident_severity?: string;
};

export type ActionItemRecord = {
  id: string;
  post_mortem_id: string;
  description: string;
  assignee_id?: string;
  type: "prevent" | "mitigate" | "detect" | "investigate";
  assignee_name?: string;
  status: "open" | "in_progress" | "completed" | "cancelled";
  priority: "high" | "medium" | "low";
  due_date?: string;
  created_at: string;
  updated_at: string;
};

export type NotificationPreferences = {
  rules: NotificationPreferenceRule[];
  default_channel?: string;
};

export type NotificationPreferenceRule = {
  notification_type: string;
  channels: string[];
  start_time?: string;
  end_time?: string;
  timezone?: string;
  severity_filter?: string[];
  enabled: boolean;
};

type StatusResponse = { status: string };
type ItemsResponse<T> = { items: T[]; total?: number };
type DataResponse<T> = { data: T[] };
type ListPostMortemsParams = Record<string, string | number | boolean | undefined>;

function unwrapItems<T>(p: Promise<ItemsResponse<T>>): Promise<T[]> {
  return p.then((r) => r.items ?? []);
}
function unwrapData<T>(p: Promise<DataResponse<T>>): Promise<T[]> {
  return p.then((r) => r.data ?? []);
}

const base = import.meta.env.VITE_API_BASE_URL ?? "";

export function e(segment: string | number): string {
  return encodeURIComponent(String(segment));
}

export function buildQuery(params: Record<string, string | number | boolean | undefined>): string {
  const sp = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v != null && v !== "") sp.set(k, String(v));
  }
  const q = sp.toString();
  return q ? `?${q}` : "";
}

// safe401Paths is the ALLOWLIST of paths that do NOT trigger a login redirect on 401.
// (Inverse logic: paths listed here are exempt; everything else will redirect to /login on 401.)
// Public callback routes (e.g. /api/v1/auth/oidc/..., /api/v1/auth/reset-password) MUST be added
// here, or an expected 401 from those routes will cause a redirect-loop for unauthenticated users.
// Each entry is either a string (exact match, also matches `entry + "?"` for query-string
// variants) or `{ prefix }` (matches any path starting with that prefix). OIDC authorize/callback
// carry a provider {id} segment, so they use the prefix form. If you add a new public callback,
// mirror it here and keep this list in sync with apps/backend/api/http.go.
type Safe401Entry = string | { prefix: string };

const safe401Paths: Safe401Entry[] = [
  "/api/v1/auth/login",
  "/api/v1/auth/logout",
  "/api/v1/auth/refresh",
  "/api/v1/auth/me",
  "/api/v1/auth/change-email",
  "/api/v1/auth/change-password",
  "/api/v1/auth/profile",
  "/api/v1/auth/forgot-password",
  "/api/v1/auth/reset-password",
  "/api/v1/auth/google",
  "/api/v1/auth/google/enabled",
  "/api/v1/auth/google/callback",
  "/api/v1/auth/slack",
  "/api/v1/auth/slack/enabled",
  "/api/v1/auth/slack/callback",
  { prefix: "/api/v1/auth/oidc/" },
];

function isSafe401Path(path: string): boolean {
  const [pathname] = path.split("?");
  return safe401Paths.some((entry) => {
    if (typeof entry === "string") return pathname === entry;
    return pathname === entry.prefix || pathname.startsWith(entry.prefix);
  });
}

function readCSRFTokenFromCookie(): string | null {
  const match = document.cookie.match(/alga_csrf=([^;]+)/);
  return match ? match[1] : null;
}

const MAX_ERROR_MESSAGE_LENGTH = 500;

function sanitizeErrorMessage(message: string): string {
  // eslint-disable-next-line no-control-regex -- intentional: strip C0 controls and DEL
  const cleaned = message.replace(/[\u0000-\u001f\u007f]+/g, " ").trim();
  if (cleaned.length <= MAX_ERROR_MESSAGE_LENGTH) return cleaned;
  return cleaned.slice(0, MAX_ERROR_MESSAGE_LENGTH - 1) + "…";
}

// Store CSRF token after login
let csrfToken: string | null = null;

// Stable error codes returned in the API error envelope
// ({error:{code,message,details}}). Mirrors the backend ErrorCode set.
export type ApiErrorCode =
  | "validation_failed"
  | "unauthorized"
  | "forbidden"
  | "not_found"
  | "conflict"
  | "rate_limited"
  | "internal";

// Field-level validation detail attached to a validation_failed error.
export type ApiErrorDetail = {
  field: string;
  message: string;
};

// Typed error thrown for every non-2xx API response. Surfaces the stable
// error code, human-readable message, and (for validation failures) the
// per-field details returned by the backend envelope.
export class ApiError extends Error {
  status: number;
  code: ApiErrorCode;
  details: ApiErrorDetail[];
  constructor(status: number, code: ApiErrorCode, message: string, details: ApiErrorDetail[] = []) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

async function request<T>(
  path: string,
  init?: RequestInit,
  schema?: z.ZodType<unknown>,
): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
  };
  if (init?.headers) {
    const h = init.headers;
    if (h instanceof Headers) {
      h.forEach((v, k) => {
        headers[k] = v;
      });
    } else if (Array.isArray(h)) {
      for (const [k, v] of h) headers[k] = v;
    } else {
      Object.assign(headers, h);
    }
  }

  // Add CSRF token for state-changing operations
  if (init?.method && !["GET", "HEAD", "OPTIONS"].includes(init.method.toUpperCase())) {
    const token = csrfToken || readCSRFTokenFromCookie();
    if (token) {
      headers["X-CSRF-Token"] = token;
    }
  }

  const { headers: _initHeaders, ...restInit } = init ?? {};
  const res = await fetch(`${base}${path}`, {
    headers,
    credentials: "same-origin",
    ...restInit,
  });

  if (res.status === 401 && !isSafe401Path(path)) {
    redirectToLogin();
    throw new ApiError(res.status, "unauthorized", "Unauthorized");
  }

  if (!res.ok) {
    const text = await res.text();
    let message = `Request failed: ${res.status}`;
    let code: ApiErrorCode = "internal";
    let details: ApiErrorDetail[] = [];
    try {
      const data: unknown = JSON.parse(text);
      const errField = (data as { error?: unknown })?.error;
      if (typeof errField === "string") {
        message = errField;
      } else if (errField && typeof errField === "object") {
        const ef = errField as {
          code?: string;
          message?: string;
          details?: unknown;
        };
        if (typeof ef.message === "string") message = ef.message;
        if (typeof ef.code === "string") code = ef.code as ApiErrorCode;
        if (Array.isArray(ef.details)) {
          details = ef.details as ApiErrorDetail[];
        }
      }
    } catch {
      // keep defaults
    }
    throw new ApiError(res.status, code, sanitizeErrorMessage(message), details);
  }

  if (res.status === 204) {
    return undefined as T;
  }
  const text = await res.text();
  if (!text) {
    return undefined as T;
  }
  const parsed: unknown = JSON.parse(text);
  // Unwrap the stable {data: ...} envelope for single resources and lists.
  // Status/aggregate responses without a `data` key are returned as-is.
  let value: unknown = parsed;
  if (
    parsed !== null &&
    typeof parsed === "object" &&
    "data" in parsed &&
    (parsed as { data?: unknown }).data !== undefined
  ) {
    value = (parsed as { data: unknown }).data;
  }
  // When a schema is supplied, check the (unwrapped) payload at the boundary
  // so a malformed response fails here instead of crashing deep in a component.
  // The result is cast back to the hand-written TS type `T` that callers rely
  // on; the schema is the runtime source of truth, `T` the compile-time one.
  if (schema) {
    value = validate(schema, value);
  }
  return value as T;
}

type LoginResponse = UserInfo & { csrf_token?: string };

type SetupResponse = UserInfo & { csrf_token?: string };

export const api = {
  // CSRF Token management
  setCSRFToken(token: string | null) {
    csrfToken = token;
  },

  getCSRFToken(): string | null {
    return csrfToken || readCSRFTokenFromCookie();
  },

  // Auth
  async login(email: string, password: string) {
    const res = await request<LoginResponse>("/api/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ email, password }),
    });
    if (res.csrf_token) {
      csrfToken = res.csrf_token;
    }
    return res;
  },

  logout() {
    return request<StatusResponse>("/api/v1/auth/logout", { method: "POST" });
  },

  getCurrentUser() {
    return request<UserInfo>("/api/v1/auth/me");
  },

  refreshSession() {
    return request<StatusResponse>("/api/v1/auth/refresh", {
      method: "POST",
      headers: { "X-Requested-With": "XMLHttpRequest" },
    });
  },

  changePassword(currentPassword: string, newPassword: string) {
    return request<StatusResponse>("/api/v1/auth/change-password", {
      method: "POST",
      body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
    });
  },

  changeEmail(password: string, email: string) {
    return request<StatusResponse>("/api/v1/auth/change-email", {
      method: "POST",
      body: JSON.stringify({ password, email }),
    });
  },

  updateProfile(fullName: string, phone: string, phoneCountry: string) {
    return request<StatusResponse>("/api/v1/auth/profile", {
      method: "POST",
      body: JSON.stringify({ full_name: fullName, phone, phone_country: phoneCountry }),
    });
  },

  forgotPassword(email: string) {
    return request<StatusResponse>("/api/v1/auth/forgot-password", {
      method: "POST",
      body: JSON.stringify({ email }),
    });
  },

  resetPassword(token: string, newPassword: string) {
    return request<StatusResponse>("/api/v1/auth/reset-password", {
      method: "POST",
      body: JSON.stringify({ token, new_password: newPassword }),
    });
  },

  isGoogleAuthEnabled() {
    return request<{ enabled: boolean }>("/api/v1/auth/google/enabled");
  },

  isSlackAuthEnabled() {
    return request<{ enabled: boolean }>("/api/v1/auth/slack/enabled");
  },

  // Setup
  getSetupStatus() {
    return request<{ needs_setup: boolean }>("/api/v1/setup/status");
  },
  setup(body: { email: string; password: string; full_name?: string }) {
    return request<SetupResponse>("/api/v1/setup", {
      method: "POST",
      body: JSON.stringify(body),
    });
  },

  // Users
  getUsers() {
    return request<UserInfo[]>("/api/v1/users");
  },
  createUser(email: string, password: string, role: string, fullName?: string) {
    const body: { email: string; password: string; role: string; full_name?: string } = {
      email,
      password,
      role,
    };
    if (fullName) body.full_name = fullName;
    return request<UserInfo>("/api/v1/users", {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  updateUser(
    id: string,
    updates: {
      role?: string;
      password?: string;
      email?: string;
      full_name?: string;
      phone?: string;
      phone_country?: string;
    },
  ) {
    return request<StatusResponse>(`/api/v1/users/${e(id)}`, {
      method: "PUT",
      body: JSON.stringify(updates),
    });
  },
  deleteUser(id: string) {
    return request<StatusResponse>(`/api/v1/users/${e(id)}`, { method: "DELETE" });
  },

  // Audit (read-only admin review surface, requires audit:read)
  getAuditEvents(params?: {
    event?: string;
    entity_type?: string;
    entity_id?: string;
    limit?: number;
    skip?: number;
  }) {
    return request<{ items: AuditEventRow[]; total: number }>(
      `/api/v1/audit-events${buildQuery({
        event: params?.event,
        entity_type: params?.entity_type,
        entity_id: params?.entity_id,
        limit: params?.limit,
        skip: params?.skip,
      })}`,
    );
  },

  // Alerts
  getAlerts(params: {
    status?: string;
    start_date?: string;
    end_date?: string;
    search?: string;
    sort?: string;
    limit?: number;
    skip?: number;
  }) {
    return request<AlertRecord[] | null>(
      `/api/v1/alerts${buildQuery({
        status: params.status,
        start_date: params.start_date,
        end_date: params.end_date,
        search: params.search,
        sort: params.sort,
        limit: params.limit,
        skip: params.skip,
      })}`,
      undefined,
      alertListSchema,
    );
  },
  createAlert(input: {
    alertname: string;
    severity?: string;
    message?: string;
    description?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
    source?: string;
  }) {
    return request<AlertRecord>("/api/v1/alerts", {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
  getAlert(alertNumber: number) {
    return request<AlertDetailResponse>(
      `/api/v1/alerts/${e(alertNumber)}`,
      undefined,
      alertDetailResponseSchema,
    );
  },
  getAlertRelated(alertNumber: number) {
    return request<AlertRelatedResponse>(`/api/v1/alerts/${e(alertNumber)}/related`);
  },
  acknowledgeAlert(alertNumber: number) {
    return request<AlertRecord>(`/api/v1/alerts/${e(alertNumber)}/acknowledge`, {
      method: "POST",
    });
  },
  resolveAlert(alertNumber: number) {
    return request<AlertRecord>(`/api/v1/alerts/${e(alertNumber)}/resolve`, {
      method: "POST",
    });
  },
  reopenAlert(alertNumber: number) {
    return request<AlertRecord>(`/api/v1/alerts/${e(alertNumber)}/reopen`, {
      method: "POST",
    });
  },
  deleteAlert(alertNumber: number) {
    return request<StatusResponse>(`/api/v1/alerts/${e(alertNumber)}`, {
      method: "DELETE",
    });
  },
  investigateAlert(alertNumber: number) {
    return request<AlertDetailResponse>(`/api/v1/alerts/${e(alertNumber)}/investigate`, {
      method: "POST",
    });
  },
  assignAlertInvestigation(
    investigationId: string,
    assigneeType: "user" | "agent",
    assigneeId?: string,
  ) {
    return request<AlertInvestigationRecord>(
      `/api/v1/alert-investigations/${e(investigationId)}/assign`,
      {
        method: "PATCH",
        body: JSON.stringify({ assignee_type: assigneeType, assignee_id: assigneeId }),
      },
    );
  },
  assignIncidentInvestigation(
    investigationId: string,
    assigneeType: "user" | "agent",
    assigneeId?: string,
  ) {
    return request<{ data: unknown }>(
      `/api/v1/incident-investigations/${e(investigationId)}/assign`,
      {
        method: "PATCH",
        body: JSON.stringify({ assignee_type: assigneeType, assignee_id: assigneeId }),
      },
    );
  },
  getAlertThread(alertNumber: number) {
    return request<OwnerThreadWireResponse>(`/api/v1/alerts/${e(alertNumber)}/thread`).then(
      normalizeOwnerThreadResponse,
    );
  },
  addAlertThreadMessage(
    alertNumber: number,
    body: {
      message: string;
      type?: OwnerThreadMessageType;
      reply_to_message_id?: string;
      mentions?: string[];
    },
  ) {
    return request<OwnerThreadWireResponse>(`/api/v1/alerts/${e(alertNumber)}/thread/messages`, {
      method: "POST",
      body: JSON.stringify(body),
    }).then(normalizeOwnerThreadResponse);
  },
  postAlertThreadTyping(alertNumber: number) {
    return request<StatusResponse>(`/api/v1/alerts/${e(alertNumber)}/thread/typing`, {
      method: "POST",
    });
  },
  getIncidentThread(incidentNumber: string | number) {
    return request<OwnerThreadWireResponse>(`/api/v1/incidents/${e(incidentNumber)}/thread`).then(
      normalizeOwnerThreadResponse,
    );
  },
  addIncidentThreadMessage(
    incidentNumber: string | number,
    body: {
      message: string;
      type?: OwnerThreadMessageType;
      reply_to_message_id?: string;
      mentions?: string[];
    },
  ) {
    return request<OwnerThreadWireResponse>(
      `/api/v1/incidents/${e(incidentNumber)}/thread/messages`,
      {
        method: "POST",
        body: JSON.stringify(body),
      },
    ).then(normalizeOwnerThreadResponse);
  },

  // Webhook Tokens
  getWebhookTokens() {
    return request<WebhookTokenRow[]>("/api/v1/webhook-tokens");
  },
  createWebhookToken(name: string, expiresAtRFC3339?: string) {
    const body: { name: string; expires_at?: string } = { name };
    if (expiresAtRFC3339) body.expires_at = expiresAtRFC3339;
    return request<WebhookTokenRow>("/api/v1/webhook-tokens", {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  revokeWebhookToken(id: string) {
    return request<StatusResponse>(`/api/v1/webhook-tokens/${e(id)}`, { method: "DELETE" });
  },

  // Agent Tokens
  getAgentTokens() {
    return request<AgentTokenRow[]>("/api/v1/agent-tokens");
  },
  createAgentToken(
    name: string,
    expiresAtRFC3339?: string,
    agentType?: AgentType,
    scope?: "all" | "labels",
    labelSelectors?: RouteCondition[],
    capabilities?: AgentCapability[],
  ) {
    const body: {
      name: string;
      expires_at?: string;
      agent_type?: AgentType;
      scope?: string;
      label_selectors?: RouteCondition[];
      capabilities?: AgentCapability[];
    } = { name };
    if (expiresAtRFC3339) body.expires_at = expiresAtRFC3339;
    if (agentType) body.agent_type = agentType;
    if (scope) body.scope = scope;
    if (labelSelectors) body.label_selectors = labelSelectors;
    if (capabilities) body.capabilities = capabilities;
    return request<AgentTokenRow>("/api/v1/agent-tokens", {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  revokeAgentToken(id: string) {
    return request<StatusResponse>(`/api/v1/agent-tokens/${e(id)}`, { method: "DELETE" });
  },
  regenerateAgentToken(id: string) {
    return request<AgentTokenRow>(`/api/v1/agent-tokens/${e(id)}/regenerate`, {
      method: "POST",
    });
  },
  updateAgentToken(
    id: string,
    scope: "all" | "labels",
    labelSelectors?: RouteCondition[],
    enabled?: boolean,
    capabilities?: AgentCapability[],
  ) {
    const body: {
      scope: string;
      label_selectors?: RouteCondition[];
      enabled?: boolean;
      capabilities?: AgentCapability[];
    } = { scope, label_selectors: labelSelectors ?? [] };
    if (enabled !== undefined) body.enabled = enabled;
    if (capabilities !== undefined) body.capabilities = capabilities;
    return request<{
      status: string;
      scope: string;
      label_selectors?: RouteCondition[];
      capabilities?: AgentCapability[];
    }>(`/api/v1/agent-tokens/${e(id)}`, {
      method: "PUT",
      body: JSON.stringify(body),
    });
  },

  // Personal Access Tokens
  getPATs() {
    return unwrapData<PATRow>(request<{ data: PATRow[] }>("/api/v1/user/tokens"));
  },
  createPAT(name: string, permissions: string[], expiresAtRFC3339?: string) {
    const body: {
      name: string;
      permissions: string[];
      expires_at?: string;
    } = { name, permissions };
    if (expiresAtRFC3339) body.expires_at = expiresAtRFC3339;
    return request<PATRow & { token: string }>("/api/v1/user/tokens", {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  revokePAT(id: string) {
    return request<StatusResponse>(`/api/v1/user/tokens/${e(id)}`, { method: "DELETE" });
  },
  getAdminPATs() {
    return unwrapData<PATRow>(request<{ data: PATRow[] }>("/api/v1/admin/tokens"));
  },
  revokeAdminPAT(id: string) {
    return request<StatusResponse>(`/api/v1/admin/tokens/${e(id)}`, { method: "DELETE" });
  },

  // Agent Direct Messages
  listAgentDMMessages(agentTokenId: string, opts?: { before?: string; limit?: number }) {
    return request<{ items: AgentDMMessageRow[]; has_more: boolean; chat_id: string }>(
      `/api/v1/agent-tokens/${e(agentTokenId)}/chat/messages${buildQuery({
        before: opts?.before,
        limit: opts?.limit,
      })}`,
    );
  },
  postAgentDMMessage(agentTokenId: string, message: string) {
    return request<AgentDMMessageRow>(`/api/v1/agent-tokens/${e(agentTokenId)}/chat/messages`, {
      method: "POST",
      body: JSON.stringify({ message }),
    });
  },
  postAgentDMTyping(agentTokenId: string) {
    return request<StatusResponse>(`/api/v1/agent-tokens/${e(agentTokenId)}/chat/typing`, {
      method: "POST",
    });
  },

  // Routes
  getRoutes() {
    return request<{ routes: RouteConfig[]; default_destinations?: RouteTarget[] }>(
      "/api/v1/routes",
    );
  },
  updateRoutes(payload: { routes: RouteConfig[] }) {
    return request<StatusResponse>("/api/v1/routes", {
      method: "PUT",
      body: JSON.stringify(payload),
    });
  },

  // Integrations
  getIntegrations() {
    return request<IntegrationInfo>("/api/v1/integrations");
  },
  updateIntegrations(payload: {
    voice_provider?: "twilio" | "telnyx";
    mattermost?: {
      url: string;
      secret: string;
      team: string;
      default_channel: string;
      provider_enabled?: boolean;
    };
    slack?: {
      bot_token: string;
      signing_secret: string;
      default_channel: string;
      provider_enabled?: boolean;
      client_id?: string;
      client_secret?: string;
    };
    twilio?: {
      account_sid: string;
      auth_token: string;
      from_number: string;
      provider_enabled?: boolean;
    };
    telnyx?: {
      api_key: string;
      connection_id: string;
      from_number: string;
      public_key: string;
      tts_voice: string;
      tts_language: string;
      tts_api_key_ref: string;
      provider_enabled?: boolean;
    };
  }) {
    return request<StatusResponse>("/api/v1/integrations", {
      method: "PUT",
      body: JSON.stringify(payload),
    });
  },
  testIntegration(
    provider: "mattermost" | "slack",
    credentials?: {
      mattermost?: { url: string; secret: string; team: string };
      slack?: { bot_token: string };
    },
  ) {
    return request<{ status: string; message: string }>("/api/v1/integrations/test", {
      method: "POST",
      body: JSON.stringify({ provider, ...credentials }),
    });
  },

  initiateSlackOAuth() {
    return request<{ url: string }>("/api/v1/integrations/slack/oauth/authorize", {
      method: "POST",
    });
  },

  disconnectSlack() {
    return request<StatusResponse>("/api/v1/integrations/slack/disconnect", {
      method: "POST",
    });
  },

  initiateUserSlackOAuth() {
    return request<{ url: string }>("/api/v1/users/me/slack/authorize", {
      method: "POST",
    });
  },

  disconnectUserSlack() {
    return request<StatusResponse>("/api/v1/users/me/slack/disconnect", {
      method: "POST",
    });
  },

  initiateUserGoogleOAuth() {
    return request<{ url: string }>("/api/v1/users/me/google/authorize", {
      method: "POST",
    });
  },

  disconnectUserGoogle() {
    return request<StatusResponse>("/api/v1/users/me/google/disconnect", {
      method: "POST",
    });
  },

  // Search
  searchAlerts(query: string, limit = 10) {
    return request<AlertRecord[] | null>(
      `/api/v1/alerts${buildQuery({ search: query || undefined, limit })}`,
    );
  },
  searchIncidents(query: string, limit = 10) {
    return request<PaginatedIncidents>(
      `/api/v1/incidents${buildQuery({ search: query || undefined, limit })}`,
    );
  },

  // Knowledge base
  getKnowledgeNotes(params?: {
    kind?: string;
    tag?: string;
    q?: string;
    author_type?: string;
    limit?: number;
    skip?: number;
  }) {
    return request<{ items: KnowledgeNote[]; total: number }>(
      `/api/v1/knowledge${buildQuery({
        kind: params?.kind,
        tag: params?.tag,
        q: params?.q,
        author_type: params?.author_type,
        limit: params?.limit,
        skip: params?.skip,
      })}`,
    );
  },
  createKnowledgeNote(input: KnowledgeNoteInput) {
    return request<KnowledgeNote>("/api/v1/knowledge", {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
  updateKnowledgeNote(id: string, input: Partial<KnowledgeNoteInput>) {
    return request<KnowledgeNote>(`/api/v1/knowledge/${e(id)}`, {
      method: "PUT",
      body: JSON.stringify(input),
    });
  },
  deleteKnowledgeNote(id: string) {
    return request<StatusResponse>(`/api/v1/knowledge/${e(id)}`, { method: "DELETE" });
  },

  // Notifications
  getNotifications(limit?: number, skip?: number) {
    return request<NotificationRecord[]>(`/api/v1/notifications${buildQuery({ limit, skip })}`);
  },
  getUnreadNotificationCount() {
    return request<{ count: number }>("/api/v1/notifications/unread-count");
  },
  markNotificationRead(id: string) {
    return request<StatusResponse>(`/api/v1/notifications/${e(id)}/read`, { method: "POST" });
  },
  markAllNotificationsRead() {
    return request<StatusResponse>("/api/v1/notifications/read-all", { method: "POST" });
  },

  // Dashboard
  getDashboardStats() {
    return request<DashboardStats>("/api/v1/dashboard/stats");
  },

  getDailySummary() {
    return request<DailySummaryResponse>("/api/v1/dashboard/daily-summary");
  },

  generateDailySummary() {
    return request<DailySummaryResponse>("/api/v1/dashboard/daily-summary", { method: "POST" });
  },

  getChannels() {
    return request<{ name: string; display_name?: string }[]>("/api/v1/channels");
  },

  getDestinations(provider?: string) {
    return request<{ name: string; id?: string }[]>(
      `/api/v1/destinations${buildQuery({ provider })}`,
    );
  },

  // Memories
  getMemories(params?: {
    q?: string;
    memory_type?: string;
    agent_id?: string;
    investigation_id?: string;
    limit?: number;
    offset?: number;
  }) {
    return request<{ items: AgentMemoryRecord[]; total: number }>(
      `/api/v1/memories${buildQuery({
        q: params?.q,
        memory_type: params?.memory_type,
        agent_id: params?.agent_id,
        investigation_id: params?.investigation_id,
        limit: params?.limit,
        offset: params?.offset,
      })}`,
    );
  },
  createMemory(input: AgentMemoryInput) {
    return request<AgentMemoryRecord>("/api/v1/memories", {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
  updateMemory(id: string, content: string) {
    return request<AgentMemoryRecord>(`/api/v1/memories/${e(id)}`, {
      method: "PUT",
      body: JSON.stringify({ content }),
    });
  },
  deleteMemory(id: string) {
    return request<StatusResponse>(`/api/v1/memories/${e(id)}`, { method: "DELETE" });
  },

  // System Config
  getSystemConfig() {
    return request<SystemConfigValues>("/api/v1/system/config");
  },
  updateSystemConfig(payload: Partial<SystemConfigValues>) {
    return request<StatusResponse>("/api/v1/system/config", {
      method: "PUT",
      body: JSON.stringify(payload),
    });
  },

  // Maintenance Windows
  getMaintenanceWindows(params?: { enabled?: boolean }) {
    return unwrapItems<MaintenanceWindowRecord>(
      request<{ items: MaintenanceWindowRecord[]; total: number }>(
        `/api/v1/maintenance-windows${buildQuery({ enabled: params?.enabled })}`,
      ),
    );
  },
  getMaintenanceWindow(id: string) {
    return request<MaintenanceWindowRecord>(`/api/v1/maintenance-windows/${e(id)}`);
  },
  createMaintenanceWindow(input: {
    name: string;
    start_time: string;
    end_time: string;
    label_matchers?: Record<string, string>;
    enabled?: boolean;
  }) {
    return request<MaintenanceWindowRecord>("/api/v1/maintenance-windows", {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
  updateMaintenanceWindow(
    id: string,
    input: {
      name?: string;
      start_time?: string;
      end_time?: string;
      label_matchers?: Record<string, string>;
      enabled?: boolean;
    },
  ) {
    return request<MaintenanceWindowRecord>(`/api/v1/maintenance-windows/${e(id)}`, {
      method: "PUT",
      body: JSON.stringify(input),
    });
  },
  deleteMaintenanceWindow(id: string) {
    return request<StatusResponse>(`/api/v1/maintenance-windows/${e(id)}`, {
      method: "DELETE",
    });
  },

  // Heartbeats
  getHeartbeats(params?: { enabled?: boolean; status?: string; search?: string }) {
    return unwrapItems<HeartbeatRecord>(
      request<{ items: HeartbeatRecord[]; total: number }>(
        `/api/v1/heartbeats${buildQuery({
          enabled: params?.enabled,
          status: params?.status,
          search: params?.search,
        })}`,
      ),
    );
  },
  getHeartbeat(id: string) {
    return request<HeartbeatRecord>(`/api/v1/heartbeats/${e(id)}`);
  },
  createHeartbeat(input: {
    name: string;
    description?: string;
    interval_seconds: number;
    grace_seconds?: number;
    severity?: string;
    labels?: Record<string, string>;
    owner_team_id?: string;
    enabled?: boolean;
  }) {
    return request<HeartbeatRecord>("/api/v1/heartbeats", {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
  updateHeartbeat(
    id: string,
    input: {
      name?: string;
      description?: string;
      interval_seconds?: number;
      grace_seconds?: number;
      severity?: string;
      labels?: Record<string, string>;
      owner_team_id?: string;
      enabled?: boolean;
    },
  ) {
    return request<HeartbeatRecord>(`/api/v1/heartbeats/${e(id)}`, {
      method: "PUT",
      body: JSON.stringify(input),
    });
  },
  deleteHeartbeat(id: string) {
    return request<StatusResponse>(`/api/v1/heartbeats/${e(id)}`, {
      method: "DELETE",
    });
  },
  regenerateHeartbeatToken(id: string) {
    return request<HeartbeatRecord>(`/api/v1/heartbeats/${e(id)}/regenerate-token`, {
      method: "POST",
    });
  },
  heartbeatPingUrl(token: string) {
    const origin = typeof window !== "undefined" ? window.location.origin : "";
    return `${origin}/api/v1/heartbeats/ping/${token}`;
  },

  // Status Pages
  getStatusPages(params?: { enabled?: boolean; search?: string }) {
    return unwrapItems<StatusPageRecord>(
      request<{ items: StatusPageRecord[]; total: number }>(
        `/api/v1/status-pages${buildQuery({ enabled: params?.enabled, search: params?.search })}`,
      ),
    );
  },
  getStatusPageView(slug: string) {
    return request<StatusPageView>(`/api/v1/status-pages/slug/${e(slug)}`);
  },
  createStatusPage(input: {
    name: string;
    slug: string;
    description?: string;
    visibility?: StatusPageVisibility;
    owner_team_id?: string;
    enabled?: boolean;
  }) {
    return request<StatusPageRecord>("/api/v1/status-pages", {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
  updateStatusPage(
    id: string,
    input: {
      name?: string;
      slug?: string;
      description?: string;
      visibility?: StatusPageVisibility;
      owner_team_id?: string;
      enabled?: boolean;
    },
  ) {
    return request<StatusPageRecord>(`/api/v1/status-pages/${e(id)}`, {
      method: "PUT",
      body: JSON.stringify(input),
    });
  },
  deleteStatusPage(id: string) {
    return request<StatusResponse>(`/api/v1/status-pages/${e(id)}`, {
      method: "DELETE",
    });
  },
  getStatusPageComponents(pageId: string) {
    return unwrapItems<StatusPageComponentRecord>(
      request<{ items: StatusPageComponentRecord[] }>(
        `/api/v1/status-pages/${e(pageId)}/components`,
      ),
    );
  },
  createStatusPageComponent(
    pageId: string,
    input: {
      name: string;
      description?: string;
      service_id?: string;
      display_order?: number;
      status?: ComponentStatus;
    },
  ) {
    return request<StatusPageComponentRecord>(`/api/v1/status-pages/${e(pageId)}/components`, {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
  updateStatusPageComponent(
    pageId: string,
    componentId: string,
    input: {
      name?: string;
      description?: string;
      service_id?: string;
      display_order?: number;
      status?: ComponentStatus;
    },
  ) {
    return request<StatusPageComponentRecord>(
      `/api/v1/status-pages/${e(pageId)}/components/${e(componentId)}`,
      { method: "PUT", body: JSON.stringify(input) },
    );
  },
  deleteStatusPageComponent(pageId: string, componentId: string) {
    return request<StatusResponse>(
      `/api/v1/status-pages/${e(pageId)}/components/${e(componentId)}`,
      { method: "DELETE" },
    );
  },

  // Incidents
  getIncidentMetrics(startDate?: string, endDate?: string) {
    return request<IncidentMetrics>(
      `/api/v1/incidents/metrics${buildQuery({ start_date: startDate, end_date: endDate })}`,
    );
  },

  getIncidents(params: {
    status?: string;
    priority?: string;
    search?: string;
    sort?: string;
    limit?: number;
    skip?: number;
  }) {
    return request<PaginatedIncidents>(
      `/api/v1/incidents${buildQuery({
        status: params.status,
        priority: params.priority,
        search: params.search,
        sort: params.sort,
        limit: params.limit,
        skip: params.skip,
      })}`,
      undefined,
      incidentListSchema,
    );
  },
  createIncident(input: {
    title: string;
    description?: string;
    severity?: Severity;
    impact_level?: ImpactLevel;
    priority?: IncidentPriority;
    incident_type?: string;
    service_id?: string;
    conference_url?: string;
    tags?: string[];
    custom_fields?: Record<string, unknown>;
    alert_numbers?: number[];
  }) {
    return request<IncidentRecord>("/api/v1/incidents", {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
  getIncident(incidentNumber: string | number) {
    return request<{ incident: IncidentRecord }>(
      `/api/v1/incidents/${e(incidentNumber)}`,
      undefined,
      incidentDetailSchema,
    ).then((r) => r.incident);
  },
  patchIncident(
    incidentNumber: string | number,
    body: {
      title?: string;
      description?: string;
      summary?: string;
      severity?: Severity;
      impact_level?: ImpactLevel;
      priority?: IncidentPriority;
      incident_type?: string;
      conference_url?: string;
      tags?: string[];
      custom_fields?: Record<string, unknown>;
    },
  ) {
    return request<IncidentRecord>(`/api/v1/incidents/${e(incidentNumber)}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    });
  },
  deleteIncident(incidentNumber: string | number) {
    return request<StatusResponse>(`/api/v1/incidents/${e(incidentNumber)}`, {
      method: "DELETE",
    });
  },
  acknowledgeIncident(incidentNumber: string | number) {
    return request<IncidentRecord>(`/api/v1/incidents/${e(incidentNumber)}/acknowledge`, {
      method: "POST",
    });
  },
  mitigateIncident(incidentNumber: string | number) {
    return request<IncidentRecord>(`/api/v1/incidents/${e(incidentNumber)}/mitigate`, {
      method: "POST",
    });
  },
  resolveIncident(incidentNumber: string | number) {
    return request<IncidentResolveResponse>(`/api/v1/incidents/${e(incidentNumber)}/resolve`, {
      method: "POST",
    });
  },
  closeIncident(incidentNumber: string | number) {
    return request<IncidentResolveResponse>(`/api/v1/incidents/${e(incidentNumber)}/close`, {
      method: "POST",
    });
  },
  reopenIncident(incidentNumber: string | number) {
    return request<IncidentRecord>(`/api/v1/incidents/${e(incidentNumber)}/reopen`, {
      method: "POST",
    });
  },
  cancelIncident(incidentNumber: string | number) {
    return request<IncidentRecord>(`/api/v1/incidents/${e(incidentNumber)}/cancel`, {
      method: "POST",
    });
  },
  createIncidentSlackChannel(incidentNumber: string | number) {
    return request<IncidentRecord>(`/api/v1/incidents/${e(incidentNumber)}/slack-channel`, {
      method: "POST",
    });
  },
  deleteIncidentSlackChannel(incidentNumber: string | number) {
    return request<IncidentRecord>(`/api/v1/incidents/${e(incidentNumber)}/slack-channel`, {
      method: "DELETE",
    });
  },
  createIncidentGoogleMeet(incidentNumber: string | number) {
    return request<IncidentRecord>(`/api/v1/incidents/${e(incidentNumber)}/google-meet`, {
      method: "POST",
    });
  },
  unlinkIncidentGoogleMeet(incidentNumber: string | number) {
    return request<IncidentRecord>(`/api/v1/incidents/${e(incidentNumber)}/google-meet`, {
      method: "DELETE",
    });
  },
  getIncidentTimeline(incidentNumber: string | number) {
    return request<IncidentTimelineRecord[]>(`/api/v1/incidents/${e(incidentNumber)}/timeline`);
  },
  addIncidentTimelineEntry(
    incidentNumber: string | number,
    body: { event_type: string; message: string; metadata?: Record<string, unknown> },
  ) {
    return request<IncidentTimelineRecord>(`/api/v1/incidents/${e(incidentNumber)}/timeline`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  getIncidentAlerts(incidentNumber: string | number) {
    return request<AlertRecord[]>(`/api/v1/incidents/${e(incidentNumber)}/alerts`);
  },
  linkAlertToIncident(incidentNumber: string | number, alertNumber: number) {
    return request<IncidentRecord>(`/api/v1/incidents/${e(incidentNumber)}/alerts`, {
      method: "POST",
      body: JSON.stringify({ alert_number: alertNumber }),
    });
  },
  unlinkAlertFromIncident(incidentNumber: string | number, alertNumber: number) {
    return request<IncidentRecord>(
      `/api/v1/incidents/${e(incidentNumber)}/alerts/${e(alertNumber)}`,
      {
        method: "DELETE",
      },
    );
  },
  getICSRoles(incidentNumber: string | number) {
    return request<ICSRoleRecord[]>(`/api/v1/incidents/${e(incidentNumber)}/ics/roles`);
  },
  assignICSRole(
    incidentNumber: string | number,
    body: {
      role_type: ICSRoleType;
      user_id?: string;
      agent_token_id?: string;
      parent_assignment_id?: string;
      scope_description?: string;
    },
  ) {
    return request<ICSRoleRecord>(`/api/v1/incidents/${e(incidentNumber)}/ics/roles`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  endICSRole(
    incidentNumber: string | number,
    roleId: string,
    body: { ended_reason: ICSEndReason },
  ) {
    return request<StatusResponse>(
      `/api/v1/incidents/${e(incidentNumber)}/ics/roles/${e(roleId)}`,
      {
        method: "DELETE",
        body: JSON.stringify(body),
      },
    );
  },
  getIncidentDocument(incidentNumber: string | number) {
    return request<IncidentDocumentSection[]>(
      `/api/v1/incidents/${e(incidentNumber)}/ics/document`,
    );
  },
  updateIncidentDocumentSection(
    incidentNumber: string | number,
    section: DocumentSection,
    body: {
      content: string;
      version: number;
    },
  ) {
    return request<IncidentDocumentSection>(
      `/api/v1/incidents/${e(incidentNumber)}/ics/document/${e(section)}`,
      { method: "PUT", body: JSON.stringify(body) },
    );
  },
  promoteIncident(incidentNumber: string | number) {
    return request<IncidentRecord>(`/api/v1/incidents/${e(incidentNumber)}/promote`, {
      method: "POST",
    });
  },
  escalateIncident(incidentNumber: string | number) {
    return request<IncidentRecord>(`/api/v1/incidents/${e(incidentNumber)}/escalate`, {
      method: "POST",
    });
  },

  getIncidentCoordinationMessages(
    incidentNumber: string | number,
    params: { limit?: number; skip?: number } = {},
  ) {
    return request<IncidentCoordinationMessage[]>(
      `/api/v1/incidents/${e(incidentNumber)}/coordination/messages${buildQuery({
        limit: params.limit,
        skip: params.skip,
      })}`,
    );
  },
  addIncidentCoordinationMessage(
    incidentNumber: string | number,
    body: {
      kind?: IncidentCoordinationMessageKind;
      body: string;
      internal?: boolean;
      mentions?: string[];
      linked_investigation_id?: string;
      metadata?: Record<string, unknown>;
    },
  ) {
    return request<IncidentCoordinationMessage>(
      `/api/v1/incidents/${e(incidentNumber)}/coordination/messages`,
      { method: "POST", body: JSON.stringify(body) },
    );
  },

  getIncidentStatusUpdates(
    incidentNumber: string | number,
    params: { limit?: number; skip?: number } = {},
  ) {
    return request<IncidentCoordinationMessage[]>(
      `/api/v1/incidents/${e(incidentNumber)}/status-updates${buildQuery({
        limit: params.limit,
        skip: params.skip,
      })}`,
    );
  },

  createIncidentStatusUpdate(incidentNumber: string | number, body: StatusUpdateCreateRequest) {
    return request<IncidentCoordinationMessage>(
      `/api/v1/incidents/${e(incidentNumber)}/status-updates`,
      { method: "POST", body: JSON.stringify(body) },
    );
  },

  // Onboarding
  getOnboardingStatus() {
    return request<{ completed: boolean }>("/api/v1/onboarding/status");
  },

  completeOnboarding() {
    return request<StatusResponse>("/api/v1/onboarding/complete", { method: "POST" });
  },

  // Services
  getServices(params?: { status?: string; q?: string; limit?: number; skip?: number }) {
    return request<{ items: ServiceRecord[]; total: number }>(
      `/api/v1/services${buildQuery({
        status: params?.status,
        q: params?.q,
        limit: params?.limit,
        skip: params?.skip,
      })}`,
    );
  },
  createService(input: {
    name: string;
    display_name: string;
    description?: string;
    owner_team_id?: string;
    label_matchers?: Record<string, unknown>[];
    sla_response_minutes?: number;
    sla_resolve_minutes?: number;
  }) {
    return request<ServiceRecord>("/api/v1/services", {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
  getService(serviceId: string) {
    return request<ServiceRecord>(`/api/v1/services/${e(serviceId)}`);
  },
  patchService(
    serviceId: string,
    body: {
      display_name?: string;
      description?: string;
      owner_team_id?: string;
      label_matchers?: Record<string, unknown>[];
      sla_response_minutes?: number;
      sla_resolve_minutes?: number;
      status?: string;
    },
  ) {
    return request<ServiceRecord>(`/api/v1/services/${e(serviceId)}`, {
      method: "PATCH",
      body: JSON.stringify(body),
    });
  },
  deleteService(serviceId: string) {
    return request<StatusResponse>(`/api/v1/services/${e(serviceId)}`, { method: "DELETE" });
  },
  getServiceDependencies(serviceId: string) {
    return request<ServiceDependencyRecord[]>(`/api/v1/services/${e(serviceId)}/dependencies`);
  },
  addServiceDependency(
    serviceId: string,
    body: { dependent_on_service_id: string; dependency_type: string },
  ) {
    return request<ServiceDependencyRecord>(`/api/v1/services/${e(serviceId)}/dependencies`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },
  removeServiceDependency(serviceId: string, dependencyId: string) {
    return request<StatusResponse>(
      `/api/v1/services/${e(serviceId)}/dependencies/${e(dependencyId)}`,
      {
        method: "DELETE",
      },
    );
  },
  getServiceIncidents(serviceId: string, params?: { limit?: number; skip?: number }) {
    return request<IncidentRecord[]>(
      `/api/v1/services/${e(serviceId)}/incidents${buildQuery({ limit: params?.limit, skip: params?.skip })}`,
    );
  },

  // Teams
  getTeams(params?: { q?: string; limit?: number; skip?: number }) {
    return request<{ items: TeamRecord[]; total: number }>(
      `/api/v1/teams${buildQuery({ q: params?.q, limit: params?.limit, skip: params?.skip })}`,
    );
  },
  createTeam(input: { name: string; description?: string }) {
    return request<TeamRecord>("/api/v1/teams", {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
  getTeam(teamId: string) {
    return request<TeamRecord>(`/api/v1/teams/${e(teamId)}`);
  },
  updateTeam(teamId: string, input: { name?: string; description?: string }) {
    return request<TeamRecord>(`/api/v1/teams/${e(teamId)}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    });
  },
  deleteTeam(teamId: string) {
    return request<StatusResponse>(`/api/v1/teams/${e(teamId)}`, { method: "DELETE" });
  },
  getTeamMembers(teamId: string) {
    return request<TeamMemberRecord[]>(`/api/v1/teams/${e(teamId)}/members`);
  },
  addTeamMember(teamId: string, input: { user_id: string; role?: string }) {
    return request<TeamMemberRecord>(`/api/v1/teams/${e(teamId)}/members`, {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
  updateTeamMemberRole(teamId: string, userId: string, role: string) {
    return request<StatusResponse>(`/api/v1/teams/${e(teamId)}/members/${e(userId)}`, {
      method: "PATCH",
      body: JSON.stringify({ role }),
    });
  },
  removeTeamMember(teamId: string, userId: string) {
    return request<StatusResponse>(`/api/v1/teams/${e(teamId)}/members/${e(userId)}`, {
      method: "DELETE",
    });
  },

  // Escalation Policies
  getEscalationPolicies(params?: { q?: string; limit?: number; skip?: number }) {
    return request<{ items: EscalationPolicyRecord[]; total: number }>(
      `/api/v1/escalation-policies${buildQuery({ q: params?.q, limit: params?.limit, skip: params?.skip })}`,
    );
  },
  createEscalationPolicy(input: {
    name: string;
    description?: string;
    repeat_count?: number;
    levels?: Array<{
      delay_minutes: number;
      notify_channels?: string[];
      targets?: Array<{
        target_type: string;
        target_user_id?: string;
        target_team_id?: string;
      }>;
    }>;
  }) {
    return request<EscalationPolicyRecord>("/api/v1/escalation-policies", {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
  getEscalationPolicy(policyId: string) {
    return request<EscalationPolicyRecord>(`/api/v1/escalation-policies/${e(policyId)}`);
  },
  updateEscalationPolicy(
    policyId: string,
    input: {
      name?: string;
      description?: string;
      repeat_count?: number;
      levels?: Array<{
        delay_minutes: number;
        notify_channels?: string[];
        targets?: Array<{
          target_type: string;
          target_user_id?: string;
          target_team_id?: string;
        }>;
      }>;
    },
  ) {
    return request<EscalationPolicyRecord>(`/api/v1/escalation-policies/${e(policyId)}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    });
  },
  deleteEscalationPolicy(policyId: string) {
    return request<StatusResponse>(`/api/v1/escalation-policies/${e(policyId)}`, {
      method: "DELETE",
    });
  },

  // On-Call Schedules
  getSchedules(params?: { team_id?: string; q?: string; limit?: number; skip?: number }) {
    return request<{ items: OnCallScheduleRecord[]; total: number }>(
      `/api/v1/on-call/schedules${buildQuery({
        team_id: params?.team_id,
        q: params?.q,
        limit: params?.limit,
        skip: params?.skip,
      })}`,
    );
  },
  getSchedule(scheduleId: string) {
    return request<OnCallScheduleRecord>(`/api/v1/on-call/schedules/${e(scheduleId)}`);
  },
  updateSchedule(
    scheduleId: string,
    input: {
      layers?: ScheduleLayerInput[];
    },
  ) {
    return request<OnCallScheduleRecord>(`/api/v1/on-call/schedules/${e(scheduleId)}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    });
  },
  getScheduleCurrent(scheduleId: string) {
    return request<OnCallCurrent>(`/api/v1/on-call/schedules/${e(scheduleId)}/current`);
  },
  getScheduleTimeline(scheduleId: string, range?: { from?: string; to?: string }) {
    return request<ScheduleShift[]>(
      `/api/v1/on-call/schedules/${e(scheduleId)}/timeline${buildQuery({
        from: range?.from,
        to: range?.to,
      })}`,
    );
  },
  scheduleICalUrl(scheduleId: string) {
    return `/api/v1/on-call/schedules/${e(scheduleId)}/ical`;
  },
  getScheduleOverrides(scheduleId: string) {
    return request<ScheduleOverrideRecord[]>(
      `/api/v1/on-call/schedules/${e(scheduleId)}/overrides`,
    );
  },
  createScheduleOverride(
    scheduleId: string,
    input: { user_id: string; start_at: string; end_at: string },
  ) {
    return request<ScheduleOverrideRecord>(`/api/v1/on-call/schedules/${e(scheduleId)}/overrides`, {
      method: "POST",
      body: JSON.stringify(input),
    });
  },
  deleteScheduleOverride(overrideId: string) {
    return request<StatusResponse>(`/api/v1/on-call/overrides/${e(overrideId)}`, {
      method: "DELETE",
    });
  },
  getWhoIsOnCall() {
    return request<OnCallCurrent[]>("/api/v1/on-call/who-is-on-call");
  },
  getMyOnCall() {
    return request<OnCallSelfView>("/api/v1/on-call/me");
  },

  // On-Call Handoffs
  getHandoff(id: string) {
    return request<HandoffRecord>(`/api/v1/on-call/handoffs/${e(id)}`);
  },
  pendingHandoffs() {
    return request<HandoffRecord[]>("/api/v1/on-call/handoffs/pending");
  },
  updateHandoffNotes(id: string, field: "outgoing_notes" | "incoming_notes", notes: string) {
    return request<StatusResponse>(`/api/v1/on-call/handoffs/${e(id)}/notes`, {
      method: "POST",
      body: JSON.stringify({ field, notes }),
    });
  },
  acknowledgeHandoff(id: string) {
    return request<StatusResponse>(`/api/v1/on-call/handoffs/${e(id)}/acknowledge`, {
      method: "POST",
    });
  },

  // Playbooks
  listPlaybooks(params?: {
    kind?: string;
    service_id?: string;
    tag?: string;
    search?: string;
    limit?: number;
    skip?: number;
  }) {
    return request<{ items: PlaybookRecord[]; total: number }>(
      `/api/v1/playbooks${buildQuery({
        kind: params?.kind,
        service_id: params?.service_id,
        tag: params?.tag,
        search: params?.search,
        limit: params?.limit,
        skip: params?.skip,
      })}`,
    );
  },
  getPlaybook(id: string) {
    return request<PlaybookRecord>(`/api/v1/playbooks/${e(id)}`);
  },
  createPlaybook(data: {
    title: string;
    kind: string;
    summary?: string;
    service_id?: string;
    label_selectors?: Array<{ key: string; op: string; value: string }>;
    tags?: string[];
    steps?: Array<{
      title: string;
      description?: string;
      expected_duration?: string;
      command?: string;
    }>;
  }) {
    return request<PlaybookRecord>("/api/v1/playbooks", {
      method: "POST",
      body: JSON.stringify(data),
    });
  },
  updatePlaybook(
    id: string,
    data: {
      title?: string;
      kind?: string;
      summary?: string;
      service_id?: string | null;
      label_selectors?: Array<{ key: string; op: string; value: string }>;
      tags?: string[];
    },
  ) {
    return request<StatusResponse>(`/api/v1/playbooks/${e(id)}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  },
  deletePlaybook(id: string) {
    return request<StatusResponse>(`/api/v1/playbooks/${e(id)}`, { method: "DELETE" });
  },
  addPlaybookStep(
    playbookId: string,
    data: { title: string; description?: string; expected_duration?: string; command?: string },
  ) {
    return request<PlaybookStepRecord>(`/api/v1/playbooks/${e(playbookId)}/steps`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  },
  updatePlaybookStep(
    playbookId: string,
    stepId: string,
    data: { title?: string; description?: string; expected_duration?: string; command?: string },
  ) {
    return request<StatusResponse>(`/api/v1/playbooks/${e(playbookId)}/steps/${e(stepId)}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  },
  deletePlaybookStep(playbookId: string, stepId: string) {
    return request<StatusResponse>(`/api/v1/playbooks/${e(playbookId)}/steps/${e(stepId)}`, {
      method: "DELETE",
    });
  },
  reorderPlaybookSteps(playbookId: string, order: Array<{ id: string; step_number: number }>) {
    return request<StatusResponse>(`/api/v1/playbooks/${e(playbookId)}/steps/reorder`, {
      method: "POST",
      body: JSON.stringify(order),
    });
  },

  // Pager Load Metrics
  getPagerLoadMetrics(params: {
    schedule_id: string;
    start_date: string;
    end_date: string;
    group_by?: string;
  }) {
    return request<PagerLoadData>(
      `/api/v1/on-call/metrics${buildQuery({
        schedule_id: params.schedule_id,
        start_date: params.start_date,
        end_date: params.end_date,
        group_by: params.group_by,
      })}`,
    );
  },

  // Post-Mortems & Action Items
  getPostMortem(incidentNumber: string | number) {
    return request<PostMortemRecord>(`/api/v1/incidents/${e(incidentNumber)}/post-mortem`);
  },
  createPostMortem(incidentNumber: string | number, data: Partial<PostMortemRecord>) {
    return request<PostMortemRecord>(`/api/v1/incidents/${e(incidentNumber)}/post-mortem`, {
      method: "POST",
      body: JSON.stringify(data),
    });
  },
  updatePostMortem(incidentNumber: string | number, data: Partial<PostMortemRecord>) {
    return request<PostMortemRecord>(`/api/v1/incidents/${e(incidentNumber)}/post-mortem`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  },
  submitPostMortemForReview(incidentNumber: string | number) {
    return request<PostMortemRecord>(
      `/api/v1/incidents/${e(incidentNumber)}/post-mortem/submit-review`,
      {
        method: "POST",
      },
    );
  },
  approvePostMortem(incidentNumber: string | number) {
    return request<PostMortemRecord>(`/api/v1/incidents/${e(incidentNumber)}/post-mortem/approve`, {
      method: "POST",
    });
  },
  publishPostMortem(incidentNumber: string | number) {
    return request<PostMortemRecord>(`/api/v1/incidents/${e(incidentNumber)}/post-mortem/publish`, {
      method: "POST",
    });
  },

  getActionItems(incidentNumber: string | number) {
    return request<ActionItemRecord[]>(
      `/api/v1/incidents/${e(incidentNumber)}/post-mortem/action-items`,
    );
  },
  createActionItem(incidentNumber: string | number, data: Partial<ActionItemRecord>) {
    return request<ActionItemRecord>(
      `/api/v1/incidents/${e(incidentNumber)}/post-mortem/action-items`,
      {
        method: "POST",
        body: JSON.stringify(data),
      },
    );
  },
  updateActionItem(
    incidentNumber: string | number,
    itemId: string,
    data: Partial<ActionItemRecord>,
  ) {
    return request<ActionItemRecord>(
      `/api/v1/incidents/${e(incidentNumber)}/post-mortem/action-items/${e(itemId)}`,
      {
        method: "PATCH",
        body: JSON.stringify(data),
      },
    );
  },
  deleteActionItem(incidentNumber: string | number, itemId: string) {
    return request<StatusResponse>(
      `/api/v1/incidents/${e(incidentNumber)}/post-mortem/action-items/${e(itemId)}`,
      {
        method: "DELETE",
      },
    );
  },
  getOpenActionItems() {
    return request<ActionItemRecord[]>("/api/v1/action-items");
  },

  listPostMortems(params: ListPostMortemsParams = {}) {
    return request<{ items: PostMortemRecord[]; total: number }>(
      `/api/v1/post-mortems${buildQuery(params)}`,
    );
  },
  deletePostMortem(incidentNumber: string | number) {
    return request<StatusResponse>(`/api/v1/incidents/${e(incidentNumber)}/post-mortem`, {
      method: "DELETE",
    });
  },

  // Notification Preferences
  getNotificationPreferences() {
    return request<NotificationPreferences>("/api/v1/users/me/notification-preferences");
  },
  updateNotificationPreferences(prefs: NotificationPreferences) {
    return request<StatusResponse>("/api/v1/users/me/notification-preferences", {
      method: "PUT",
      body: JSON.stringify(prefs),
    });
  },
  sendTestNotification() {
    return request<void>("/api/v1/users/me/notification-preferences/test", {
      method: "POST",
    });
  },

  // OIDC SSO Providers (admin)
  listOIDCProviders() {
    return unwrapItems<OIDCProviderRecord>(
      request<{ items: OIDCProviderRecord[]; total: number }>("/api/v1/oidc/providers"),
    );
  },
  createOIDCProvider(data: {
    name: string;
    issuer: string;
    client_id: string;
    client_secret: string;
    scopes?: string[];
    enabled?: boolean;
  }) {
    return request<OIDCProviderRecord>("/api/v1/oidc/providers", {
      method: "POST",
      body: JSON.stringify(data),
    });
  },
  updateOIDCProvider(
    id: string,
    data: {
      name?: string;
      issuer?: string;
      client_id?: string;
      client_secret?: string;
      scopes?: string[];
      enabled?: boolean;
    },
  ) {
    return request<OIDCProviderRecord>(`/api/v1/oidc/providers/${e(id)}`, {
      method: "PUT",
      body: JSON.stringify(data),
    });
  },
  deleteOIDCProvider(id: string) {
    return request<StatusResponse>(`/api/v1/oidc/providers/${e(id)}`, { method: "DELETE" });
  },

  // OIDC SSO (public — login page)
  listPublicOIDCProviders() {
    return request<OIDCProviderPublic[]>("/api/v1/auth/oidc/providers");
  },

  // Credential providers (admin)
  listCredentialProviders(params?: { type?: string; q?: string; enabled?: boolean }) {
    return unwrapItems<CredentialProviderRecord>(
      request<{ items: CredentialProviderRecord[]; total: number }>(
        `/api/v1/credential-providers${buildQuery({
          type: params?.type,
          q: params?.q,
          enabled: params?.enabled,
        })}`,
      ),
    );
  },
  getCredentialProvider(id: string) {
    return request<CredentialProviderRecord>(`/api/v1/credential-providers/${e(id)}`);
  },
  createCredentialProvider(data: {
    name: string;
    type: CredentialProviderType;
    enabled?: boolean;
    config?: Record<string, string>;
  }) {
    return request<CredentialProviderRecord>("/api/v1/credential-providers", {
      method: "POST",
      body: JSON.stringify(data),
    });
  },
  updateCredentialProvider(
    id: string,
    data: {
      name?: string;
      type?: CredentialProviderType;
      enabled?: boolean;
      config?: Record<string, string>;
    },
  ) {
    return request<CredentialProviderRecord>(`/api/v1/credential-providers/${e(id)}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  },
  deleteCredentialProvider(id: string) {
    return request<StatusResponse>(`/api/v1/credential-providers/${e(id)}`, {
      method: "DELETE",
    });
  },

  // Shared secrets (admin)
  listSharedSecrets(params?: { provider_id?: string; q?: string; limit?: number }) {
    return unwrapItems<SharedSecretRecord>(
      request<{ items: SharedSecretRecord[]; total: number }>(
        `/api/v1/shared-secrets${buildQuery({
          provider_id: params?.provider_id,
          q: params?.q,
          limit: params?.limit,
        })}`,
      ),
    );
  },
  createSharedSecret(data: {
    provider_id: string;
    name: string;
    description?: string;
    remote_ref?: string;
    value?: string;
    allowed_agent_ids?: string[];
  }) {
    return request<SharedSecretRecord>("/api/v1/shared-secrets", {
      method: "POST",
      body: JSON.stringify(data),
    });
  },
  updateSharedSecret(
    id: string,
    data: {
      name?: string;
      description?: string;
      remote_ref?: string;
      value?: string;
      allowed_agent_ids?: string[];
    },
  ) {
    return request<SharedSecretRecord>(`/api/v1/shared-secrets/${e(id)}`, {
      method: "PATCH",
      body: JSON.stringify(data),
    });
  },
  deleteSharedSecret(id: string) {
    return request<StatusResponse>(`/api/v1/shared-secrets/${e(id)}`, {
      method: "DELETE",
    });
  },
};
