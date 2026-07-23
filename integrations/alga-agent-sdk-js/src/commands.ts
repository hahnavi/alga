export interface InvestigationCommand {
  op: string;
  chat_id?: string;
  fingerprint?: string;
  severity?: string;
  reason?: string;
  root_cause?: string;
  resolution?: string;
  triage_result_id?: string;
  agreed?: boolean;
  correct_decision?: string;
  correct_severity?: string;
  note?: string;
  target_agent_id?: string;
  incident_id?: string;
  priority?: string;
  title?: string;
  role_type?: string;
  user_id?: string;
  agent_token_id?: string;
  scope_description?: string;
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
  const cmd: InvestigationCommand = { op: "triage_feedback", triage_result_id: triageResultId, agreed };
  if (correctDecision !== undefined) cmd.correct_decision = correctDecision;
  if (correctSeverity !== undefined) cmd.correct_severity = correctSeverity;
  if (note !== undefined) cmd.note = note;
  return cmd;
}

export function assignInvestigation(targetAgentId: string): InvestigationCommand {
  return { op: "assign_investigation", target_agent_id: targetAgentId };
}

export function promoteToIncident(title?: string, severity?: string, priority?: string): InvestigationCommand {
  return { op: "promote_to_incident", title, severity, priority };
}

// --- Incident tools ---

export function setIncidentPriority(incidentId: string, priority: string): InvestigationCommand {
  return { op: "set_incident_priority", incident_id: incidentId, priority };
}

export function setIncidentSeverity(incidentId: string, severity: string): InvestigationCommand {
  return { op: "set_incident_severity", incident_id: incidentId, severity };
}

export function triggerEscalation(incidentId: string): InvestigationCommand {
  return { op: "trigger_escalation", incident_id: incidentId };
}

export function requestStatusUpdate(incidentId: string): InvestigationCommand {
  return { op: "request_status_update", incident_id: incidentId };
}

export function mitigateIncident(incidentId: string, reason?: string): InvestigationCommand {
  const cmd: InvestigationCommand = { op: "mitigate_incident", incident_id: incidentId };
  if (reason !== undefined) cmd.reason = reason;
  return cmd;
}

export function resolveIncident(incidentId: string, reason?: string): InvestigationCommand {
  const cmd: InvestigationCommand = { op: "resolve_incident", incident_id: incidentId };
  if (reason !== undefined) cmd.reason = reason;
  return cmd;
}

export function beginTriage(incidentId: string): InvestigationCommand {
  return { op: "begin_triage", incident_id: incidentId };
}

export function promoteIncident(incidentId: string): InvestigationCommand {
  return { op: "promote_incident", incident_id: incidentId };
}

export function assignIncidentRole(opts: {
  incidentId: string;
  roleType: string;
  userId?: string;
  agentTokenId?: string;
  scopeDescription?: string;
}): InvestigationCommand {
  const cmd: InvestigationCommand = {
    op: "assign_incident_role",
    incident_id: opts.incidentId,
    role_type: opts.roleType,
  };
  if (opts.userId !== undefined) cmd.user_id = opts.userId;
  if (opts.agentTokenId !== undefined) cmd.agent_token_id = opts.agentTokenId;
  if (opts.scopeDescription !== undefined) cmd.scope_description = opts.scopeDescription;
  return cmd;
}
