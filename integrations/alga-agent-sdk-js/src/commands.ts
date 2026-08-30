// InvestigationCommand is the wire payload of a `kind: "inv_tool"` agent
// message. It maps 1:1 to the backend InvTool struct so the SDK can drive
// every command the backend supports. Fields are optional to keep payloads
// small. The incident identifier is `incident_number` (number), matching the
// backend contract — never `incident_id`.
export interface InvestigationCommand {
  op: string;
  // chat_id is the agent chat thread identifier. The backend grammar is
  // "alert_<number>", "incident_coord_<number>", "incident_inv_<number>".
  chat_id?: string;

  // --- Alert-scoped ---
  fingerprint?: string;

  // --- Generic mutation context ---
  reason?: string;
  note?: string;
  priority?: string;
  severity?: string;
  title?: string;

  // --- set_outcome / set_incident_resolution_docs / resolve_incident ---
  root_cause?: string;
  resolution?: string;
  summary?: string;
  impact_assessment?: string;
  actions_taken?: string;
  eta?: string;

  // --- triage_feedback ---
  triage_result_id?: string;
  agreed?: boolean;
  correct_decision?: string;
  correct_severity?: string;

  // --- assign_investigation / assign_incident_role ---
  target_agent_id?: string;
  role_type?: string;
  user_id?: string;
  agent_token_id?: string;
  scope_description?: string;

  // --- Incident-scoped (require incident_number) ---
  incident_number?: number;

  // --- coordination handoff / status ---
  message?: string;
  audience?: string;
  urgency?: string;
  status_level?: string;
  source_coordination_message_id?: string;
  internal?: boolean;
}

// --- Alert investigation tools ---

export function resolveAlert(fingerprint: string): InvestigationCommand {
  return { op: "resolve_alert", fingerprint };
}

export function reopenAlert(fingerprint: string): InvestigationCommand {
  return { op: "reopen_alert", fingerprint };
}

export function setOutcome(rootCause?: string, resolution?: string): InvestigationCommand {
  const cmd: InvestigationCommand = { op: "set_outcome" };
  if (rootCause !== undefined) cmd.root_cause = rootCause;
  if (resolution !== undefined) cmd.resolution = resolution;
  return cmd;
}

export function cancelInvestigation(reason?: string): InvestigationCommand {
  const cmd: InvestigationCommand = { op: "cancel_investigation" };
  if (reason !== undefined) cmd.reason = reason;
  return cmd;
}

export function pauseInvestigation(reason?: string): InvestigationCommand {
  const cmd: InvestigationCommand = { op: "pause_investigation" };
  if (reason !== undefined) cmd.reason = reason;
  return cmd;
}

export function triageFeedback(
  triageResultId: string,
  agreed = true,
  correctDecision?: string,
  correctSeverity?: string,
  note?: string,
): InvestigationCommand {
  const cmd: InvestigationCommand = {
    op: "triage_feedback",
    triage_result_id: triageResultId,
    agreed,
  };
  if (correctDecision !== undefined) cmd.correct_decision = correctDecision;
  if (correctSeverity !== undefined) cmd.correct_severity = correctSeverity;
  if (note !== undefined) cmd.note = note;
  return cmd;
}

export function assignInvestigation(targetAgentId: string): InvestigationCommand {
  return { op: "assign_investigation", target_agent_id: targetAgentId };
}

export function promoteToIncident(
  title?: string,
  severity?: string,
  priority?: string,
): InvestigationCommand {
  const cmd: InvestigationCommand = { op: "promote_to_incident" };
  if (title !== undefined) cmd.title = title;
  if (severity !== undefined) cmd.severity = severity;
  if (priority !== undefined) cmd.priority = priority;
  return cmd;
}

// --- Incident tools ---

export function setIncidentPriority(
  incidentNumber: number,
  priority: string,
): InvestigationCommand {
  return { op: "set_incident_priority", incident_number: incidentNumber, priority };
}

export function setIncidentSeverity(
  incidentNumber: number,
  severity: string,
): InvestigationCommand {
  return { op: "set_incident_severity", incident_number: incidentNumber, severity };
}

export function triggerEscalation(incidentNumber: number): InvestigationCommand {
  return { op: "trigger_escalation", incident_number: incidentNumber };
}

export function mitigateIncident(incidentNumber: number, reason?: string): InvestigationCommand {
  const cmd: InvestigationCommand = { op: "mitigate_incident", incident_number: incidentNumber };
  if (reason !== undefined) cmd.reason = reason;
  return cmd;
}

// resolveIncident is for commander-capable agents only. Investigator agents
// should record outcomes and ask the commander to verify resolution.
export function resolveIncident(incidentNumber: number, reason?: string): InvestigationCommand {
  const cmd: InvestigationCommand = { op: "resolve_incident", incident_number: incidentNumber };
  if (reason !== undefined) cmd.reason = reason;
  return cmd;
}

export function beginTriage(incidentNumber: number): InvestigationCommand {
  return { op: "begin_triage", incident_number: incidentNumber };
}

export function promoteIncident(incidentNumber: number): InvestigationCommand {
  return { op: "promote_incident", incident_number: incidentNumber };
}

// Assigns an incident role (e.g. "commander", "communicator") to a human user.
// The backend requires exactly one assignee, so this sets user_id only.
export function assignIncidentRoleToUser(
  incidentNumber: number,
  roleType: string,
  userId: string,
  scopeDescription?: string,
): InvestigationCommand {
  const cmd: InvestigationCommand = {
    op: "assign_incident_role",
    incident_number: incidentNumber,
    role_type: roleType,
    user_id: userId,
  };
  if (scopeDescription !== undefined) cmd.scope_description = scopeDescription;
  return cmd;
}

// Assigns an incident role to an agent token. The backend requires exactly one
// assignee, so this sets agent_token_id only.
export function assignIncidentRoleToAgent(
  incidentNumber: number,
  roleType: string,
  agentTokenId: string,
  scopeDescription?: string,
): InvestigationCommand {
  const cmd: InvestigationCommand = {
    op: "assign_incident_role",
    incident_number: incidentNumber,
    role_type: roleType,
    agent_token_id: agentTokenId,
  };
  if (scopeDescription !== undefined) cmd.scope_description = scopeDescription;
  return cmd;
}

// --- Coordination / status tools ---

// PostHandoff posts a coordination handoff message. Deprecated in favor of
// DispatchTask + CompleteTask — every handoff activates the commander and
// communicator and triggers ping-pong loops. Use CompleteTask to hand work
// back to the commander.
export function postHandoff(
  incidentNumber: number,
  message: string,
  audience: string,
  urgency: string,
): InvestigationCommand {
  return { op: "post_handoff", incident_number: incidentNumber, message, audience, urgency };
}

// PublishStatusUpdate publishes a status update. message and statusLevel are
// required. Status updates go to the Status Updates card and do NOT activate
// other agents — they are the safe channel for in-progress communication.
export function publishStatusUpdate(
  incidentNumber: number,
  message: string,
  statusLevel: string,
): InvestigationCommand {
  return {
    op: "publish_status_update",
    incident_number: incidentNumber,
    message,
    status_level: statusLevel,
  };
}

// SetIncidentResolutionDocs sets incident resolution documents. At least one
// of the optional fields must be provided.
export function setIncidentResolutionDocs(
  incidentNumber: number,
  opts: {
    summary?: string;
    impactAssessment?: string;
    actionsTaken?: string;
    rootCause?: string;
    resolution?: string;
  },
): InvestigationCommand {
  const cmd: InvestigationCommand = {
    op: "set_incident_resolution_docs",
    incident_number: incidentNumber,
  };
  if (opts.summary !== undefined) cmd.summary = opts.summary;
  if (opts.impactAssessment !== undefined) cmd.impact_assessment = opts.impactAssessment;
  if (opts.actionsTaken !== undefined) cmd.actions_taken = opts.actionsTaken;
  if (opts.rootCause !== undefined) cmd.root_cause = opts.rootCause;
  if (opts.resolution !== undefined) cmd.resolution = opts.resolution;
  return cmd;
}
