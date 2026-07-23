package ics

import "alga/capability"

type RoleType string

const (
	RoleIncidentCommander  RoleType = "incident_commander"
	RoleCommunicationsLead RoleType = "communications_lead"
	RoleResponder          RoleType = "responder"
)

func ValidRoleType(rt RoleType) bool {
	switch rt {
	case RoleIncidentCommander, RoleCommunicationsLead, RoleResponder:
		return true
	}
	return false
}

func AssignableRoleType(rt RoleType) bool {
	return rt == RoleIncidentCommander || rt == RoleCommunicationsLead || rt == RoleResponder
}

func RoleRequiredCapability(rt RoleType) string {
	switch rt {
	case RoleIncidentCommander:
		return capability.Command
	case RoleCommunicationsLead:
		return capability.Communicate
	case RoleResponder:
		return capability.Investigate
	default:
		return ""
	}
}

func RoleLabel(rt RoleType) string {
	switch rt {
	case RoleIncidentCommander:
		return "Incident Commander"
	case RoleCommunicationsLead:
		return "Communicator"
	case RoleResponder:
		return "Responder"
	default:
		return string(rt)
	}
}

func RoleResponsibilityPrompt(rt RoleType) string {
	switch rt {
	case RoleIncidentCommander:
		return "owns incident direction, escalation decisions, final calls, and documentation quality"
	case RoleCommunicationsLead:
		return "owns status updates, stakeholder communication, summaries, and human-facing messages"
	case RoleResponder:
		return "owns investigation, mitigation, evidence gathering, and technical recovery work"
	default:
		return string(rt)
	}
}

type RoleStatus string

const (
	RoleStatusActive RoleStatus = "active"
	RoleStatusEnded  RoleStatus = "ended"
)

type EndReason string

const (
	EndReasonReplaced         EndReason = "replaced"
	EndReasonIncidentResolved EndReason = "incident_resolved"
	EndReasonAssigned         EndReason = "assigned"
	EndReasonAgentOffline     EndReason = "agent_offline"
)

type DocumentSection string

const (
	SectionImpactAssessment DocumentSection = "impact_assessment"
	SectionCurrentStatus    DocumentSection = "current_status"
	SectionActionsTaken     DocumentSection = "actions_taken"
	SectionOpenQuestions    DocumentSection = "open_questions"
	SectionResources        DocumentSection = "resources"
	SectionTimelineSummary  DocumentSection = "timeline_summary"
	SectionRootCause        DocumentSection = "root_cause"
	SectionResolution       DocumentSection = "resolution"
)

func ValidDocumentSection(s DocumentSection) bool {
	switch s {
	case SectionImpactAssessment, SectionCurrentStatus, SectionActionsTaken,
		SectionOpenQuestions, SectionResources, SectionTimelineSummary,
		SectionRootCause, SectionResolution:
		return true
	}
	return false
}

type ICSEventType string

const (
	ICSEventRoleAssigned     ICSEventType = "role_assigned"
	ICSEventRoleEnded        ICSEventType = "role_ended"
	ICSEventDocumentUpdated  ICSEventType = "document_updated"
	ICSEventWarRoomCreated   ICSEventType = "war_room_created"
	ICSEventTriageStarted    ICSEventType = "triage_started"
	ICSEventIncidentPromoted ICSEventType = "incident_promoted"
)

type WarRoomProvider string

const (
	WarRoomProviderSlack      WarRoomProvider = "slack"
	WarRoomProviderMattermost WarRoomProvider = "mattermost"
)
