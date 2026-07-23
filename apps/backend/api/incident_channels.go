// Package api: incident_channels.go covers incident chat channel handlers —
// Slack incident channel create/delete plus auto-creation, and Google Meet
// war-room create/unlink.
package api

import (
	"net/http"

	"alga/logger"
	"alga/rbac"
	"alga/store"
)

func (s *Server) handleCreateSlackChannel(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}
	if s.incidentChannelManager == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "slack incident channels not configured")
		return
	}

	record, ok := s.getIncidentOrError(w, r, incidentID)
	if !ok {
		return
	}

	if record.SlackChannelID != "" {
		writeError(w, ErrorCodeConflict, "slack channel already exists for this incident")
		return
	}

	isPrivate := true
	s.mu.RLock()
	if s.cfg.SlackIncidentChannelVisibility == "public" {
		isPrivate = false
	}
	s.mu.RUnlock()

	if err := s.incidentChannelManager.CreateIncidentChannel(r.Context(), record, isPrivate); err != nil {
		writeInternalError(w, err, "failed to create slack channel")
		return
	}

	updated, _ := s.incidentStore.GetIncident(r.Context(), mustParseIncidentNumber(incidentID))
	s.publishIncidentEvent("incident_updated", updated)
	s.audit(r, store.AuditIncidentSlackChannelCreated, map[string]any{
		"incident_number": incidentID,
		"channel_id":      record.SlackChannelID,
	})
	writeData(w, http.StatusOK, updated)
}

func (s *Server) handleDeleteSlackChannel(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}

	record, ok := s.getIncidentOrError(w, r, incidentID)
	if !ok {
		return
	}

	if record.SlackChannelID == "" {
		writeError(w, ErrorCodeNotFound, "no slack channel linked to this incident")
		return
	}

	record.SlackChannelID = ""
	record.SlackChannelName = ""
	record.SlackChannelArchived = false
	if _, err := s.incidentStore.UpdateIncident(r.Context(), mustParseIncidentNumber(incidentID), record); err != nil {
		writeInternalError(w, err, "failed to unlink slack channel")
		return
	}

	updated, _ := s.incidentStore.GetIncident(r.Context(), mustParseIncidentNumber(incidentID))
	s.publishIncidentEvent("incident_updated", updated)
	s.audit(r, store.AuditIncidentSlackChannelUnlinked, map[string]any{
		"incident_number": incidentID,
	})
	writeData(w, http.StatusOK, updated)
}

func (s *Server) handleCreateGoogleMeet(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}
	if s.googleMeetClient == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "google meet is not configured")
		return
	}

	record, ok := s.getIncidentOrError(w, r, incidentID)
	if !ok {
		return
	}

	if record.GoogleMeetSpaceName != "" {
		writeError(w, ErrorCodeConflict, "google meet space already exists for this incident")
		return
	}

	space, err := s.googleMeetClient.CreateSpace(r.Context())
	if err != nil {
		writeInternalError(w, err, "failed to create google meet space")
		return
	}

	if err := s.incidentStore.SetIncidentWarRoomMeet(r.Context(), record.IncidentNumber, space.SpaceName, space.MeetingURI); err != nil {
		writeInternalError(w, err, "failed to persist google meet space")
		return
	}

	updated, _ := s.incidentStore.GetIncident(r.Context(), record.IncidentNumber)
	s.publishIncidentEvent("incident_updated", updated)
	s.audit(r, store.AuditIncidentGoogleMeetCreated, map[string]any{
		"incident_number":   incidentID,
		"google_meet_space": space.SpaceName,
	})
	writeData(w, http.StatusOK, updated)
}

func (s *Server) handleUnlinkGoogleMeet(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}

	record, ok := s.getIncidentOrError(w, r, incidentID)
	if !ok {
		return
	}

	if record.GoogleMeetSpaceName == "" {
		writeError(w, ErrorCodeNotFound, "no google meet space linked to this incident")
		return
	}

	if err := s.incidentStore.SetIncidentWarRoomMeet(r.Context(), record.IncidentNumber, "", ""); err != nil {
		writeInternalError(w, err, "failed to unlink google meet space")
		return
	}

	updated, _ := s.incidentStore.GetIncident(r.Context(), record.IncidentNumber)
	s.publishIncidentEvent("incident_updated", updated)
	s.audit(r, store.AuditIncidentGoogleMeetUnlinked, map[string]any{
		"incident_number": incidentID,
	})
	writeData(w, http.StatusOK, updated)
}

func (s *Server) tryAutoCreateSlackChannel(r *http.Request, incident *store.IncidentRecord) {
	if s.incidentChannelManager == nil || !s.incidentChannelManager.IsSupported() {
		return
	}

	s.mu.RLock()
	enabled := s.cfg.SlackIncidentChannelsEnabled
	triggerStatus := s.cfg.SlackIncidentChannelTriggerStatus
	s.mu.RUnlock()

	if !enabled || incident.Status != triggerStatus || incident.SlackChannelID != "" {
		return
	}

	isPrivate := true
	s.mu.RLock()
	if s.cfg.SlackIncidentChannelVisibility == "public" {
		isPrivate = false
	}
	s.mu.RUnlock()

	if err := s.incidentChannelManager.CreateIncidentChannel(r.Context(), incident, isPrivate); err != nil {
		logger.WarnCtx(r.Context(), "auto-create slack incident channel failed", "error", err)
	}
}
