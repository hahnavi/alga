package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"alga/config"
	algacrypto "alga/crypto"
	"alga/logger"
	"alga/store"

	"github.com/google/uuid"
)

type userGoogleHandler struct {
	cfg        *config.Config
	userStore  store.UserStore
	auditStore store.AuditStore
	stateStore oauthStateStore
}

func newUserGoogleHandler(cfg *config.Config, userStore store.UserStore, auditStore store.AuditStore, stateStore oauthStateStore) *userGoogleHandler {
	return &userGoogleHandler{
		cfg:        cfg,
		userStore:  userStore,
		auditStore: auditStore,
		stateStore: stateStore,
	}
}

func (h *userGoogleHandler) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "not authenticated")
		return
	}

	if !GoogleOAuthEnabled(h.cfg) {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "Google Sign-In is not configured")
		return
	}

	if user.GoogleID != "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "Google account is already linked")
		return
	}

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		writeInternalError(w, err, "failed to generate OAuth state")
		return
	}
	state := fmt.Sprintf("user-google:%s:%s", user.ID.String(), hex.EncodeToString(stateBytes))

	if err := h.stateStore.Set(state); err != nil {
		writeInternalError(w, err, "failed to store OAuth state")
		return
	}

	redirectURI := h.getRedirectURI(r)
	authURL := fmt.Sprintf(
		"https://accounts.google.com/o/oauth2/v2/auth?client_id=%s&redirect_uri=%s&response_type=code&scope=%s&state=%s",
		url.QueryEscape(h.cfg.GoogleClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape("openid email profile"),
		url.QueryEscape(state),
	)

	writeJSON(w, http.StatusOK, map[string]string{"url": authURL})
}

func (h *userGoogleHandler) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	if !GoogleOAuthEnabled(h.cfg) {
		h.redirectResult(w, r, "error", "google_not_configured")
		return
	}

	q := r.URL.Query()
	if errParam := q.Get("error"); errParam != "" {
		h.redirectResult(w, r, "error", "google_auth_failed")
		return
	}

	code := q.Get("code")
	state := q.Get("state")
	if code == "" || state == "" {
		h.redirectResult(w, r, "error", "missing_code_or_state")
		return
	}

	valid, err := h.stateStore.Validate(state)
	if err != nil {
		logger.ErrorCtx(r.Context(), "user google oauth: state validation error", "component", "api", "error", err)
		h.redirectResult(w, r, "error", "state_validation_failed")
		return
	}
	if !valid {
		h.redirectResult(w, r, "error", "invalid_or_expired_state")
		return
	}

	if !strings.HasPrefix(state, "user-google:") {
		h.redirectResult(w, r, "error", "invalid_state_format")
		return
	}

	parts := strings.SplitN(state, ":", 3)
	if len(parts) < 3 {
		h.redirectResult(w, r, "error", "invalid_state_format")
		return
	}
	userID, err := uuid.Parse(parts[1])
	if err != nil {
		h.redirectResult(w, r, "error", "invalid_user_id")
		return
	}

	current, err := h.userStore.GetByID(userID)
	if err != nil || current == nil {
		h.redirectResult(w, r, "error", "user_not_found")
		return
	}

	tokenResp, err := ExchangeGoogleCode(code, h.cfg.GoogleClientID, h.cfg.GoogleClientSecret, h.getRedirectURI(r))
	if err != nil {
		logger.ErrorCtx(r.Context(), "user google oauth: token exchange failed", "component", "api", "error", err)
		h.redirectResult(w, r, "error", "token_exchange_failed")
		return
	}

	claims, err := parseIDTokenPayload(tokenResp.IDToken)
	if err != nil {
		logger.ErrorCtx(r.Context(), "user google oauth: id token parse failed", "component", "api", "error", err)
		h.redirectResult(w, r, "error", "id_token_invalid")
		return
	}

	if !algacrypto.ConstantTimeEqualString(claims.Audience, h.cfg.GoogleClientID) {
		h.redirectResult(w, r, "error", "google_audience_mismatch")
		return
	}

	if !claims.EmailVerified || claims.Email == "" {
		h.redirectResult(w, r, "error", "google_email_not_verified")
		return
	}

	if !strings.EqualFold(claims.Email, current.Email) {
		h.auditStore.Log(store.AuditGoogleLoginFailed, &current.ID, current.Email, r.RemoteAddr, r.UserAgent(), false, map[string]any{
			"reason":         "bind_email_mismatch",
			"google_email":   claims.Email,
			"google_subject": claims.Sub,
		})
		h.redirectResult(w, r, "error", "email_mismatch")
		return
	}

	if existing, _ := h.userStore.GetByGoogleID(claims.Sub); existing != nil && existing.ID != current.ID {
		h.auditStore.Log(store.AuditGoogleLoginFailed, &current.ID, current.Email, r.RemoteAddr, r.UserAgent(), false, map[string]any{
			"reason":         "bind_google_subject_already_linked",
			"google_subject": claims.Sub,
		})
		h.redirectResult(w, r, "error", "google_account_already_linked")
		return
	}

	if err := h.userStore.UpdateGoogleID(current.ID, claims.Sub); err != nil {
		logger.ErrorCtx(r.Context(), "user google oauth: failed to set google id", "component", "api", "user_id", current.ID.String(), "error", err)
		h.redirectResult(w, r, "error", "save_failed")
		return
	}

	h.auditStore.Log(store.AuditUserGoogleLinked, &current.ID, current.Email, r.RemoteAddr, r.UserAgent(), true, map[string]any{
		"google_email":   claims.Email,
		"google_subject": claims.Sub,
		"method":         "google_oauth_bind",
	})

	logger.InfoCtx(r.Context(), "user google oauth bind success", "component", "api", "user_id", current.ID.String(), "email", current.Email)

	h.redirectResult(w, r, "success", "")
}

func (h *userGoogleHandler) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "not authenticated")
		return
	}

	if user.GoogleID == "" {
		writeStatus(w, "disconnected")
		return
	}

	if err := h.userStore.ClearGoogleID(user.ID); err != nil {
		writeInternalError(w, err, "failed to disconnect Google account")
		return
	}

	previous := user.GoogleID
	h.auditStore.Log(store.AuditUserGoogleUnlinked, &user.ID, user.Email, "", "", true, map[string]any{
		"previous_google_subject": previous,
	})

	writeStatus(w, "disconnected")
}

func (h *userGoogleHandler) getRedirectURI(r *http.Request) string {
	if h.cfg.GoogleOAuthRedirectURL != "" {
		return h.cfg.GoogleOAuthRedirectURL + "/bind"
	}
	scheme := "https"
	if r.Header.Get("X-Forwarded-Proto") == "http" {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/api/v1/users/me/google/callback", scheme, r.Host)
}

func (h *userGoogleHandler) redirectResult(w http.ResponseWriter, r *http.Request, status, message string) {
	u := "/settings?google_linked=" + url.QueryEscape(status)
	if message != "" {
		u += "&message=" + url.QueryEscape(message)
	}

	scheme := "https"
	if r.Header.Get("X-Forwarded-Proto") == "http" {
		scheme = "http"
	}
	redirectURL := fmt.Sprintf("%s://%s%s", scheme, r.Host, u)

	http.Redirect(w, r, redirectURL, http.StatusFound)
}
