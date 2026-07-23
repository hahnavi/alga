package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"alga/valkey"
)

func (s *Server) handleDashboardStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	if !s.requireStore(w, s.dashboardStore, "dashboard store") {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	statsJSON, err := s.cache.GetOrSet(ctx, valkey.PrefixDashboardStats, valkey.TTLDashboardStats, func(ctx context.Context) ([]byte, error) {
		stats, err := s.dashboardStore.GetStats(ctx)
		if err != nil {
			return nil, err
		}
		return json.Marshal(stats)
	})
	if err != nil {
		writeInternalError(w, err, "failed to load dashboard stats")
		return
	}

	writeRawJSON(w, http.StatusOK, statsJSON)
}
