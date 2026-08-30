import type { OpenClawConfig } from "openclaw/plugin-sdk/config-contracts";
import type { OutboundReplyPayload } from "openclaw/plugin-sdk/reply-payload";

export type CoreConfig = OpenClawConfig;

export type AlgaReplyPayload = OutboundReplyPayload & {
  channelData?: { alga?: AlgaChannelData };
};

export type AlgaAccountConfig = {
  name?: string;
  enabled?: boolean;
  serverUrl?: string;
  token?: string;
  allowFrom?: Array<string | number>;
  defaultTo?: string;
};

export type ResolvedAlgaAccount = {
  accountId: string;
  enabled: boolean;
  configured: boolean;
  name?: string;
  serverUrl: string;
  httpBase: string;
  token: string;
  tokenSource?: "config" | "env" | "none";
  tokenStatus?: "available" | "missing";
  config: AlgaAccountConfig & { allowFrom: Array<string | number> };
};

export type AlgaInvestigationCommandOp =
  | "resolve_alert"
  | "reopen_alert"
  | "set_outcome"
  | "cancel_investigation"
  | "pause_investigation"
  | "triage_feedback"
  | "promote_to_incident"
  | "assign_investigation"
  | "set_incident_priority"
  | "set_incident_severity"
  | "trigger_escalation"
  | "mitigate_incident"
  | "resolve_incident"
  | "begin_triage"
  | "promote_incident"
  | "assign_incident_role"
  | "post_handoff"
  | "publish_status_update"
  | "set_incident_resolution_docs";

export type AlgaInvestigationSeverity = "critical" | "high" | "warning" | "info";

export type AlgaInvestigationCommand = {
  op: AlgaInvestigationCommandOp;
  fingerprint?: string;
  severity?: AlgaInvestigationSeverity;
  root_cause?: string;
  resolution?: string;
  reason?: string;
  triage_result_id?: string;
  agreed?: boolean;
  correct_decision?: string;
  correct_severity?: string;
  note?: string;
  title?: string;
  priority?: string;
  target_agent_id?: string;
  incident_number?: number;
  role_type?: string;
  user_id?: string;
  agent_token_id?: string;
  scope_description?: string;
  message?: string;
  audience?: "none" | "commander" | "communicator" | "command";
  urgency?: "info" | "needs_attention" | "decision_needed";
  status_level?: "investigating" | "identified" | "mitigated" | "monitoring" | "resolved";
  source_coordination_message_id?: string;
  summary?: string;
  impact_assessment?: string;
  actions_taken?: string;
};

export type AlgaChannelData = {
  command?: AlgaInvestigationCommand;
};

export type AlgaOutboundMessagePayload =
  | { kind: "text"; text: string; sender_id?: string; sender_name?: string }
  | { kind: "tool_call"; text: string }
  | { kind: "inv_tool"; command: AlgaInvestigationCommand };

export type InvestigationSignalEventType = "investigation_resume" | "investigation_abort";

export type InvestigationSignalEvent = {
  investigation_id: string;
  reason?: string;
  actor?: string;
};
