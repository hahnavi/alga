// Package api: incident_timeline.go covers incident timeline handlers —
// listing timeline entries and adding a new manual entry.
package api

import (
	"net/http"
	"strings"

	"alga/rbac"
	"alga/store"
)

func (s *Server) handleListIncidentTimeline(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsRead) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}

	entries, err := s.incidentStore.GetTimeline(r.Context(), mustParseIncidentNumber(incidentID))
	if err != nil {
		writeInternalError(w, err, "failed to list timeline")
		return
	}
	writeData(w, http.StatusOK, ensureSlice(entries))
}

func (s *Server) handleAddIncidentTimelineEntry(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsWrite) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}

	var req struct {
		EventType string         `json:"event_type"`
		Message   string         `json:"message"`
		Metadata  map[string]any `json:"metadata,omitempty"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Message) == "" {
		writeError(w, ErrorCodeValidationFailed, "message is required")
		return
	}

	user := userFromContext(r.Context())
	actorID := ""
	if user != nil {
		actorID = user.ID.String()
	}

	entry := &store.IncidentTimelineEntryRecord{
		IncidentNumber: mustParseIncidentNumber(incidentID),
		EventType:      req.EventType,
		ActorID:        parseUUIDPtr(actorID),
		ActorType:      "user",
		Message:        req.Message,
		Metadata:       req.Metadata,
	}
	if err := s.incidentStore.AddTimelineEntry(r.Context(), entry); err != nil {
		writeInternalError(w, err, "failed to add timeline entry")
		return
	}
	s.audit(r, store.AuditIncidentUpdated, map[string]any{
		"timeline_entry":  true,
		"event_type":      req.EventType,
		"incident_number": entry.IncidentNumber,
	})
	writeData(w, http.StatusCreated, entry)
}
