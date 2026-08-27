// Package api: incident_lifecycle.go holds incident state-transition handlers
// (acknowledge, mitigate, resolve, close, reopen, cancel, escalate) plus the
// shared helpers they depend on: resolution-requirement checks, escalation
// cancellation, timeline writes, status propagation, and SSE publishing.
package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"alga/escalation"
	"alga/ics"
	"alga/logger"
	"alga/metrics"
	"alga/rabbitmq"
	"alga/rbac"
	"alga/sse"
	"alga/store"
	"alga/strutil"
	"alga/worker"
)

func (s *Server) handleAcknowledgeIncident(w http.ResponseWriter, r *http.Request, incidentID string) {
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

	if record.Status != "detected" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "incident must be in 'detected' status to acknowledge")
		return
	}

	if err := s.incidentStore.TransitionIncidentStatus(r.Context(), mustParseIncidentNumber(incidentID), []string{"detected"}, "active"); err != nil {
		if errors.Is(err, store.ErrIncidentStatusConflict) {
			writeConflict(w, "incident status changed concurrently")
			return
		}
		writeInternalError(w, err, "failed to acknowledge incident")
		return
	}

	// Cancel pending escalation on acknowledge
	s.cancelEscalationForIncident(r.Context(), incidentID, "incident acknowledged")

	s.addIncidentTimeline(r, incidentID, "acknowledged", "Incident acknowledged")

	updated, _ := s.incidentStore.GetIncident(r.Context(), mustParseIncidentNumber(incidentID))
	s.publishIncidentEvent("incident_updated", updated)
	s.publishIncidentLifecycleNotifications(r.Context(), updated, "incident_acknowledged", "acknowledged")
	s.audit(r, store.AuditIncidentAcknowledged, map[string]any{
		"incident_number": incidentID,
	})
	s.propagateServiceStatus(updated)
	s.tryAutoCreateSlackChannel(r, updated)
	s.invalidateDashboardCache(r)
	writeData(w, http.StatusOK, updated)
}

func (s *Server) handleMitigateIncident(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}

	if err := s.incidentStore.TransitionIncidentStatus(r.Context(), mustParseIncidentNumber(incidentID), []string{"detected", "active"}, "mitigated"); err != nil {
		if errors.Is(err, store.ErrIncidentStatusConflict) {
			writeConflict(w, "incident status changed concurrently")
			return
		}
		writeInternalError(w, err, "failed to mitigate incident")
		return
	}

	s.addIncidentTimeline(r, incidentID, "mitigated", "Incident mitigated")

	metrics.IncidentsMitigatedTotal.Add(1)

	updated, _ := s.incidentStore.GetIncident(r.Context(), mustParseIncidentNumber(incidentID))
	s.publishIncidentEvent("incident_updated", updated)
	s.publishIncidentLifecycleNotifications(r.Context(), updated, "incident_mitigated", "mitigated")
	s.audit(r, store.AuditIncidentMitigated, map[string]any{
		"incident_number": incidentID,
	})
	s.propagateServiceStatus(updated)
	if s.incidentChannelManager != nil {
		s.incidentChannelManager.PostStatusChange(r.Context(), updated, "mitigated")
	}
	s.invalidateDashboardCache(r)
	writeData(w, http.StatusOK, updated)
}

func (s *Server) missingIncidentResolutionRequirements(ctx context.Context, inc *store.IncidentRecord) ([]string, error) {
	missing := []string{}
	if inc == nil || strings.TrimSpace(inc.Summary) == "" {
		missing = append(missing, "summary")
	}
	if s.incidentDocumentStore == nil {
		missing = append(missing, "impact_assessment", "root_cause", "resolution")
		return missing, nil
	}
	sections, err := s.incidentDocumentStore.GetAllSections(ctx, inc.IncidentNumber)
	if err != nil {
		return nil, err
	}
	contentBySection := map[string]string{}
	for _, section := range sections {
		contentBySection[section.Section] = strings.TrimSpace(section.Content)
	}
	if contentBySection["impact_assessment"] == "" {
		missing = append(missing, "impact_assessment")
	}
	if contentBySection["root_cause"] == "" {
		missing = append(missing, "root_cause")
	}
	if contentBySection["resolution"] == "" {
		missing = append(missing, "resolution")
	}
	return missing, nil
}

func (s *Server) handleResolveIncident(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}

	current, err := s.incidentStore.GetIncident(r.Context(), mustParseIncidentNumber(incidentID))
	if err != nil {
		if errors.Is(err, store.ErrIncidentNotFound) {
			writeError(w, ErrorCodeNotFound, "incident not found")
			return
		}
		writeInternalError(w, err, "failed to get incident")
		return
	}
	missing, err := s.missingIncidentResolutionRequirements(r.Context(), current)
	if err != nil {
		writeInternalError(w, err, "failed to validate incident resolution requirements")
		return
	}
	if len(missing) > 0 {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "incident resolution requires: "+strings.Join(missing, ", "))
		return
	}

	if err := s.incidentStore.TransitionIncidentStatus(r.Context(), mustParseIncidentNumber(incidentID), []string{"detected", "active", "mitigated"}, "resolved"); err != nil {
		if errors.Is(err, store.ErrIncidentStatusConflict) {
			writeConflict(w, "incident status changed concurrently")
			return
		}
		writeInternalError(w, err, "failed to resolve incident")
		return
	}

	s.addIncidentTimeline(r, incidentID, "resolved", "Incident resolved")

	metrics.IncidentsResolvedTotal.Add(1)
	metrics.IncidentsActive.Add(-1)

	updated, _ := s.incidentStore.GetIncident(r.Context(), mustParseIncidentNumber(incidentID))
	s.publishIncidentEvent("incident_updated", updated)
	s.publishIncidentLifecycleNotifications(r.Context(), updated, "incident_resolved", "resolved")
	s.audit(r, store.AuditIncidentResolved, map[string]any{
		"incident_number": incidentID,
	})
	s.propagateServiceStatus(updated)
	if s.incidentChannelManager != nil {
		s.incidentChannelManager.PostStatusChange(r.Context(), updated, "resolved")
		s.incidentChannelManager.PostResolutionSummary(r.Context(), updated)
	}
	s.ensurePostMortemDraft(r.Context(), updated, "Incident resolved")

	cascade := runAlertCascade(r.Context(), s.alertStore, s.auditStore, cascadePublisherFromDual(s.ssePublisher), mustParseIncidentNumber(incidentID), cascadeActorFromRequest(r))

	s.invalidateDashboardCache(r)
	writeData(w, http.StatusOK, incidentResolveResponse{Incident: updated, Cascade: cascadeSummary(cascade)})
}

func (s *Server) handleCloseIncident(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}

	if err := s.incidentStore.TransitionIncidentStatus(r.Context(), mustParseIncidentNumber(incidentID), []string{"resolved"}, "closed"); err != nil {
		if errors.Is(err, store.ErrIncidentStatusConflict) {
			writeConflict(w, "incident status changed concurrently")
			return
		}
		writeInternalError(w, err, "failed to close incident")
		return
	}

	s.addIncidentTimeline(r, incidentID, "closed", "Incident closed")

	// teardown-on-close: end any live ICS role assignments so a terminal
	// incident stops surfacing active commanders/responders in ICS queries.
	// Handler-explicit on purpose — a sweeper would race a reopen.
	if s.icsRoleStore != nil {
		if err := s.icsRoleStore.EndAllRolesForIncident(r.Context(), mustParseIncidentNumber(incidentID), ics.EndReasonIncidentClosed); err != nil {
			logger.WarnCtx(r.Context(), "failed to end ICS roles on incident close", "component", "api", "incident_number", incidentID, "error", err)
		} else {
			s.addIncidentTimeline(r, incidentID, "ics_roles_ended", "ICS roles ended: incident closed")
		}
	}

	metrics.IncidentsClosedTotal.Add(1)

	updated, _ := s.incidentStore.GetIncident(r.Context(), mustParseIncidentNumber(incidentID))
	s.publishIncidentEvent("incident_updated", updated)
	s.audit(r, store.AuditIncidentClosed, map[string]any{
		"incident_number": incidentID,
	})
	s.propagateServiceStatus(updated)
	if s.incidentChannelManager != nil {
		s.incidentChannelManager.PostStatusChange(r.Context(), updated, "closed")
		s.mu.RLock()
		archiveOnClose := s.cfg.SlackIncidentChannelArchiveOnClose
		s.mu.RUnlock()
		if archiveOnClose && updated.SlackChannelID != "" {
			if err := s.incidentChannelManager.ArchiveIncidentChannel(r.Context(), updated); err != nil {
				logger.WarnCtx(r.Context(), "failed to archive slack incident channel", "error", err)
			} else {
				s.audit(r, store.AuditIncidentSlackChannelArchived, map[string]any{
					"incident_number": incidentID,
					"channel_id":      updated.SlackChannelID,
				})
			}
		}
	}
	if s.postmortemStore != nil {
		pm, pmErr := s.postmortemStore.GetByIncidentID(r.Context(), updated.ID)
		if pmErr == nil && pm == nil {
			w.Header().Set("X-Post-Mortem-Missing", "true")
		}
	}

	cascade := runAlertCascade(r.Context(), s.alertStore, s.auditStore, cascadePublisherFromDual(s.ssePublisher), mustParseIncidentNumber(incidentID), cascadeActorFromRequest(r))

	s.invalidateDashboardCache(r)
	writeData(w, http.StatusOK, incidentResolveResponse{Incident: updated, Cascade: cascadeSummary(cascade)})
}

func (s *Server) handleReopenIncident(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}

	if err := s.incidentStore.TransitionIncidentStatus(r.Context(), mustParseIncidentNumber(incidentID), []string{"mitigated", "resolved", "closed"}, "active"); err != nil {
		if errors.Is(err, store.ErrIncidentStatusConflict) {
			writeConflict(w, "incident status changed concurrently")
			return
		}
		writeInternalError(w, err, "failed to reopen incident")
		return
	}

	// reopen reset: clear the SLA resolve stamp and the breach-dedup
	// markers handler-explicitly so resolve-breach detection can re-fire on the
	// reopened incident. Kept out of applyStatusTimestampsBun's "active" case,
	// which also fires on detected→active promotion where wiping the stamp
	// would corrupt legitimate clocks.
	incidentNumber := mustParseIncidentNumber(incidentID)
	if err := s.incidentStore.ClearSLAResolvedAt(r.Context(), incidentNumber); err != nil {
		logger.WarnCtx(r.Context(), "failed to clear SLA resolve stamp on reopen", "component", "api", "incident_number", incidentID, "error", err)
	}
	worker.ClearBreachDedupKeys(r.Context(), s.vkClient, incidentNumber)

	s.addIncidentTimeline(r, incidentID, "reopened", "Incident reopened")

	metrics.IncidentsReopenedTotal.Add(1)
	metrics.IncidentsActive.Add(1)

	updated, _ := s.incidentStore.GetIncident(r.Context(), mustParseIncidentNumber(incidentID))
	s.publishIncidentEvent("incident_updated", updated)
	s.publishIncidentLifecycleNotifications(r.Context(), updated, "incident_reopened", "reopened")
	s.audit(r, store.AuditIncidentReopened, map[string]any{
		"incident_number": incidentID,
	})
	s.propagateServiceStatus(updated)
	if s.incidentChannelManager != nil {
		s.incidentChannelManager.PostStatusChange(r.Context(), updated, "active")
		if updated.SlackChannelID != "" && updated.SlackChannelArchived {
			if err := s.incidentChannelManager.UnarchiveIncidentChannel(r.Context(), updated); err != nil {
				logger.WarnCtx(r.Context(), "failed to unarchive slack incident channel", "error", err)
			} else {
				s.audit(r, store.AuditIncidentSlackChannelUnarchived, map[string]any{
					"incident_number": incidentID,
					"channel_id":      updated.SlackChannelID,
				})
			}
		}
	}
	s.invalidateDashboardCache(r)
	writeData(w, http.StatusOK, updated)
}

func (s *Server) handleCancelIncident(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}

	if err := s.incidentStore.TransitionIncidentStatus(r.Context(), mustParseIncidentNumber(incidentID), []string{"detected", "active"}, "cancelled"); err != nil {
		if errors.Is(err, store.ErrIncidentStatusConflict) {
			writeConflict(w, "incident status changed concurrently")
			return
		}
		writeInternalError(w, err, "failed to cancel incident")
		return
	}

	s.addIncidentTimeline(r, incidentID, "cancelled", "Incident cancelled")

	metrics.IncidentsCancelledTotal.Add(1)
	metrics.IncidentsActive.Add(-1)

	updated, _ := s.incidentStore.GetIncident(r.Context(), mustParseIncidentNumber(incidentID))
	s.publishIncidentEvent("incident_updated", updated)
	s.audit(r, store.AuditIncidentCancelled, map[string]any{
		"incident_number": incidentID,
	})
	s.propagateServiceStatus(updated)
	if s.incidentChannelManager != nil {
		s.incidentChannelManager.PostStatusChange(r.Context(), updated, "cancelled")
	}
	s.invalidateDashboardCache(r)
	writeData(w, http.StatusOK, updated)
}

func (s *Server) handleEscalateIncident(w http.ResponseWriter, r *http.Request, incidentID string) {
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

	// Per-incident rate limit on manual /escalate: prevents click-spam and
	// parallel operator tabs from re-enqueuing the same EscalationMessage
	// within a 60s window. The same guard is also enforced at the worker
	// layer (claimEscalationFirstPublish), so a bypass here is still safe
	// at the cost of one extra worker-side check.
	if s.vkClient != nil {
		rateLimitKey := "alga:esc:manual:" + incidentID
		ok2, rlErr := s.vkClient.SetNX(r.Context(), rateLimitKey, "1", 60*1_000_000_000)
		if rlErr == nil && !ok2 {
			writeError(w, ErrorCodeRateLimited, "an escalation was already triggered for this incident in the last 60 seconds")
			return
		}
	}

	// Suppress if the user has acknowledged or silenced the incident; manual
	// /escalate is a no-op in that case.
	if s.vkClient != nil {
		hashKey := "alga:esc:" + incidentID
		ack, _ := s.vkClient.HGet(r.Context(), hashKey, "acknowledged")
		if ack == "1" {
			writeError(w, ErrorCodeConflict, "incident escalation has already been acknowledged")
			return
		}
		silencedStr, _ := s.vkClient.HGet(r.Context(), hashKey, "silenced_until")
		if silencedStr != "" {
			if silenced, perr := strconv.ParseInt(silencedStr, 10, 64); perr == nil && silenced > time.Now().Unix() {
				writeError(w, ErrorCodeConflict, "incident escalation is silenced; cancel the silence or wait for it to expire")
				return
			}
		}
	}

	s.addIncidentTimeline(r, incidentID, "escalated", "Incident escalated")

	s.publishIncidentEvent("incident_updated", record)
	s.audit(r, store.AuditIncidentEscalated, map[string]any{
		"incident_number": incidentID,
	})

	if s.rabbitmqPublisher != nil {
		escMsg := rabbitmq.EscalationMessage{
			IncidentNumber: record.IncidentNumber,
			Level:          1,
			MaxRetries:     rabbitmq.MaxEscalationRetries,
		}

		if record.ServiceID != nil {
			svc, err := s.serviceStore.GetService(r.Context(), record.ServiceID.String())
			if err == nil && svc != nil && svc.EscalationPolicyID != nil {
				escMsg.PolicyID = *svc.EscalationPolicyID
			}
		}

		if err := s.rabbitmqPublisher.PublishEscalation(r.Context(), escMsg); err != nil {
			logger.WarnCtx(r.Context(), "failed to publish escalation message", "incident_number", incidentID, "error", err)
		}
	} else {
		logger.WarnCtx(r.Context(), "no RabbitMQ publisher configured; manual escalation not dispatched to queue", "incident_number", incidentID)
	}
	writeData(w, http.StatusOK, record)
}

// cancelEscalationForIncident delegates to the shared escalation-package
// helper so every ack surface (API, phone callbacks, Slack buttons) cancels
// through the same state keys and timeline contract.
func (s *Server) cancelEscalationForIncident(ctx context.Context, incidentID, reason string) {
	escalation.CancelForIncident(ctx, s.vkClient, s.incidentStore, incidentID, reason)
}

func (s *Server) addIncidentTimeline(r *http.Request, incidentID, eventType, message string) {
	incidentNumber := mustParseIncidentNumber(incidentID)
	if incidentNumber == 0 {
		return
	}
	user := userFromContext(r.Context())
	actorID := ""
	if user != nil {
		actorID = user.ID.String()
	}
	if err := s.incidentStore.AddTimelineEntry(r.Context(), &store.IncidentTimelineEntryRecord{
		IncidentNumber: incidentNumber,
		EventType:      eventType,
		ActorID:        parseUUIDPtr(actorID),
		ActorType:      "user",
		Message:        message,
	}); err != nil {
		logger.WarnCtx(r.Context(), "failed to add incident timeline entry", "incident_number", incidentID, "event_type", eventType, "error", err)
	}
}

func (s *Server) publishIncidentEvent(eventType string, data any) {
	if s.ssePublisher == nil {
		return
	}
	s.ssePublisher.Publish(sse.Event{Type: eventType, Data: data})
}

// incidentLifecycleTitleMaxLen caps the incident title carried in lifecycle
// notification bodies.
const incidentLifecycleTitleMaxLen = 200

// incidentNotificationRecipients resolves who should hear about an incident's
// lifecycle transitions: every active human ICS role holder, plus the record's
// commander and on-call responder fields as fallbacks when no roles are
// assigned. Recipients are always derived from stored incident state — never
// from request bodies.
func (s *Server) incidentNotificationRecipients(ctx context.Context, inc *store.IncidentRecord) []uuid.UUID {
	if inc == nil {
		return nil
	}
	seen := make(map[uuid.UUID]bool)
	var out []uuid.UUID
	add := func(id *uuid.UUID) {
		if id == nil || seen[*id] {
			return
		}
		seen[*id] = true
		out = append(out, *id)
	}
	if s.icsRoleStore != nil {
		roles, err := s.icsRoleStore.GetActiveRoles(ctx, inc.IncidentNumber)
		if err != nil {
			logger.WarnCtx(ctx, "failed to load ICS roles for notification fan-out", "component", "api", "incident_number", inc.IncidentNumber, "error", err)
		}
		for _, role := range roles {
			add(role.UserID)
		}
	}
	add(inc.CommanderID)
	add(inc.OnCallResponderID)
	return out
}

// publishIncidentLifecycleNotifications fans one lifecycle transition out to
// the incident's participants. Fire-and-forget per house style: publish
// failures are logged and never fail the triggering request.
func (s *Server) publishIncidentLifecycleNotifications(ctx context.Context, inc *store.IncidentRecord, notificationType, verb string) {
	if inc == nil || s.rabbitmqPublisher == nil {
		return
	}
	recipients := s.incidentNotificationRecipients(ctx, inc)
	if len(recipients) == 0 {
		return
	}
	title := fmt.Sprintf("Incident %d %s", inc.IncidentNumber, verb)
	message := strutil.TruncateOneLine(inc.Title, incidentLifecycleTitleMaxLen)
	resourceID := strconv.FormatInt(inc.IncidentNumber, 10)
	for _, userID := range recipients {
		if err := s.rabbitmqPublisher.PublishNotificationDispatch(ctx, rabbitmq.NotificationDispatchMessage{
			UserID:           userID.String(),
			IncidentNumber:   inc.IncidentNumber,
			NotificationType: notificationType,
			Title:            title,
			Message:          message,
			ResourceType:     "incident",
			ResourceID:       resourceID,
		}); err != nil {
			logger.WarnCtx(ctx, "failed to publish incident lifecycle notification", "component", "api", "incident_number", inc.IncidentNumber, "notification_type", notificationType, "user_id", userID, "error", err)
		}
	}
}

func (s *Server) propagateServiceStatus(incident *store.IncidentRecord) {
	if incident == nil || incident.ServiceID == nil || s.statusTracker == nil {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("goroutine panic recovered", "panic", r, "location", "propagateServiceStatus")
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.statusTracker.PropagateAndCascade(ctx, incident.ServiceID.String())
	}()
}
