package api

import (
	"net/http"
	"strings"

	"alga/rbac"
	"alga/store"

	"github.com/google/uuid"
)

func (s *Server) handleListHandoffs(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.OnCallRead) {
		return
	}
	if !s.requireHandoffStore(w) {
		return
	}

	limit, skip := parseLimitSkip(r, 50)

	filter := store.HandoffFilter{}
	if v := r.URL.Query().Get("schedule_id"); v != "" {
		if uid, err := uuid.Parse(v); err == nil {
			filter.ScheduleID = &uid
		}
	}
	if v := r.URL.Query().Get("user_id"); v != "" {
		if uid, err := uuid.Parse(v); err == nil {
			filter.UserID = &uid
		}
	}
	if v := r.URL.Query().Get("status"); v != "" {
		filter.Status = v
	}

	records, total, err := s.handoffStore.List(r.Context(), filter, int(limit), int(skip))
	if err != nil {
		writeInternalError(w, err, "failed to list handoffs")
		return
	}
	writePaginatedJSON(w, ensureSlice(records), total)
}

func (s *Server) handleGetHandoff(w http.ResponseWriter, r *http.Request, id string) {
	if !s.checkPermission(w, r, rbac.OnCallRead) {
		return
	}
	if !s.requireHandoffStore(w) {
		return
	}

	uid, err := uuid.Parse(id)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid id")
		return
	}

	record, err := s.handoffStore.Get(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to get handoff")
		return
	}
	if record == nil {
		writeError(w, ErrorCodeNotFound, "handoff not found")
		return
	}
	writeData(w, http.StatusOK, record)
}

func (s *Server) handlePendingHandoffs(w http.ResponseWriter, r *http.Request) {
	if !s.checkPermission(w, r, rbac.OnCallRead) {
		return
	}
	if !s.requireHandoffStore(w) {
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "not authenticated")
		return
	}

	records, err := s.handoffStore.GetPendingForUser(r.Context(), user.ID)
	if err != nil {
		writeInternalError(w, err, "failed to get pending handoffs")
		return
	}
	writeData(w, http.StatusOK, ensureSlice(records))
}

func (s *Server) handleSaveHandoffNotes(w http.ResponseWriter, r *http.Request, id string) {
	if !s.checkPermission(w, r, rbac.OnCallWrite) {
		return
	}
	if !s.requireHandoffStore(w) {
		return
	}

	uid, err := uuid.Parse(id)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid id")
		return
	}

	record, err := s.handoffStore.Get(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to get handoff")
		return
	}
	if record == nil {
		writeError(w, ErrorCodeNotFound, "handoff not found")
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "not authenticated")
		return
	}

	var req struct {
		Notes string `json:"notes"`
		Field string `json:"field"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	switch req.Field {
	case "outgoing_notes":
		if record.OutgoingUserID == nil || user.ID != *record.OutgoingUserID {
			writeError(w, ErrorCodeForbidden, "only the outgoing user can save outgoing notes")
			return
		}
		if err := s.handoffStore.UpdateOutgoingNotes(r.Context(), uid, req.Notes); err != nil {
			writeInternalError(w, err, "failed to save outgoing notes")
			return
		}
	case "incoming_notes":
		if record.IncomingUserID == nil || user.ID != *record.IncomingUserID {
			writeError(w, ErrorCodeForbidden, "only the incoming user can save incoming notes")
			return
		}
		if err := s.handoffStore.UpdateIncomingNotes(r.Context(), uid, req.Notes); err != nil {
			writeInternalError(w, err, "failed to save incoming notes")
			return
		}
	default:
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "field must be outgoing_notes or incoming_notes")
		return
	}

	s.audit(r, store.AuditHandoffNotesSaved, map[string]any{
		"handoff_id": id,
		"field":      req.Field,
	})
	writeStatus(w, "ok")
}

func (s *Server) handleAcknowledgeHandoff(w http.ResponseWriter, r *http.Request, id string) {
	if !s.checkPermission(w, r, rbac.OnCallWrite) {
		return
	}
	if !s.requireHandoffStore(w) {
		return
	}

	uid, err := uuid.Parse(id)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid id")
		return
	}

	record, err := s.handoffStore.Get(r.Context(), uid)
	if err != nil {
		writeInternalError(w, err, "failed to get handoff")
		return
	}
	if record == nil {
		writeError(w, ErrorCodeNotFound, "handoff not found")
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "not authenticated")
		return
	}

	if record.IncomingUserID == nil || user.ID != *record.IncomingUserID {
		writeError(w, ErrorCodeForbidden, "only the incoming user can acknowledge the handoff")
		return
	}

	if err := s.handoffStore.Acknowledge(r.Context(), uid); err != nil {
		writeInternalError(w, err, "failed to acknowledge handoff")
		return
	}

	s.audit(r, store.AuditHandoffAcknowledged, map[string]any{
		"handoff_id": id,
	})
	writeStatus(w, "ok")
}

func (s *Server) handleHandoffRoutes(w http.ResponseWriter, r *http.Request) {
	suffix := pathID(r, "/api/v1/on-call/handoffs/")
	if suffix == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing handoff id")
		return
	}

	if suffix == "pending" {
		if r.Method == http.MethodGet {
			s.handlePendingHandoffs(w, r)
		} else {
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		}
		return
	}

	if strings.HasSuffix(suffix, "/notes") {
		id := strings.TrimSuffix(suffix, "/notes")
		if r.Method == http.MethodPost {
			s.handleSaveHandoffNotes(w, r, id)
		} else {
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		}
		return
	}

	if strings.HasSuffix(suffix, "/acknowledge") {
		id := strings.TrimSuffix(suffix, "/acknowledge")
		if r.Method == http.MethodPost {
			s.handleAcknowledgeHandoff(w, r, id)
		} else {
			writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		}
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetHandoff(w, r, suffix)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) requireHandoffStore(w http.ResponseWriter) bool {
	s.mu.RLock()
	st := s.handoffStore
	s.mu.RUnlock()
	return s.requireStore(w, st, "handoff store")
}
