package api

import (
	"net/http"

	"alga/store"
)

func (s *Server) handleDeadLetteredInvestigations(w http.ResponseWriter, r *http.Request) {
	limit64, skip64 := parseLimitSkip(r, 50)
	limit := int(limit64)
	skip := int(skip64)

	filter := map[string]any{
		"status_in": []string{"failed", "timed_out"},
		"limit":     limit,
		"skip":      skip,
	}

	investigations, err := s.alertInvestigationStore.ListAlertInvestigations(r.Context(), filter)
	if err != nil {
		writeInternalError(w, err, "failed to list dead-lettered investigations")
		return
	}

	if investigations == nil {
		investigations = []store.AlertInvestigationRecord{}
	}

	resp := map[string]any{
		"investigations": investigations,
		"limit":          limit,
		"skip":           skip,
	}

	if len(investigations) == limit {
		filter["skip"] = skip + limit
		more, err := s.alertInvestigationStore.ListAlertInvestigations(r.Context(), filter)
		if err == nil && len(more) > 0 {
			resp["has_more"] = true
		}
	}
	writeData(w, http.StatusOK, resp)
}
