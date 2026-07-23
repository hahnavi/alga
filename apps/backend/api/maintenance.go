package api

import (
	"net/http"
	"strings"
	"time"

	"alga/logger"
	"alga/rbac"
	"alga/store"
)

func (s *Server) handleMaintenanceWindows(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.maintenanceWindowStore, "maintenance window store") {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listMaintenanceWindows(w, r)
	case http.MethodPost:
		if !s.checkPermission(w, r, rbac.RoutesWrite) {
			return
		}
		s.createMaintenanceWindow(w, r)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleMaintenanceWindowByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w, s.maintenanceWindowStore, "maintenance window store") {
		return
	}
	id := pathID(r, "/api/v1/maintenance-windows/")
	id = strings.TrimSuffix(id, "/")
	if id == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		rec, err := s.maintenanceWindowStore.Get(r.Context(), id)
		if err != nil {
			writeInternalError(w, err, "failed to get maintenance window")
			return
		}
		if rec == nil {
			writeError(w, ErrorCodeNotFound, "maintenance window not found")
			return
		}
		writeData(w, http.StatusOK, rec)
	case http.MethodPut:
		if !s.checkPermission(w, r, rbac.RoutesWrite) {
			return
		}
		var req maintenanceWindowRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		patch := &store.MaintenanceWindowRecord{
			Name:          req.Name,
			LabelMatchers: req.LabelMatchers,
		}
		if req.StartTime != "" {
			if t, ok := parseTimeQuery(req.StartTime); ok {
				patch.StartTime = t
			}
		}
		if req.EndTime != "" {
			if t, ok := parseTimeQuery(req.EndTime); ok {
				patch.EndTime = t
			}
		}
		if !patch.StartTime.IsZero() && !patch.EndTime.IsZero() {
			if !patch.EndTime.After(patch.StartTime) {
				writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "end_time must be after start_time")
				return
			}
		} else if !patch.StartTime.IsZero() || !patch.EndTime.IsZero() {
			existing, err := s.maintenanceWindowStore.Get(r.Context(), id)
			if err == nil && existing != nil {
				st := patch.StartTime
				if st.IsZero() {
					st = existing.StartTime
				}
				et := patch.EndTime
				if et.IsZero() {
					et = existing.EndTime
				}
				if !et.After(st) {
					writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "end_time must be after start_time")
					return
				}
			}
		}
		if req.Enabled != nil {
			patch.Enabled = *req.Enabled
			patch.EnabledSet = true
		}
		out, err := s.maintenanceWindowStore.Update(r.Context(), id, patch)
		if err != nil {
			writeInternalError(w, err, "failed to update maintenance window")
			return
		}
		s.audit(r, store.AuditMaintenanceWindowUpdated, map[string]any{
			"maintenance_window_id": id,
			"enabled":               out.Enabled,
		})
		logger.InfoCtx(r.Context(), "maintenance window updated", "component", "api", "maintenance_window_id", id, "enabled", out.Enabled)
		writeData(w, http.StatusOK, out)
	case http.MethodDelete:
		if !s.checkPermission(w, r, rbac.RoutesWrite) {
			return
		}
		if err := s.maintenanceWindowStore.Delete(r.Context(), id); err != nil {
			writeInternalError(w, err, "failed to delete maintenance window")
			return
		}
		logger.InfoCtx(r.Context(), "maintenance window deleted", "component", "api", "maintenance_window_id", id)
		s.audit(r, store.AuditMaintenanceWindowDeleted, map[string]any{
			"maintenance_window_id": id,
		})
		writeStatus(w, "deleted")
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) listMaintenanceWindows(w http.ResponseWriter, r *http.Request) {
	q := store.MaintenanceWindowQuery{}
	if v := r.URL.Query().Get("enabled"); v != "" {
		enabled := strings.EqualFold(v, "true") || v == "1"
		q.Enabled = &enabled
	}
	limit, skip := parseLimitSkip(r, 50)
	q.Limit = int(limit)
	q.Skip = int(skip)
	items, total, err := s.maintenanceWindowStore.List(r.Context(), q)
	if err != nil {
		writeInternalError(w, err, "failed to list maintenance windows")
		return
	}
	writePaginatedJSON(w, ensureSlice(items), total)
}

type maintenanceWindowRequest struct {
	Name          string            `json:"name,omitempty"`
	StartTime     string            `json:"start_time,omitempty"`
	EndTime       string            `json:"end_time,omitempty"`
	LabelMatchers map[string]string `json:"label_matchers,omitempty"`
	Enabled       *bool             `json:"enabled,omitempty"`
}

func (s *Server) createMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	var req maintenanceWindowRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "name is required")
		return
	}
	if req.StartTime == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "start_time is required")
		return
	}
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid start_time format (expected RFC3339)")
		return
	}
	if req.EndTime == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "end_time is required")
		return
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid end_time format (expected RFC3339)")
		return
	}
	if !endTime.After(startTime) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "end_time must be after start_time")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	user := userFromContext(r.Context())
	createdBy := ""
	if user != nil {
		createdBy = user.Email
	}
	record := &store.MaintenanceWindowRecord{
		Name:          strings.TrimSpace(req.Name),
		StartTime:     startTime,
		EndTime:       endTime,
		LabelMatchers: req.LabelMatchers,
		CreatedBy:     createdBy,
		Enabled:       enabled,
	}
	out, err := s.maintenanceWindowStore.Create(r.Context(), record)
	if err != nil {
		writeInternalError(w, err, "failed to create maintenance window")
		return
	}
	logger.InfoCtx(r.Context(), "maintenance window created", "component", "api", "maintenance_window_id", out.ID.String(), "name", out.Name)
	s.audit(r, store.AuditMaintenanceWindowCreated, map[string]any{
		"maintenance_window_id": out.ID.String(),
		"name":                  out.Name,
	})
	writeData(w, http.StatusCreated, out)
}
