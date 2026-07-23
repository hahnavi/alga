package api

import (
	"net/http"

	"alga/health"
)

// handleLive serves GET /live: liveness only, no dependency checks.
func (s *Server) handleLive(w http.ResponseWriter, r *http.Request) {
	if h := s.probeHandler(); h != nil {
		h.Live(w, r)
		return
	}
	writeStatus(w, "ok")
}

// handleReady serves GET /ready and its aliases: readiness with dependency checks.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if h := s.probeHandler(); h != nil {
		h.Ready(w, r)
		return
	}
	writeStatus(w, "ok")
}

// probeHandler returns the shared probe handler, or nil if not wired.
func (s *Server) probeHandler() *health.Handler {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.healthHandler
}
