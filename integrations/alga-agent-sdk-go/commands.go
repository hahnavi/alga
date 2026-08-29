package alga

import "time"

// InvestigationCommand is the wire payload of a `kind: "inv_tool"` agent
// message. It maps 1:1 to the backend InvTool struct
// (apps/backend/api/agent/agent_tools.go) so the SDK can drive every command
// the backend supports. Fields are omitempty to keep payloads small.
type InvestigationCommand struct {
	Op string `json:"op"`
	// ChatID is the agent chat thread identifier. The backend grammar is
	// "alert_<number>" for alert investigations, "incident_coord_<number>"
	// for incident coordination threads, and "incident_inv_<number>" for
	// incident-scoped investigations. Most ops require this to be set so the
	// backend can authorize the caller against the underlying investigation.
	ChatID string `json:"chat_id,omitempty"`

	// --- Alert-scoped ---
	Fingerprint string `json:"fingerprint,omitempty"`

	// --- Generic mutation context ---
	Reason   string `json:"reason,omitempty"`
	Note     string `json:"note,omitempty"`
	Priority string `json:"priority,omitempty"`
	Severity string `json:"severity,omitempty"`
	Title    string `json:"title,omitempty"`

	// --- set_outcome ---
	RootCause        *string `json:"root_cause,omitempty"`
	Resolution       *string `json:"resolution,omitempty"`
	Summary          string  `json:"summary,omitempty"`
	ImpactAssessment string  `json:"impact_assessment,omitempty"`
	ActionsTaken     string  `json:"actions_taken,omitempty"`
	ETADetail        string  `json:"eta,omitempty"`

	// --- triage_feedback ---
	TriageResultID  string `json:"triage_result_id,omitempty"`
	Agreed          bool   `json:"agreed,omitempty"`
	CorrectDecision string `json:"correct_decision,omitempty"`
	CorrectSeverity string `json:"correct_severity,omitempty"`

	// --- assign_investigation / assign_incident_role ---
	TargetAgentID    string  `json:"target_agent_id,omitempty"`
	RoleType         string  `json:"role_type,omitempty"`
	UserID           string  `json:"user_id,omitempty"`
	AgentTokenID     string  `json:"agent_token_id,omitempty"`
	ScopeDescription *string `json:"scope_description,omitempty"`

	// --- Incident-scoped (require IncidentNumber) ---
	IncidentNumber int64 `json:"incident_number,omitempty"`

	// --- coordination handoff / status ---
	Message                     string `json:"message,omitempty"`
	Audience                    string `json:"audience,omitempty"`
	Urgency                     string `json:"urgency,omitempty"`
	StatusLevel                 string `json:"status_level,omitempty"`
	SourceCoordinationMessageID string `json:"source_coordination_message_id,omitempty"`
	Internal                    bool   `json:"internal,omitempty"`
}

// StrPtr is a convenience helper for the *string command fields.
func StrPtr(s string) *string { return &s }

// --- Alert investigation tools ---

func ResolveAlert(fingerprint string) InvestigationCommand {
	return InvestigationCommand{Op: "resolve_alert", Fingerprint: fingerprint}
}

func ReopenAlert(fingerprint string) InvestigationCommand {
	return InvestigationCommand{Op: "reopen_alert", Fingerprint: fingerprint}
}

func SetOutcome(rootCause, resolution *string) InvestigationCommand {
	return InvestigationCommand{Op: "set_outcome", RootCause: rootCause, Resolution: resolution}
}

func CancelInvestigation(reason string) InvestigationCommand {
	return InvestigationCommand{Op: "cancel_investigation", Reason: reason}
}

func PauseInvestigation(reason string) InvestigationCommand {
	return InvestigationCommand{Op: "pause_investigation", Reason: reason}
}

func TriageFeedback(triageResultID string, agreed bool, correctDecision, correctSeverity, note string) InvestigationCommand {
	return InvestigationCommand{
		Op:              "triage_feedback",
		TriageResultID:  triageResultID,
		Agreed:          agreed,
		CorrectDecision: correctDecision,
		CorrectSeverity: correctSeverity,
		Note:            note,
	}
}

func AssignInvestigation(targetAgentID string) InvestigationCommand {
	return InvestigationCommand{Op: "assign_investigation", TargetAgentID: targetAgentID}
}

func PromoteToIncident(title, severity, priority string) InvestigationCommand {
	return InvestigationCommand{Op: "promote_to_incident", Title: title, Severity: severity, Priority: priority}
}

// --- Incident tools ---

func SetIncidentPriority(incidentNumber int64, priority string) InvestigationCommand {
	return InvestigationCommand{Op: "set_incident_priority", IncidentNumber: incidentNumber, Priority: priority}
}

func SetIncidentSeverity(incidentNumber int64, severity string) InvestigationCommand {
	return InvestigationCommand{Op: "set_incident_severity", IncidentNumber: incidentNumber, Severity: severity}
}

func TriggerEscalation(incidentNumber int64) InvestigationCommand {
	return InvestigationCommand{Op: "trigger_escalation", IncidentNumber: incidentNumber}
}

func MitigateIncident(incidentNumber int64, reason string) InvestigationCommand {
	return InvestigationCommand{Op: "mitigate_incident", IncidentNumber: incidentNumber, Reason: reason}
}

// ResolveIncident is for commander-capable agents only. Investigator agents
// should record outcomes and ask the commander to verify resolution through
// coordination.
func ResolveIncident(incidentNumber int64, reason string) InvestigationCommand {
	return InvestigationCommand{Op: "resolve_incident", IncidentNumber: incidentNumber, Reason: reason}
}

func BeginTriage(incidentNumber int64) InvestigationCommand {
	return InvestigationCommand{Op: "begin_triage", IncidentNumber: incidentNumber}
}

func PromoteIncident(incidentNumber int64) InvestigationCommand {
	return InvestigationCommand{Op: "promote_incident", IncidentNumber: incidentNumber}
}

// AssignIncidentRoleToUser assigns an incident role (e.g. "commander",
// "communicator") to a human user. The backend requires exactly one assignee,
// so this sets user_id only. Pass scopeDescription "" to omit it.
func AssignIncidentRoleToUser(incidentNumber int64, roleType, userID, scopeDescription string) InvestigationCommand {
	cmd := InvestigationCommand{
		Op:             "assign_incident_role",
		IncidentNumber: incidentNumber,
		RoleType:       roleType,
		UserID:         userID,
	}
	if scopeDescription != "" {
		cmd.ScopeDescription = &scopeDescription
	}
	return cmd
}

// AssignIncidentRoleToAgent assigns an incident role to an agent token. The
// backend requires exactly one assignee, so this sets agent_token_id only.
// Pass scopeDescription "" to omit it.
func AssignIncidentRoleToAgent(incidentNumber int64, roleType, agentTokenID, scopeDescription string) InvestigationCommand {
	cmd := InvestigationCommand{
		Op:             "assign_incident_role",
		IncidentNumber: incidentNumber,
		RoleType:       roleType,
		AgentTokenID:   agentTokenID,
	}
	if scopeDescription != "" {
		cmd.ScopeDescription = &scopeDescription
	}
	return cmd
}

// --- Coordination / status tools ---

// PostHandoff posts a coordination handoff message. message, audience, and
// urgency are required by the backend. The backend accepts only the
// structured responder→commander handoff and urgent commander-attention
// calls; see the incident role prompts for the expected message format.
func PostHandoff(incidentNumber int64, message, audience, urgency string) InvestigationCommand {
	return InvestigationCommand{
		Op:             "post_handoff",
		IncidentNumber: incidentNumber,
		Message:        message,
		Audience:       audience,
		Urgency:        urgency,
	}
}

// PublishStatusUpdate publishes a status update. message and statusLevel are
// required. Status updates go to the Status Updates card and do NOT activate
// other agents — they are the safe channel for in-progress communication.
func PublishStatusUpdate(incidentNumber int64, message, statusLevel string) InvestigationCommand {
	return InvestigationCommand{
		Op:             "publish_status_update",
		IncidentNumber: incidentNumber,
		Message:        message,
		StatusLevel:    statusLevel,
	}
}

// SetIncidentResolutionDocs sets incident resolution documents. At least one
// of the optional fields must be provided.
func SetIncidentResolutionDocs(incidentNumber int64, summary, impactAssessment, actionsTaken, rootCause, resolution string) InvestigationCommand {
	cmd := InvestigationCommand{
		Op:               "set_incident_resolution_docs",
		IncidentNumber:   incidentNumber,
		Summary:          summary,
		ImpactAssessment: impactAssessment,
		ActionsTaken:     actionsTaken,
	}
	if rootCause != "" {
		cmd.RootCause = &rootCause
	}
	if resolution != "" {
		cmd.Resolution = &resolution
	}
	return cmd
}

// --- Time helper ---

// Now returns the current time. Wrapped as a package var so tests can stub
// deterministic timestamps (e.g. for idempotency-key generation).
var Now = func() time.Time { return time.Now() }
