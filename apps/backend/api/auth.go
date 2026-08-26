package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nyaruka/phonenumbers/v2"

	"alga/api/platform"
	algacrypto "alga/crypto"
	"alga/logger"
	"alga/rbac"
	"alga/sse"
	"alga/store"
)

// generateCSRFToken generates a random CSRF token
func generateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// AuthMiddlewareSSE provides authentication for SSE endpoints
// Provides authentication for SSE endpoints
func AuthMiddlewareSSE(next http.HandlerFunc, sessionStore store.SessionStore, userStore store.UserStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("alga_session")
		if err != nil {
			writeError(w, ErrorCodeUnauthorized, "not authenticated")
			return
		}

		session, err := sessionStore.GetSession(cookie.Value)
		if err != nil || session == nil {
			writeError(w, ErrorCodeUnauthorized, "not authenticated")
			return
		}

		user, err := userStore.GetByID(session.UserID)
		if err != nil || user == nil {
			writeError(w, ErrorCodeUnauthorized, "not authenticated")
			return
		}

		if user.LockedUntil != nil && time.Now().Before(*user.LockedUntil) {
			writeError(w, ErrorCodeForbidden, "account is locked")
			return
		}

		ctx := platform.WithUser(r.Context(), user)
		ctx = sse.SetUserContext(ctx, user)
		// Inject the authenticated user id so the logger's contextHandler
		// attaches user_id to every downstream log line (W8).
		ctx = logger.WithUser(ctx, user.ID.String())
		next(w, r.WithContext(ctx))
	}
}

// userFromContext retrieves the authenticated user from the request context.
// Delegates to platform.UserFromContext.
func userFromContext(ctx context.Context) *store.UserRecord {
	return platform.UserFromContext(ctx)
}

// authMiddleware is a thin wrapper around platform.AuthMiddleware, building the
// AuthDeps from the *Server fields. The canonical implementation lives in the
// platform package.
func (s *Server) authMiddleware(next http.HandlerFunc, perms ...rbac.Permission) http.HandlerFunc {
	return platform.AuthMiddleware(s.PlatformAuthDeps(), next, perms...)
}

// checkPermission is a thin wrapper around platform.CheckPermission.
func (s *Server) checkPermission(w http.ResponseWriter, r *http.Request, perms ...rbac.Permission) bool {
	return platform.CheckPermission(w, r, perms...)
}

// validateCSRFToken is a thin wrapper around platform.ValidateCSRFToken.
func (s *Server) validateCSRFToken(r *http.Request) bool {
	return platform.ValidateCSRFToken(r)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	clientIP := s.ipExtractor.clientIP(r)

	// Check rate limiting
	allowed, remaining, lockedUntil := s.loginLimiter.CheckLoginAllowed(clientIP)
	if !allowed {
		s.auditStore.Log(store.AuditLoginFailed, nil, "", clientIP, r.UserAgent(), false, map[string]any{
			"reason":       "rate_limited",
			"locked_until": lockedUntil,
		})
		// The limiter may deny without a lock expiry (e.g. Valkey outage fallback);
		// omit Retry-After rather than dereference a nil pointer.
		if lockedUntil != nil {
			w.Header().Set("Retry-After", strconv.Itoa(int(time.Until(*lockedUntil).Seconds())))
		}
		writeError(w, ErrorCodeRateLimited, "too many login attempts. please try again later")
		return
	}

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Email == "" || req.Password == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "email and password are required")
		return
	}

	// Validate password policy on the password being submitted (for registration only)
	// This is just for login, so we don't validate here

	user, err := s.userStore.Authenticate(req.Email, req.Password)
	if err != nil {
		// Record failed login
		_ = s.userStore.RecordFailedLogin(req.Email)

		// Log the failure
		var userID *uuid.UUID
		if user != nil {
			userID = &user.ID
		}
		s.auditStore.Log(store.AuditLoginFailed, userID, req.Email, clientIP, r.UserAgent(), false, map[string]any{
			"error":              err.Error(),
			"remaining_attempts": remaining - 1,
		})

		if errors.Is(err, store.ErrAccountLocked) {
			writeError(w, ErrorCodeForbidden, "account is locked due to too many failed login attempts. please try again in 30 minutes")
			return
		}
		writeError(w, ErrorCodeUnauthorized, "invalid credentials")
		return
	}

	if user == nil {
		// User not found or password mismatch
		_ = s.userStore.RecordFailedLogin(req.Email)
		s.auditStore.Log(store.AuditLoginFailed, nil, req.Email, clientIP, r.UserAgent(), false, map[string]any{
			"reason": "invalid_credentials",
		})
		writeError(w, ErrorCodeUnauthorized, "invalid credentials")
		return
	}

	// Reset failed attempts and record successful login
	_ = s.userStore.RecordSuccessfulLogin(user.ID, clientIP)
	s.loginLimiter.Reset(clientIP)

	session, err := s.sessionStore.CreateSession(user.ID, clientIP, r.UserAgent())
	if err != nil {
		s.auditStore.Log(store.AuditLoginFailed, &user.ID, user.Email, clientIP, r.UserAgent(), false, map[string]any{
			"reason": "session_creation_failed",
			"error":  err.Error(),
		})
		writeError(w, ErrorCodeInternal, "failed to create session")
		return
	}

	// Generate and set CSRF token
	csrfToken, err := generateCSRFToken()
	if err != nil {
		writeError(w, ErrorCodeInternal, "failed to generate csrf token")
		return
	}

	// Set session cookie with secure settings
	s.setSessionCookie(w, session.ID)
	s.setCSRFCookie(w, csrfToken)

	// Log successful login
	s.auditStore.Log(store.AuditLoginSuccess, &user.ID, user.Email, clientIP, r.UserAgent(), true, nil)

	// Include permissions so the frontend can gate UI (sidebar, route
	// metadata) immediately after login without a second round-trip to
	// /auth/me. Mirrors handleGetCurrentUser.
	perms := rbac.AllPermissions(user.Role)
	permStrings := make([]string, len(perms))
	for i, p := range perms {
		permStrings[i] = string(p)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":            user.ID.String(),
		"email":         user.Email,
		"full_name":     user.FullName,
		"phone":         user.Phone,
		"phone_country": user.PhoneCountry,
		"role":          user.Role,
		"created_at":    user.CreatedAt,
		"permissions":   permStrings,
		"csrf_token":    csrfToken,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	// Validate CSRF token for logout
	if !s.validateCSRFToken(r) {
		writeError(w, ErrorCodeForbidden, "invalid csrf token")
		return
	}

	clientIP := s.ipExtractor.clientIP(r)

	if cookie, err := r.Cookie("alga_session"); err == nil {
		// Get session for audit logging
		if session, _ := s.sessionStore.GetSession(cookie.Value); session != nil {
			if user, _ := s.userStore.GetByID(session.UserID); user != nil {
				s.auditStore.Log(store.AuditLogout, &user.ID, user.Email, clientIP, r.UserAgent(), true, nil)
			}
		}
		_ = s.sessionStore.DeleteSession(cookie.Value)
	}

	// Clear cookies
	s.clearSessionCookie(w)
	s.clearCSRFCookie(w)

	writeStatus(w, "logged out")
}

func (s *Server) handleGetCurrentUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "not authenticated")
		return
	}

	// Permissions come from the same RBAC table that gates route handlers
	// (see apps/backend/rbac/roles.go). The frontend consumes this to
	// drive `auth.hasPermission(...)` instead of mirroring a hardcoded
	// role→permission map.
	perms := rbac.AllPermissions(user.Role)
	permStrings := make([]string, len(perms))
	for i, p := range perms {
		permStrings[i] = string(p)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":                 user.ID.String(),
		"email":              user.Email,
		"full_name":          user.FullName,
		"phone":              user.Phone,
		"phone_country":      user.PhoneCountry,
		"role":               user.Role,
		"created_at":         user.CreatedAt,
		"last_login_at":      user.LastLoginAt,
		"slack_linked":       user.SlackUserID != "",
		"slack_display_name": user.SlackDisplayName,
		"google_linked":      user.GoogleID != "",
		"permissions":        permStrings,
	})
}

func (s *Server) handleRefreshSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
		if csrfCookie, err := r.Cookie("alga_csrf"); err != nil || csrfCookie.Value == "" {
			writeError(w, ErrorCodeForbidden, "missing csrf token")
			return
		} else if r.Header.Get("X-CSRF-Token") == "" {
			writeError(w, ErrorCodeForbidden, "missing csrf token")
			return
		} else if !algacrypto.ConstantTimeEqualString(csrfCookie.Value, r.Header.Get("X-CSRF-Token")) {
			writeError(w, ErrorCodeForbidden, "invalid csrf token")
			return
		}
	}

	clientIP := s.ipExtractor.clientIP(r)

	cookie, err := r.Cookie("alga_session")
	if err != nil {
		writeError(w, ErrorCodeUnauthorized, "not authenticated")
		return
	}

	session, err := s.sessionStore.RefreshSession(cookie.Value, clientIP, r.UserAgent())
	if err != nil || session == nil {
		s.auditStore.Log(store.AuditLoginFailed, nil, "", clientIP, r.UserAgent(), false, map[string]any{
			"reason": "session_refresh_failed",
		})
		writeError(w, ErrorCodeUnauthorized, "session expired")
		return
	}

	// Get user for audit log
	user, _ := s.userStore.GetByID(session.UserID)
	if user != nil {
		s.auditStore.Log(store.AuditSessionRefreshed, &user.ID, user.Email, clientIP, r.UserAgent(), true, nil)
	}

	// Set new session cookie
	s.setSessionCookie(w, session.ID)

	// Re-issue CSRF cookie with the same value so its Max-Age stays aligned with the
	// sliding session (otherwise POSTs start failing while the session cookie is still valid).
	if csrfCookie, err := r.Cookie("alga_csrf"); err == nil && csrfCookie.Value != "" {
		s.setCSRFCookie(w, csrfCookie.Value)
	}

	writeStatus(w, "session refreshed")
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "not authenticated")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	// Validate new password
	if err := validatePasswordPolicy(req.NewPassword); err != nil {
		s.auditStore.Log(store.AuditPasswordChanged, &user.ID, user.Email, s.ipExtractor.clientIP(r), r.UserAgent(), false, map[string]any{
			"reason": "password_policy_violation",
			"error":  err.Error(),
		})
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
		return
	}

	// Verify current password
	_, err := s.userStore.Authenticate(user.Email, req.CurrentPassword)
	if err != nil {
		s.auditStore.Log(store.AuditPasswordChanged, &user.ID, user.Email, s.ipExtractor.clientIP(r), r.UserAgent(), false, map[string]any{
			"reason": "invalid_current_password",
		})
		writeError(w, ErrorCodeUnauthorized, "current password is incorrect")
		return
	}

	hash, err := hashPassword(req.NewPassword)
	if err != nil {
		writeError(w, ErrorCodeInternal, "failed to hash password")
		return
	}

	if err := s.userStore.UpdateUser(user.ID, map[string]any{"password": hash}); err != nil {
		s.auditStore.Log(store.AuditPasswordChanged, &user.ID, user.Email, s.ipExtractor.clientIP(r), r.UserAgent(), false, map[string]any{
			"reason": "update_failed",
		})
		writeError(w, ErrorCodeInternal, "failed to update password")
		return
	}

	// Invalidate all other sessions for security
	_ = s.sessionStore.DeleteAllUserSessions(user.ID)

	s.auditStore.Log(store.AuditPasswordChanged, &user.ID, user.Email, s.ipExtractor.clientIP(r), r.UserAgent(), true, nil)

	writeStatus(w, "password changed successfully")
}

func (s *Server) handleChangeEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "not authenticated")
		return
	}

	var req struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	if req.Password == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "password is required")
		return
	}

	_, err := s.userStore.Authenticate(user.Email, req.Password)
	if err != nil {
		s.auditStore.Log(store.AuditEmailChanged, &user.ID, user.Email, s.ipExtractor.clientIP(r), r.UserAgent(), false, map[string]any{
			"reason": "invalid_password",
		})
		writeError(w, ErrorCodeUnauthorized, "password is incorrect")
		return
	}

	email := strings.TrimSpace(req.Email)
	if email == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "email is required")
		return
	}

	existing, err := s.userStore.GetByEmail(email)
	if err != nil {
		writeInternalError(w, err, "failed to check email")
		return
	}
	if existing != nil && existing.ID != user.ID {
		writeError(w, ErrorCodeConflict, "email is already in use")
		return
	}

	if err := s.userStore.UpdateUser(user.ID, map[string]any{"email": email}); err != nil {
		s.auditStore.Log(store.AuditEmailChanged, &user.ID, user.Email, s.ipExtractor.clientIP(r), r.UserAgent(), false, map[string]any{
			"reason": "update_failed",
		})
		writeError(w, ErrorCodeInternal, "failed to update email")
		return
	}

	// Invalidate all sessions (including this one) so the user must
	// re-authenticate with the new email. Mirrors handleChangePassword
	// (ASVS V3.6/V2.5).
	_ = s.sessionStore.DeleteAllUserSessions(user.ID)

	s.auditStore.Log(store.AuditEmailChanged, &user.ID, user.Email, s.ipExtractor.clientIP(r), r.UserAgent(), true, map[string]any{
		"email":            email,
		"sessions_revoked": true,
	})

	writeStatus(w, "email updated successfully")
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "not authenticated")
		return
	}

	var req struct {
		FullName     string `json:"full_name"`
		Phone        string `json:"phone"`
		PhoneCountry string `json:"phone_country"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	normalized, err := validatePhoneNumber(strings.TrimSpace(req.Phone))
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
		return
	}

	phoneCountry := strings.ToUpper(strings.TrimSpace(req.PhoneCountry))
	resolvedCountry, err := validatePhoneCountry(phoneCountry, normalized)
	if err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
		return
	}

	if err := s.userStore.UpdateUser(user.ID, map[string]any{
		"full_name":     req.FullName,
		"phone":         normalized,
		"phone_country": resolvedCountry,
	}); err != nil {
		writeError(w, ErrorCodeInternal, "failed to update profile")
		return
	}

	s.audit(r, store.AuditUserUpdated, map[string]any{
		"user_id": user.ID.String(),
		"fields":  []string{"full_name", "phone", "phone_country"},
	})
	writeStatus(w, "profile updated successfully")
}

func (s *Server) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	if s.passwordResetStore == nil || s.emailSender == nil || !s.emailSender.Enabled() {
		writeErrorStatus(w, http.StatusServiceUnavailable, ErrorCodeInternal, "password reset is not configured")
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	email := strings.TrimSpace(req.Email)
	if email == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "email is required")
		return
	}

	const successMsg = "if the email exists, a reset link will be sent"

	user, err := s.userStore.GetByEmail(email)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": successMsg})
		return
	}
	if user == nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": successMsg})
		return
	}

	// Generate the reset token with the CSPRNG-bytes + base64url pattern used
	// by all other bearer tokens (sessions, PATs, agents, webhooks) rather than
	// a UUID, for consistency (ASVS V3.5, SPEC gap M5).
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		writeInternalError(w, err, "failed to generate reset token")
		return
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	tokenHash := algacrypto.Default().HMACString(token)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	expiresAt := time.Now().Add(1 * time.Hour)
	if _, err := s.passwordResetStore.CreateToken(ctx, user.ID, tokenHash, expiresAt); err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"message": successMsg})
		return
	}

	resetURL := fmt.Sprintf("/reset-password?token=%s", token)
	textBody := fmt.Sprintf("You requested a password reset for your Alga account.\n\nClick the link below to reset your password (valid for 1 hour):\n%s\n\nIf you did not request this, ignore this email.", resetURL)
	htmlBody := fmt.Sprintf("<p>You requested a password reset for your Alga account.</p><p><a href=\"%s\">Reset your password</a> (valid for 1 hour)</p><p>If you did not request this, ignore this email.</p>", resetURL)

	sendCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.emailSender.Send(sendCtx, user.Email, "Alga Password Reset", textBody, htmlBody); err != nil {
		logger.ErrorCtx(r.Context(), "failed to send password reset email", "email", user.Email, "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]string{"message": successMsg})
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Token == "" || req.NewPassword == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "token and new_password are required")
		return
	}

	if err := validatePasswordPolicy(req.NewPassword); err != nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, err.Error())
		return
	}

	tokenHash := algacrypto.Default().HMACString(req.Token)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resetToken, err := s.passwordResetStore.GetByTokenHash(ctx, tokenHash)
	if err != nil || resetToken == nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid or expired reset token")
		return
	}
	if resetToken.Used {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "reset token has already been used")
		return
	}
	if time.Now().After(resetToken.ExpiresAt) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "reset token has expired")
		return
	}

	user, err := s.userStore.GetByID(resetToken.UserID)
	if err != nil || user == nil {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "invalid or expired reset token")
		return
	}

	hash, err := hashPassword(req.NewPassword)
	if err != nil {
		writeInternalError(w, err, "failed to hash password")
		return
	}

	if err := s.userStore.UpdateUser(user.ID, map[string]any{"password": hash}); err != nil {
		writeInternalError(w, err, "failed to update password")
		return
	}

	if err := s.passwordResetStore.MarkUsed(ctx, resetToken.ID); err != nil {
		logger.Error("failed to mark reset token as used", "error", err)
	}

	_ = s.sessionStore.DeleteAllUserSessions(user.ID)

	s.auditStore.Log(store.AuditPasswordChanged, &user.ID, user.Email, s.ipExtractor.clientIP(r), r.UserAgent(), true, map[string]any{
		"reason": "password_reset",
	})

	writeStatus(w, "password reset successfully")
}

// validatePasswordPolicy checks if a password meets the security requirements
// maxPasswordLength bounds password input length to prevent Argon2 DoS on
// very long inputs (ASVS V2.5.4, SPEC gap L7). 1024 chars is far above any
// reasonable password yet cheap to hash.
const maxPasswordLength = 1024

func validatePasswordPolicy(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters long")
	}
	if len(password) > maxPasswordLength {
		return fmt.Errorf("password must be at most %d characters long", maxPasswordLength)
	}

	hasUpper := false
	hasLower := false
	hasDigit := false
	hasSpecial := false

	for _, char := range password {
		switch {
		case char >= 'A' && char <= 'Z':
			hasUpper = true
		case char >= 'a' && char <= 'z':
			hasLower = true
		case char >= '0' && char <= '9':
			hasDigit = true
		case strings.ContainsRune("!@#$%^&*()_+-=[]{}|;:,.<>?", char):
			hasSpecial = true
		}
	}

	if !hasUpper {
		return errors.New("password must contain at least one uppercase letter")
	}
	if !hasLower {
		return errors.New("password must contain at least one lowercase letter")
	}
	if !hasDigit {
		return errors.New("password must contain at least one digit")
	}
	if !hasSpecial {
		return errors.New("password must contain at least one special character (!@#$%^&*()_+-=[]{}|;:,.<>?)")
	}

	return nil
}

func validatePhoneNumber(phone string) (string, error) {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return "", nil
	}
	num, err := phonenumbers.Parse(phone, "")
	if err != nil {
		return "", errors.New("invalid phone number; use international format like +14155551234")
	}
	if !phonenumbers.IsValidNumber(num) {
		return "", errors.New("invalid phone number; use international format like +14155551234")
	}
	return phonenumbers.Format(num, phonenumbers.E164), nil
}

// validatePhoneCountry rejects a country code that does not match the
// libphonenumber-derived region for the number. This is a soft check: for
// numbers that span multiple countries sharing a prefix (e.g. +1 = NANP, +7 =
// RU/KZ), any of those regions are accepted. When the caller omits
// phone_country, the region detected from the number is used so the column is
// always populated for downstream display.
func validatePhoneCountry(country, phone string) (string, error) {
	if phone == "" {
		return country, nil
	}
	num, err := phonenumbers.Parse(phone, "")
	if err != nil {
		return country, errors.New("invalid phone number; use international format like +14155551234")
	}
	detected := phonenumbers.GetRegionCodeForNumber(num)
	if country == "" {
		return detected, nil
	}
	if !isCompatibleRegion(country, detected, num) {
		return country, errors.New("phone country does not match the phone number prefix")
	}
	return country, nil
}

// isCompatibleRegion reports whether the chosen region is one of the regions
// that share the number's country calling code. The detected region is always
// compatible; for the rare numbers libphonenumber cannot map to a single region
// (region code "ZZ" or empty), we accept any 2-letter country code and let the
// caller decide.
func isCompatibleRegion(chosen, detected string, num *phonenumbers.PhoneNumber) bool {
	if chosen == detected {
		return true
	}
	if detected == "" || detected == "ZZ" {
		return len(chosen) == 2
	}
	for _, r := range phonenumbers.GetRegionCodesForCountryCode(int(num.GetCountryCode())) {
		if r == chosen {
			return true
		}
	}
	return false
}

// setSessionCookie sets the session cookie with secure settings
func (s *Server) setSessionCookie(w http.ResponseWriter, sessionID string) {
	cookie := &http.Cookie{
		Name:     "alga_session",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(s.sessionExpiry.Seconds()),
	}
	http.SetCookie(w, cookie)
}

// clearSessionCookie clears the session cookie
func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     "alga_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	}
	http.SetCookie(w, cookie)
}

// setCSRFCookie sets the CSRF cookie
func (s *Server) setCSRFCookie(w http.ResponseWriter, token string) {
	cookie := &http.Cookie{
		Name:     "alga_csrf",
		Value:    token,
		Path:     "/",
		HttpOnly: false, // Must be accessible by JavaScript
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(s.sessionExpiry.Seconds()),
	}
	http.SetCookie(w, cookie)
}

// clearCSRFCookie clears the CSRF cookie
func (s *Server) clearCSRFCookie(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     "alga_csrf",
		Value:    "",
		Path:     "/",
		HttpOnly: false,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	}
	http.SetCookie(w, cookie)
}
