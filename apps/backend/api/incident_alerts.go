// Package api: incident_alerts.go covers alert↔incident linkage handlers
// (list, link, unlink), the alert→incident thread handoff message, and the
// incident-scoped investigation auto-ensure helper.
package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"alga/logger"
	"alga/rbac"
	"alga/store"
)

func (s *Server) postAlertIncidentHandoffMessage(ctx context.Context, alertNumber int64, incidentID string) {
	if s.investigationThreadStore == nil || alertNumber <= 0 || incidentID == "" {
		return
	}
	ownerID := strconv.FormatInt(alertNumber, 10)
	thread, err := s.investigationThreadStore.EnsureThread(ctx, store.ThreadOwnerAlert, ownerID)
	if err != nil {
		logger.WarnCtx(ctx, "failed to ensure alert thread for incident handoff", "alert_number", alertNumber, "incident_number", incidentID, "error", err)
		return
	}
	message := fmt.Sprintf("🚨 Alert promoted to incident [**%s**](/incidents/%s). Continue investigation and command decisions in the incident.", incidentID, incidentID)
	if _, err := s.investigationThreadStore.AddMessage(ctx, thread.ThreadID, store.InvestigationThreadMessage{
		Type:     "action",
		Source:   "system",
		Message:  message,
		Username: "System",
	}); err != nil {
		logger.WarnCtx(ctx, "failed to post alert thread incident handoff", "alert_number", alertNumber, "incident_number", incidentID, "error", err)
	}
}

func (s *Server) ensureIncidentInvestigation(ctx context.Context, incident *store.IncidentRecord) {
	if incident == nil || s.incidentInvestigationStore == nil || s.incidentStore == nil {
		return
	}
	invs, err := s.incidentInvestigationStore.ListIncidentInvestigationsByIncident(ctx, incident.IncidentNumber)
	if err != nil {
		logger.WarnCtx(ctx, "failed to check incident investigations", "incident_number", incident.IncidentNumber, "error", err)
		return
	}
	for _, inv := range invs {
		if !store.IsTerminalInvestigationStatus(inv.Status) {
			return
		}
	}
	inv, err := s.incidentInvestigationStore.CreateIncidentInvestigation(ctx, store.IncidentInvestigationRecord{
		IncidentNumber: incident.IncidentNumber,
		Status:         "pending",
	})
	if err != nil {
		logger.WarnCtx(ctx, "failed to create incident investigation", "incident_number", incident.IncidentNumber, "error", err)
		return
	}
	if inv == nil {
		return
	}
	if s.pendingNotifier != nil {
		s.pendingNotifier.NotifyPending()
	}
	s.publishInvestigationEvent(inv.IncidentInvestigationID, "investigation_created", map[string]any{
		"investigation_id": inv.IncidentInvestigationID,
		"incident_number":  incident.IncidentNumber,
		"status":           inv.Status,
	})
	if s.investigationThreadStore != nil {
		if _, threadErr := s.investigationThreadStore.EnsureThread(ctx, store.ThreadOwnerIncidentInvestigation, strconv.FormatInt(incident.IncidentNumber, 10)); threadErr != nil {
			logger.WarnCtx(ctx, "failed to ensure incident thread", "incident_number", incident.IncidentNumber, "error", threadErr)
		}
	}
}

func (s *Server) handleListIncidentAlerts(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsRead) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}
	if s.alertStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "alert store not configured")
		return
	}

	fingerprints, err := s.alertStore.GetAlertsByIncident(r.Context(), mustParseIncidentNumber(incidentID))
	if err != nil {
		writeInternalError(w, err, "failed to list incident alerts")
		return
	}

	alerts := make([]store.AlertRecord, 0)
	for _, fp := range fingerprints {
		rec, err := s.alertStore.GetByFingerprint(fp)
		if err != nil || rec == nil {
			continue
		}
		alerts = append(alerts, *rec)
	}
	writeData(w, http.StatusOK, alerts)
}

func (s *Server) handleLinkAlertToIncident(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsWrite) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}
	if s.alertStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "alert store not configured")
		return
	}

	var req struct {
		AlertNumber int64 `json:"alert_number"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.AlertNumber <= 0 {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "alert_number is required")
		return
	}

	record, err := s.alertStore.GetByAlertNumber(req.AlertNumber)
	if err != nil {
		writeInternalError(w, err, "failed to get alert")
		return
	}
	if record == nil {
		writeError(w, ErrorCodeNotFound, "alert not found")
		return
	}

	if err := s.alertStore.LinkAlertToIncident(r.Context(), record.Fingerprint, mustParseIncidentNumber(incidentID)); err != nil {
		writeInternalError(w, err, "failed to link alert to incident")
		return
	}

	s.addIncidentTimeline(r, incidentID, "alert_linked", fmt.Sprintf("Alert #%d linked", req.AlertNumber))
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": incidentID})
	s.audit(r, store.AuditInvestigationAlertLinked, map[string]any{
		"incident_number": incidentID,
		"alert_number":    req.AlertNumber,
	})
	writeStatus(w, "linked")
}

func (s *Server) handleUnlinkAlertFromIncident(w http.ResponseWriter, r *http.Request, incidentID string, alertNumber int64) {
	if !s.checkPermission(w, r, rbac.IncidentsWrite) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}
	if s.alertStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "alert store not configured")
		return
	}

	record, err := s.alertStore.GetByAlertNumber(alertNumber)
	if err != nil {
		writeInternalError(w, err, "failed to get alert")
		return
	}
	if record == nil {
		writeError(w, ErrorCodeNotFound, "alert not found")
		return
	}

	if err := s.alertStore.UnlinkAlertFromIncident(r.Context(), record.Fingerprint, mustParseIncidentNumber(incidentID)); err != nil {
		writeInternalError(w, err, "failed to unlink alert from incident")
		return
	}

	s.addIncidentTimeline(r, incidentID, "alert_unlinked", fmt.Sprintf("Alert #%d unlinked", alertNumber))
	s.publishIncidentEvent("incident_updated", map[string]string{"incident_number": incidentID})
	s.audit(r, store.AuditInvestigationAlertUnlinked, map[string]any{
		"incident_number": incidentID,
		"alert_number":    alertNumber,
	})
	writeStatus(w, "unlinked")
}
