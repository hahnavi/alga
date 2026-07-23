package api

import (
	"net/http"
	"time"

	"alga/rbac"
)

func (s *Server) handleIncidentMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}
	if !s.checkPermission(w, r, rbac.IncidentsRead) {
		return
	}
	if !s.requireIncidentStore(w) {
		return
	}

	var startDate, endDate time.Time
	if v := r.URL.Query().Get("start_date"); v != "" {
		if t, ok := parseTimeQuery(v); ok {
			startDate = t
		}
	}
	if v := r.URL.Query().Get("end_date"); v != "" {
		if t, ok := parseTimeQuery(v); ok {
			endDate = t
		}
	}

	if startDate.IsZero() {
		startDate = time.Now().UTC().AddDate(0, -1, 0)
	}
	if endDate.IsZero() {
		endDate = time.Now().UTC()
	}

	metrics, err := s.incidentStore.GetIncidentMetrics(r.Context(), startDate, endDate)
	if err != nil {
		writeInternalError(w, err, "failed to compute incident metrics")
		return
	}
	writeData(w, http.StatusOK, metrics)
}
