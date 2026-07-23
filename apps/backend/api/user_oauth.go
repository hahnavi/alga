// Code moved from http.go; see git history.

package api

import (
	"net/http"
)

func (s *Server) handleUserSlackAuthorize(w http.ResponseWriter, r *http.Request) {
	if s.userSlackHandler == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "user Slack binding is not available")
		return
	}
	s.userSlackHandler.handleAuthorize(w, r)
}

func (s *Server) handleUserSlackCallback(w http.ResponseWriter, r *http.Request) {
	if s.userSlackHandler == nil {
		http.Redirect(w, r, "/settings?slack_linked=error&message=not_configured", http.StatusFound)
		return
	}
	s.userSlackHandler.handleCallback(w, r)
}

func (s *Server) handleUserSlackDisconnect(w http.ResponseWriter, r *http.Request) {
	if s.userSlackHandler == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "user Slack binding is not available")
		return
	}
	s.userSlackHandler.handleDisconnect(w, r)
}

func (s *Server) handleUserGoogleAuthorize(w http.ResponseWriter, r *http.Request) {
	if s.userGoogleHandler == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "user Google binding is not available")
		return
	}
	s.userGoogleHandler.handleAuthorize(w, r)
}

func (s *Server) handleUserGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if s.userGoogleHandler == nil {
		http.Redirect(w, r, "/settings?google_linked=error&message=not_configured", http.StatusFound)
		return
	}
	s.userGoogleHandler.handleCallback(w, r)
}

func (s *Server) handleUserGoogleDisconnect(w http.ResponseWriter, r *http.Request) {
	if s.userGoogleHandler == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "user Google binding is not available")
		return
	}
	s.userGoogleHandler.handleDisconnect(w, r)
}
