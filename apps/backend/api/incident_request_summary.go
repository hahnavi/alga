// Package api: incident_request_summary.go covers the operator-initiated
// "request summary" endpoint that dispatches a summarize task to an assigned
// agent, plus the auto post-mortem draft creation helper.
//
// Note: this is intentionally a separate file from incident_summary.go, which
// holds handleIncidentSummaryFromAgent (the agent→incident summary ingestion
// path) — a distinct responsibility.
package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"alga/logger"
	"alga/rbac"
	"alga/sse"
	"alga/store"
)

func (s *Server) ensurePostMortemDraft(ctx context.Context, incident *store.IncidentRecord, summary string) {
	if incident == nil || s.postmortemStore == nil || s.incidentStore == nil {
		return
	}
	existing, err := s.postmortemStore.GetByIncidentID(ctx, incident.ID)
	if err != nil {
		logger.WarnCtx(ctx, "failed to check post-mortem", "incident_number", incident.IncidentNumber, "error", err)
		return
	}
	if existing != nil {
		return
	}
	s.mu.RLock()
	alertStore := s.alertStore
	coordStore := s.incidentCoordinationStore
	docStore := s.incidentDocumentStore
	s.mu.RUnlock()

	draft := buildPostMortemDraft(ctx, postMortemDraftDeps{
		documentStore:     docStore,
		coordinationStore: coordStore,
		incidentStore:     s.incidentStore,
		alertStore:        alertStore,
	}, incident, summary)

	_, err = s.postmortemStore.Create(ctx, draft)
	if err != nil {
		logger.WarnCtx(ctx, "failed to auto-create post-mortem", "incident_number", incident.IncidentNumber, "error", err)
		return
	}
	_ = s.incidentStore.AddTimelineEntry(ctx, &store.IncidentTimelineEntryRecord{
		IncidentNumber: incident.IncidentNumber,
		EventType:      "postmortem_created",
		ActorType:      "system",
		Message:        "Post-mortem created",
	})
}

func (s *Server) handleRequestSummary(w http.ResponseWriter, r *http.Request, incidentID string) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	if s.incidentStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "incident store not configured")
		return
	}
	if s.investigationForwarder == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "agent forwarding not configured")
		return
	}

	ctx := r.Context()
	inc, err := s.incidentStore.GetIncident(ctx, mustParseIncidentNumber(incidentID))
	if err != nil {
		writeInternalError(w, err, "failed to get incident")
		return
	}
	if inc == nil {
		writeError(w, ErrorCodeNotFound, "incident not found")
		return
	}

	activeStatuses := map[string]bool{"detected": true, "active": true, "mitigated": true}
	if !activeStatuses[inc.Status] {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, fmt.Sprintf("cannot request summary for incident in %s status", inc.Status))
		return
	}
	if inc.SlackChannelID == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "incident has no Slack channel")
		return
	}

	invs, err := s.incidentInvestigationStore.ListIncidentInvestigationsByIncident(ctx, mustParseIncidentNumber(incidentID))
	if err != nil {
		writeInternalError(w, err, "failed to list investigations")
		return
	}

	var agentIDHex, agentName string
	for _, inv := range invs {
		if inv.AgentID != "" && (inv.Status == "investigating" || inv.Status == "pending" || inv.Status == "assigned") {
			agentIDHex = inv.AgentID
			agentName = inv.AgentName
			break
		}
	}
	if agentIDHex == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "no assigned agent found for this incident")
		return
	}

	if !s.investigationForwarder.AgentOnline(agentIDHex) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "assigned agent is currently offline")
		return
	}

	durationMinutes := int(time.Since(inc.CreatedAt).Minutes())
	evt := sse.Event{
		Type: "summarize_incident",
		Data: map[string]any{
			"incident_number": inc.IncidentNumber,
			"chat_id":         "incident_coord_" + strconv.FormatInt(inc.IncidentNumber, 10),
			"incident": map[string]any{
				"title":                inc.Title,
				"severity":             inc.Severity,
				"status":               inc.Status,
				"duration_minutes":     durationMinutes,
				"timeline_entry_count": len(inc.Timeline),
			},
		},
	}

	if err := s.investigationForwarder.ForwardEventToAgent(agentIDHex, evt); err != nil {
		writeInternalError(w, err, "failed to dispatch summary request to agent")
		return
	}

	if s.vkClient != nil {
		s.vkClient.Do(ctx, s.vkClient.Builder().Set().
			Key("alga:summary:pending:"+strconv.FormatInt(inc.IncidentNumber, 10)).
			Value("1").
			ExSeconds(int64(30*time.Minute/time.Second)).
			Build())
	}

	s.audit(r, store.AuditIncidentUpdated, map[string]any{
		"incident_number": incidentID,
		"action":          "summary_requested",
		"agent_id":        agentIDHex,
		"agent_name":      agentName,
	})

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":          "ok",
		"incident_number": incidentID,
	})
}
