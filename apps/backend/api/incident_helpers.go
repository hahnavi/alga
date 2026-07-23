// Code moved from http.go; see git history.

package api

import (
	"net/http"

	"alga/ics"
	"alga/logger"
	"alga/oncall"
	"alga/sse"
	"alga/store"
)

func (s *Server) autoAssignICOnPromote(r *http.Request, inc *store.IncidentRecord) {
	onCallUserID, err := oncall.ResolveOnCallUserForIncident(r.Context(), inc, s.serviceStore, s.escalationStore, s.onCallStore, s.onCallResolver)
	if err != nil {
		logger.WarnCtx(r.Context(), "Failed to resolve on-call user for IC assignment", "incident_number", inc.IncidentNumber, "error", err)
		return
	}
	if onCallUserID == nil {
		logger.InfoCtx(r.Context(), "Skipping IC auto-assignment: no on-call user resolved", "incident_number", inc.IncidentNumber)
		return
	}
	if s.icsRoleStore == nil {
		logger.InfoCtx(r.Context(), "Skipping IC auto-assignment: ICS role store not configured", "incident_number", inc.IncidentNumber)
		return
	}
	if _, err := s.icsRoleStore.AssignRole(r.Context(), inc.IncidentNumber, ics.RoleIncidentCommander, *onCallUserID, nil, nil); err != nil {
		logger.WarnCtx(r.Context(), "Failed to auto-assign IC on promote", "incident_number", inc.IncidentNumber, "user_id", onCallUserID, "error", err)
		return
	}
	_ = s.incidentStore.AddTimelineEntry(r.Context(), &store.IncidentTimelineEntryRecord{
		IncidentNumber: inc.IncidentNumber,
		EventType:      "ics_role_assigned",
		ActorType:      "system",
		Message:        "Auto-assigned Incident Commander (on-call)",
	})
	inc.OnCallResponderID = onCallUserID
	updated, patchErr := s.incidentStore.UpdateIncident(r.Context(), inc.IncidentNumber, inc)
	if patchErr != nil {
		logger.WarnCtx(r.Context(), "Failed to set on_call_responder_id on incident", "incident_number", inc.IncidentNumber, "user_id", onCallUserID, "error", patchErr)
	} else if updated != nil {
		*inc = *updated
	}
	logger.InfoCtx(r.Context(), "Auto-assigned IC from on-call", "incident_number", inc.IncidentNumber, "user_id", onCallUserID)
}

func (s *Server) forwardInvestigationSignal(agentID, investigationID, signalType, reason, actor string) {
	if s.investigationForwarder == nil || agentID == "" {
		return
	}
	event := sse.Event{
		Type: signalType,
		Data: map[string]any{
			"investigation_id": investigationID,
			"reason":           reason,
			"actor":            actor,
		},
	}
	if err := s.investigationForwarder.ForwardEventToAgent(agentID, event); err != nil {
		logger.Error("failed to forward signal to agent for investigation", "signal_type", signalType, "investigation_id", investigationID, "error", err)
	}
}
