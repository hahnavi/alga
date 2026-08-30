package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"alga/rbac"
	"alga/store"
)

type statusUpdateRequest struct {
	StatusLevel string `json:"status_level"`
	Body        string `json:"body"`
	Internal    bool   `json:"internal,omitempty"`
}

func (s *Server) handleIncidentStatusUpdates(w http.ResponseWriter, r *http.Request, incidentID string) {
	if s.incidentCoordinationStore == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "incident coordination store not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleListStatusUpdates(w, r, incidentID)
	case http.MethodPost:
		s.handleCreateStatusUpdate(w, r, incidentID)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleListStatusUpdates(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsRead) {
		return
	}
	limit, skip := parseLimitSkip(r, 50)
	incidentNumber, err := strconv.ParseInt(incidentID, 10, 64)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid incident number")
		return
	}
	messages, err := s.incidentCoordinationStore.ListMessagesByKind(
		r.Context(), incidentNumber, store.IncidentCoordinationKindStatusUpdate, int(limit), int(skip),
	)
	if err != nil {
		writeInternalError(w, err, "failed to list status updates")
		return
	}
	writeData(w, http.StatusOK, ensureSlice(messages))
}

func (s *Server) handleCreateStatusUpdate(w http.ResponseWriter, r *http.Request, incidentID string) {
	if !s.checkPermission(w, r, rbac.IncidentsCommand) {
		return
	}
	var req statusUpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	incidentNumber, err := strconv.ParseInt(incidentID, 10, 64)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid incident number")
		return
	}
	statusLevel := strings.TrimSpace(req.StatusLevel)
	if statusLevel == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "status_level is required")
		return
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "body is required")
		return
	}
	user := userFromContext(r.Context())
	actorName := "User"
	var actorID uuid.UUID
	if user != nil {
		actorID = user.ID
		actorName = user.DisplayName()
		if actorName == "" {
			actorName = user.Email
		}
	}
	created, err := s.incidentCoordinationStore.CreateStatusUpdate(
		r.Context(), incidentNumber, statusLevel, body, req.Internal, actorID, actorName,
	)
	if err != nil {
		if errors.Is(err, store.ErrInvalidStatusLevel) {
			writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid status_level: must be investigating, identified, mitigated, monitoring, or resolved")
			return
		}
		writeInternalError(w, err, "failed to create status update")
		return
	}
	s.syncIncidentCoordinationMessageToSlack(r, created, user)
	s.publishIncidentEvent("incident_coordination_message_created", map[string]string{
		"incident_number": incidentID,
		"message_id":      created.ID.String(),
	})
	s.audit(r, store.AuditIncidentStatusUpdateCreated, map[string]any{
		"incident_number": incidentNumber,
		"message_id":      created.ID.String(),
		"status_level":    statusLevel,
	})
	writeData(w, http.StatusCreated, created)
}
