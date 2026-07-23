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

	"github.com/google/uuid"

	"alga/config"
	"alga/logger"
	"alga/store"
)

type userSlackHandler struct {
	cfg              *config.Config
	userStore        store.UserStore
	integrationStore store.IntegrationStore
	auditStore       store.AuditStore
	stateStore       oauthStateStore
}

func newUserSlackHandler(cfg *config.Config, userStore store.UserStore, integrationStore store.IntegrationStore, auditStore store.AuditStore, stateStore oauthStateStore) *userSlackHandler {
	return &userSlackHandler{
		cfg:              cfg,
		userStore:        userStore,
		integrationStore: integrationStore,
		auditStore:       auditStore,
		stateStore:       stateStore,
	}
}

func (h *userSlackHandler) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "not authenticated")
		return
	}

	if !h.isWorkspaceConnected() {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "Slack workspace is not connected")
		return
	}

	clientID, _ := h.getAppCredentials()
	if clientID == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "Slack app client ID is not configured")
		return
	}

	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		writeInternalError(w, err, "failed to generate OAuth state")
		return
	}
	randomHex := hex.EncodeToString(stateBytes)
	state := fmt.Sprintf("user-slack:%s:%s", user.ID.String(), randomHex)

	if err := h.stateStore.Set(state); err != nil {
		writeInternalError(w, err, "failed to store OAuth state")
		return
	}

	redirectURI := h.getRedirectURI(r)
	authURL := fmt.Sprintf(
		"https://slack.com/oauth/authorize?client_id=%s&scope=identity.basic&state=%s&redirect_uri=%s",
		url.QueryEscape(clientID),
		url.QueryEscape(state),
		url.QueryEscape(redirectURI),
	)

	writeJSON(w, http.StatusOK, map[string]string{"url": authURL})
}

func (h *userSlackHandler) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	q := r.URL.Query()
	if errParam := q.Get("error"); errParam != "" {
		h.redirectResult(w, r, "error", errParam)
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
		logger.Error("user slack oauth: state validation error", "error", err)
		h.redirectResult(w, r, "error", "state_validation_failed")
		return
	}
	if !valid {
		h.redirectResult(w, r, "error", "invalid_or_expired_state")
		return
	}

	if !strings.HasPrefix(state, "user-slack:") {
		h.redirectResult(w, r, "error", "invalid_state_format")
		return
	}

	parts := strings.SplitN(state, ":", 3)
	if len(parts) < 3 {
		h.redirectResult(w, r, "error", "invalid_state_format")
		return
	}
	userIDStr := parts[1]
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		h.redirectResult(w, r, "error", "invalid_user_id")
		return
	}

	clientID, clientSecret := h.getAppCredentials()
	if clientID == "" || clientSecret == "" {
		h.redirectResult(w, r, "error", "slack_app_not_configured")
		return
	}

	redirectURI := h.getRedirectURI(r)
	tokenResp, err := h.exchangeCode(code, clientID, clientSecret, redirectURI)
	if err != nil {
		logger.Error("user slack oauth: token exchange failed", "error", err)
		h.redirectResult(w, r, "error", "token_exchange_failed")
		return
	}

	if !tokenResp.OK {
		h.redirectResult(w, r, "error", "slack_refused:"+tokenResp.Error)
		return
	}

	identity, err := h.fetchIdentity(tokenResp.Token)
	if err != nil {
		logger.Error("user slack oauth: identity fetch failed", "error", err)
		h.redirectResult(w, r, "error", "identity_fetch_failed")
		return
	}

	if !identity.OK {
		h.redirectResult(w, r, "error", "identity_fetch_failed:"+identity.Error)
		return
	}

	displayName := identity.User.RealName
	if displayName == "" {
		displayName = identity.User.Name
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.userStore.SetSlackIdentity(ctx, userID, identity.User.ID, displayName); err != nil {
		logger.Error("user slack oauth: failed to set slack identity", "error", err)
		h.redirectResult(w, r, "error", "save_failed")
		return
	}

	h.auditStore.Log(store.AuditUserSlackLinked, &userID, "", "", "", false, map[string]any{
		"slack_user_id":   identity.User.ID,
		"slack_user_name": identity.User.Name,
		"slack_team_id":   identity.Team.ID,
	})

	h.redirectResult(w, r, "success", "")
}

func (h *userSlackHandler) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	user := userFromContext(r.Context())
	if user == nil {
		writeError(w, ErrorCodeUnauthorized, "not authenticated")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := h.userStore.ClearSlackIdentity(ctx, user.ID); err != nil {
		writeInternalError(w, err, "failed to disconnect Slack identity")
		return
	}

	h.auditStore.Log(store.AuditUserSlackUnlinked, &user.ID, user.Email, "", "", false, nil)

	writeStatus(w, "disconnected")
}

func (h *userSlackHandler) getRedirectURI(r *http.Request) string {
	if h.cfg.SlackOAuthRedirectURL != "" {
		return h.cfg.SlackOAuthRedirectURL + "/users"
	}
	scheme := "https"
	if r.Header.Get("X-Forwarded-Proto") == "http" {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/api/v1/users/me/slack/callback", scheme, r.Host)
}

func (h *userSlackHandler) getAppCredentials() (string, string) {
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

func (h *userSlackHandler) isWorkspaceConnected() bool {
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

type slackIdentityTokenResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Token string `json:"access_token,omitempty"`
}

type slackIdentityResponse struct {
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

func (h *userSlackHandler) exchangeCode(code, clientID, clientSecret, redirectURI string) (*slackIdentityTokenResponse, error) {
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
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var result slackIdentityTokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}

func (h *userSlackHandler) fetchIdentity(token string) (*slackIdentityResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://slack.com/api/users.identity", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var result slackIdentityResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}

func (h *userSlackHandler) redirectResult(w http.ResponseWriter, r *http.Request, status, message string) {
	u := "/settings?slack_linked=" + url.QueryEscape(status)
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
