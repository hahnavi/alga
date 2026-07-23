package api

import (
	"net/http"

	"alga/store"
)

func (s *Server) handleOnboardingStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	completed := false
	s.mu.RLock()
	st := s.systemConfigStore
	s.mu.RUnlock()

	if st != nil {
		cfg, err := st.Get()
		if err == nil && cfg != nil {
			completed = cfg.OnboardingCompleted
		}
	}

	writeJSON(w, http.StatusOK, map[string]bool{"completed": completed})
}

func (s *Server) handleOnboardingComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	s.mu.RLock()
	st := s.systemConfigStore
	s.mu.RUnlock()

	if st == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "system config store not available")
		return
	}

	existing, _ := st.Get()
	cfg := store.SystemConfigValues{}
	if existing != nil {
		cfg = *existing
	}
	cfg.OnboardingCompleted = true

	if err := st.Save(cfg); err != nil {
		writeInternalError(w, err, "failed to save onboarding status")
		return
	}

	s.audit(r, "onboarding_completed", map[string]any{
		"completed": true,
	})

	writeJSON(w, http.StatusOK, map[string]bool{"completed": true})
}
