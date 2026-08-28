package api

import (
	"net/http"

	"alga/logger"
	"alga/store"
)

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	needsSetup := false
	count, cErr := s.userStore.CountUsers()
	if cErr != nil {
		logger.Warn("setup status count failed", "error", cErr)
	}
	if count == 0 {
		needsSetup = true
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"needs_setup": needsSetup,
	})
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	count, cErr := s.userStore.CountUsers()
	if cErr != nil {
		writeInternalError(w, cErr, "failed to check setup state")
		return
	}
	if count > 0 {
		writeError(w, ErrorCodeForbidden, "setup already completed")
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		FullName string `json:"full_name,omitempty"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Email == "" || req.Password == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "email and password are required")
		return
	}

	if err := validatePasswordPolicy(req.Password); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
		return
	}

	user, err := s.userStore.CreateUser(req.Email, req.Password, "admin")
	if err != nil {
		if store.IsDuplicateKey(err) {
			writeError(w, ErrorCodeConflict, "a user with that email already exists")
			return
		}
		writeInternalError(w, err, "failed to create admin user")
		return
	}

	// Update full name if provided
	if req.FullName != "" {
		if uErr := s.userStore.UpdateUser(user.ID, map[string]any{"full_name": req.FullName}); uErr != nil {
			logger.Warn("setup: failed to set full_name", "error", uErr)
		}
	}

	clientIP := s.ipExtractor.clientIP(r)

	// Create session and auto-login
	session, err := s.sessionStore.CreateSession(user.ID, clientIP, r.UserAgent())
	if err != nil {
		writeInternalError(w, err, "failed to create session")
		return
	}

	csrfToken, err := generateCSRFToken()
	if err != nil {
		writeInternalError(w, err, "failed to generate csrf token")
		return
	}

	s.setSessionCookie(w, session.ID)
	setRefreshTokenCookie(w, s.cfg.SecureCookies, s.sessionExpiry, session.RefreshToken)
	s.setCSRFCookie(w, csrfToken)

	// Audit
	s.auditStore.Log(store.AuditUserCreated, &user.ID, user.Email, clientIP, r.UserAgent(), true, map[string]any{
		"role": user.Role,
	})
	s.auditStore.Log(store.AuditLoginSuccess, &user.ID, user.Email, clientIP, r.UserAgent(), true, map[string]any{
		"method": "setup",
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"id":         user.ID.String(),
		"email":      user.Email,
		"full_name":  user.FullName,
		"role":       user.Role,
		"created_at": user.CreatedAt,
		"csrf_token": csrfToken,
	})
}
