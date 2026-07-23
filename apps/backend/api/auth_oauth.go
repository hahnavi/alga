// Code moved from http.go; see git history.

package api

import (
	"net/http"
	"strings"

	"github.com/google/uuid"
)

func (s *Server) handleGoogleOAuthEnabled(w http.ResponseWriter, r *http.Request) {
	if s.googleOAuthHandler == nil {
		writeJSON(w, http.StatusOK, map[string]bool{"enabled": false})
		return
	}
	s.googleOAuthHandler.handleEnabled(w, r)
}

func (s *Server) handleGoogleOAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	if s.googleOAuthHandler == nil {
		writeError(w, ErrorCodeNotFound, "Google Sign-In is not configured")
		return
	}
	s.googleOAuthHandler.handleAuthorize(w, r)
}

func (s *Server) handleGoogleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if s.googleOAuthHandler == nil {
		http.Redirect(w, r, "/login?error=google_not_configured", http.StatusFound)
		return
	}
	s.googleOAuthHandler.handleCallback(w, r)
}

func (s *Server) handleOIDCListProviders(w http.ResponseWriter, r *http.Request) {
	if s.oidcHandler == nil {
		writePaginatedJSON(w, []any{}, 0)
		return
	}
	s.oidcHandler.listProviders(w, r)
}

func (s *Server) handleOIDCCreateProvider(w http.ResponseWriter, r *http.Request) {
	if s.oidcHandler == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "OIDC is not configured")
		return
	}
	s.oidcHandler.createProvider(w, r)
}

func (s *Server) handleOIDCProviderRoutes(w http.ResponseWriter, r *http.Request) {
	if s.oidcHandler == nil {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "OIDC is not configured")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/oidc/providers/")
	rest = strings.TrimSuffix(rest, "/")
	if rest == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "missing provider id")
		return
	}
	id, err := uuid.Parse(rest)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid provider id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.oidcHandler.getProvider(w, r, id)
	case http.MethodPut:
		s.oidcHandler.updateProvider(w, r, id)
	case http.MethodDelete:
		s.oidcHandler.deleteProvider(w, r, id)
	default:
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
	}
}

func (s *Server) handleOIDCPublicProviders(w http.ResponseWriter, r *http.Request) {
	if s.oidcHandler == nil {
		writeData(w, http.StatusOK, []any{})
		return
	}
	s.oidcHandler.listPublicProviders(w, r)
}

func (s *Server) handleOIDCAuthorize(w http.ResponseWriter, r *http.Request) {
	if s.oidcHandler == nil {
		writeError(w, ErrorCodeNotFound, "SSO is not configured")
		return
	}
	idStr := pathID(r, "/api/v1/auth/oidc/")
	idStr = strings.TrimSuffix(idStr, "/authorize")
	providerID, err := uuid.Parse(idStr)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid provider id")
		return
	}
	s.oidcHandler.authorize(w, r, providerID)
}

func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if s.oidcHandler == nil {
		http.Redirect(w, r, "/login?error=oidc_not_configured", http.StatusFound)
		return
	}
	idStr := pathID(r, "/api/v1/auth/oidc/")
	idStr = strings.TrimSuffix(idStr, "/callback")
	providerID, err := uuid.Parse(idStr)
	if err != nil {
		http.Redirect(w, r, "/login?error=oidc_auth_failed", http.StatusFound)
		return
	}
	s.oidcHandler.callback(w, r, providerID)
}

func (s *Server) handleSlackSignInEnabled(w http.ResponseWriter, r *http.Request) {
	if s.slackSignInHandler == nil {
		writeJSON(w, http.StatusOK, map[string]bool{"enabled": false})
		return
	}
	s.slackSignInHandler.handleEnabled(w, r)
}

func (s *Server) handleSlackSignInAuthorize(w http.ResponseWriter, r *http.Request) {
	if s.slackSignInHandler == nil {
		writeError(w, ErrorCodeNotFound, "Slack Sign-In is not configured")
		return
	}
	s.slackSignInHandler.handleAuthorize(w, r)
}

func (s *Server) handleSlackSignInCallback(w http.ResponseWriter, r *http.Request) {
	if s.slackSignInHandler == nil {
		http.Redirect(w, r, "/login?error=slack_not_configured", http.StatusFound)
		return
	}
	s.slackSignInHandler.handleCallback(w, r)
}
