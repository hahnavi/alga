package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"alga/config"
	"alga/logger"
	"alga/store"
)

type slackSignInHandler struct {
	cfg              *config.Config
	userStore        store.UserStore
	sessionStore     store.SessionStore
	auditStore       store.AuditStore
	integrationStore store.IntegrationStore
	stateStore       oauthStateStore
	sessionExpiry    time.Duration
}

func newSlackSignInHandler(cfg *config.Config, userStore store.UserStore, sessionStore store.SessionStore, auditStore store.AuditStore, integrationStore store.IntegrationStore, stateStore oauthStateStore, sessionExpiry time.Duration) *slackSignInHandler {
	return &slackSignInHandler{
		cfg:              cfg,
		userStore:        userStore,
		sessionStore:     sessionStore,
		auditStore:       auditStore,
		integrationStore: integrationStore,
		stateStore:       stateStore,
		sessionExpiry:    sessionExpiry,
	}
}

func (h *slackSignInHandler) enabled() bool {
	clientID, _ := h.getAppCredentials()
	return clientID != "" && h.isWorkspaceConnected()
}

func (h *slackSignInHandler) handleEnabled(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": h.enabled()})
}

func (h *slackSignInHandler) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		writeError(w, ErrorCodeNotFound, "Slack Sign-In is not configured")
		return
	}

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		writeInternalError(w, err, "failed to generate OAuth state")
		return
	}
	state := hex.EncodeToString(stateBytes)

	if err := h.stateStore.Set("slack-signin:" + state); err != nil {
		logger.ErrorCtx(r.Context(), "failed to store OAuth state", "component", "api", "error", err)
		writeInternalError(w, err, "failed to store OAuth state")
		return
	}

	clientID, _ := h.getAppCredentials()
	redirectURI := h.getRedirectURI(r)
	authURL := fmt.Sprintf(
		"https://slack.com/oauth/authorize?client_id=%s&scope=identity.basic&state=%s&redirect_uri=%s",
		url.QueryEscape(clientID),
		url.QueryEscape(state),
		url.QueryEscape(redirectURI),
	)

	http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *slackSignInHandler) handleCallback(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		http.Redirect(w, r, "/login?error=slack_not_configured", http.StatusFound)
		return
	}

	q := r.URL.Query()
	if errParam := q.Get("error"); errParam != "" {
		http.Redirect(w, r, "/login?error=slack_auth_failed", http.StatusFound)
		return
	}

	code := q.Get("code")
	state := q.Get("state")
	if code == "" || state == "" {
		http.Redirect(w, r, "/login?error=slack_auth_failed", http.StatusFound)
		return
	}

	valid, err := h.stateStore.Validate("slack-signin:" + state)
	if err != nil || !valid {
		logger.WarnCtx(r.Context(), "slack signin invalid state", "component", "api", "valid", valid, "error", err)
		http.Redirect(w, r, "/login?error=slack_invalid_state", http.StatusFound)
		return
	}

	clientID, clientSecret := h.getAppCredentials()
	if clientID == "" || clientSecret == "" {
		http.Redirect(w, r, "/login?error=slack_not_configured", http.StatusFound)
		return
	}

	redirectURI := h.getRedirectURI(r)
	tokenResp, err := h.exchangeCode(code, clientID, clientSecret, redirectURI)
	if err != nil {
		logger.ErrorCtx(r.Context(), "slack signin token exchange failed", "component", "api", "error", err)
		http.Redirect(w, r, "/login?error=slack_auth_failed", http.StatusFound)
		return
	}

	if !tokenResp.OK {
		http.Redirect(w, r, "/login?error=slack_auth_failed", http.StatusFound)
		return
	}

	identity, err := h.fetchIdentity(tokenResp.Token)
	if err != nil {
		logger.ErrorCtx(r.Context(), "slack signin identity fetch failed", "component", "api", "error", err)
		http.Redirect(w, r, "/login?error=slack_auth_failed", http.StatusFound)
		return
	}

	if !identity.OK {
		http.Redirect(w, r, "/login?error=slack_auth_failed", http.StatusFound)
		return
	}

	slackUserID := identity.User.ID

	var user *store.UserRecord
	if slackUserID != "" {
		user, _ = h.userStore.GetBySlackUserID(slackUserID)
	}
	if user == nil {
		h.auditStore.Log(store.AuditSlackLoginFailed, nil, "", r.RemoteAddr, r.UserAgent(), false, map[string]any{
			"reason":        "no_matching_account",
			"slack_user_id": slackUserID,
		})
		http.Redirect(w, r, "/login?error=slack_no_account", http.StatusFound)
		return
	}

	if user.LockedUntil != nil && user.LockedUntil.After(time.Now()) {
		h.auditStore.Log(store.AuditSlackLoginFailed, &user.ID, user.Email, r.RemoteAddr, r.UserAgent(), false, map[string]any{
			"reason": "account_locked",
		})
		http.Redirect(w, r, "/login?error=slack_account_locked", http.StatusFound)
		return
	}

	_ = h.userStore.RecordSuccessfulLogin(user.ID, r.RemoteAddr)

	h.auditStore.Log(store.AuditSlackLoginSuccess, &user.ID, user.Email, r.RemoteAddr, r.UserAgent(), true, map[string]any{
		"slack_user_id": slackUserID,
		"method":        "slack_oauth",
	})

	logger.InfoCtx(r.Context(), "slack signin login success", "component", "api", "user_id", user.ID.String(), "email", user.Email)

	session, err := h.sessionStore.CreateSession(user.ID, r.RemoteAddr, r.UserAgent())
	if err != nil {
		logger.ErrorCtx(r.Context(), "slack signin session creation failed", "component", "api", "user_id", user.ID.String(), "error", err)
		h.auditStore.Log(store.AuditSlackLoginFailed, &user.ID, user.Email, r.RemoteAddr, r.UserAgent(), false, map[string]any{
			"reason": "session_creation_failed",
			"error":  err.Error(),
		})
		http.Redirect(w, r, "/login?error=slack_auth_failed", http.StatusFound)
		return
	}

	csrfToken, err := generateCSRFToken()
	if err != nil {
		http.Redirect(w, r, "/login?error=slack_auth_failed", http.StatusFound)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "alga_session",
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.cfg.SecureCookies,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(h.sessionExpiry.Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "alga_csrf",
		Value:    csrfToken,
		Path:     "/",
		HttpOnly: false,
		Secure:   h.cfg.SecureCookies,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   int(h.sessionExpiry.Seconds()),
	})

	http.Redirect(w, r, "/login?slack=success", http.StatusFound)
}

func (h *slackSignInHandler) getRedirectURI(r *http.Request) string {
	if h.cfg.SlackOAuthRedirectURL != "" {
		return h.cfg.SlackOAuthRedirectURL + "/auth"
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/api/v1/auth/slack/callback", scheme, r.Host)
}

func (h *slackSignInHandler) getAppCredentials() (string, string) {
	if h.integrationStore == nil {
		return h.cfg.SlackClientID, h.cfg.SlackClientSecret
	}

	stored, err := h.integrationStore.Get()
	if err != nil || stored == nil {
		return h.cfg.SlackClientID, h.cfg.SlackClientSecret
	}

	clientID := h.cfg.SlackClientID
	if clientID == "" {
		clientID = stored.SlackClientID
	}
	clientSecret := h.cfg.SlackClientSecret
	if clientSecret == "" {
		clientSecret = stored.SlackClientSecret
	}
	return clientID, clientSecret
}

func (h *slackSignInHandler) isWorkspaceConnected() bool {
	if h.cfg.SlackBotToken != "" {
		return true
	}
	if h.integrationStore == nil {
		return false
	}
	stored, err := h.integrationStore.Get()
	if err != nil || stored == nil {
		return false
	}
	return stored.SlackBotToken != ""
}

type slackSignInTokenResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Token string `json:"access_token,omitempty"`
}

type slackSignInIdentityResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	User  struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		RealName string `json:"real_name,omitempty"`
	} `json:"user,omitempty"`
	Team struct {
		ID string `json:"id"`
	} `json:"team,omitempty"`
}

func (h *slackSignInHandler) exchangeCode(code, clientID, clientSecret, redirectURI string) (*slackSignInTokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/oauth.access", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read token response: %w", err)
	}

	var result slackSignInTokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse token response: %w", err)
	}
	return &result, nil
}

func (h *slackSignInHandler) fetchIdentity(token string) (*slackSignInIdentityResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://slack.com/api/users.identity", nil)
	if err != nil {
		return nil, fmt.Errorf("create identity request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("identity request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read identity response: %w", err)
	}

	var result slackSignInIdentityResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse identity response: %w", err)
	}
	return &result, nil
}
