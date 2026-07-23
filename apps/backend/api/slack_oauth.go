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
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"alga/config"
	"alga/logger"
	"alga/slack"
	"alga/store"
	"alga/valkey"
)

type oauthStateStore interface {
	Set(state string) error
	Validate(state string) (bool, error)
}

type valkeyOAuthStateStore struct {
	client *valkey.Client
}

func (s *valkeyOAuthStateStore) Set(state string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := s.client.SetNX(ctx, "alga:oauth-state:"+state, "1", 10*time.Minute)
	if err != nil {
		return fmt.Errorf("oauth state store: %w", err)
	}
	return nil
}

func (s *valkeyOAuthStateStore) Validate(state string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	key := "alga:oauth-state:" + state
	cmd := s.client.Builder().Get().Key(key).Build()
	result := s.client.Do(ctx, cmd)
	_, err := result.ToString()
	if err != nil {
		return false, nil
	}
	_ = s.client.Del(ctx, key)
	return true, nil
}

type memoryOAuthStateStore struct {
	mu     sync.Mutex
	states map[string]time.Time
	stopCh chan struct{}
}

func newMemoryOAuthStateStore() *memoryOAuthStateStore {
	s := &memoryOAuthStateStore{
		states: make(map[string]time.Time),
		stopCh: make(chan struct{}),
	}
	go s.cleanup()
	return s
}

func (s *memoryOAuthStateStore) cleanup() {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("oauth state store cleanup panicked", "component", "api", "panic", r, "stack", string(debug.Stack()))
		}
	}()
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for k, t := range s.states {
				if now.After(t) {
					delete(s.states, k)
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *memoryOAuthStateStore) Stop() {
	close(s.stopCh)
}

func (s *memoryOAuthStateStore) Set(state string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state] = time.Now().Add(10 * time.Minute)
	return nil
}

func (s *memoryOAuthStateStore) Validate(state string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry, ok := s.states[state]
	if !ok {
		return false, nil
	}
	delete(s.states, state)
	return time.Now().Before(expiry), nil
}

type slackReconfigurable interface {
	Reconfigure(botToken string)
}

type slackOAuthHandler struct {
	cfg              *config.Config
	integrationStore store.IntegrationStore
	slackClient      slackReconfigurable
	stateStore       oauthStateStore
	rebuildFn        func()
}

func newSlackOAuthHandler(cfg *config.Config, integrationStore store.IntegrationStore, slackClient *slack.Client, vkClient *valkey.Client, rebuildFn func()) *slackOAuthHandler {
	h := &slackOAuthHandler{
		cfg:              cfg,
		integrationStore: integrationStore,
		slackClient:      slackClient,
		rebuildFn:        rebuildFn,
	}
	if vkClient != nil {
		h.stateStore = &valkeyOAuthStateStore{client: vkClient}
	} else {
		h.stateStore = newMemoryOAuthStateStore()
	}
	return h
}

func (h *slackOAuthHandler) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorStatus(w, http.StatusMethodNotAllowed, ErrorCodeInternal, "method not allowed")
		return
	}

	clientID, clientSecret := h.getSlackAppCredentials()
	if clientID == "" || clientSecret == "" {
		writeErrorStatus(w, http.StatusBadRequest, ErrorCodeValidationFailed, "Slack app credentials (client_id and client_secret) are not configured")
		return
	}

	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		writeInternalError(w, err, "failed to generate OAuth state")
		return
	}
	state := hex.EncodeToString(stateBytes)

	if err := h.stateStore.Set("slack:" + state); err != nil {
		writeInternalError(w, err, "failed to store OAuth state")
		return
	}

	redirectURI := h.getRedirectURI(r)
	scopes := "chat:write,chat:write.customize,chat:write.public,channels:read,groups:read,channels:manage,groups:manage,channels:history,groups:history,im:write,mpim:write"
	authURL := fmt.Sprintf(
		"https://slack.com/oauth/v2/authorize?client_id=%s&scope=%s&state=%s&redirect_uri=%s",
		url.QueryEscape(clientID),
		url.QueryEscape(scopes),
		url.QueryEscape(state),
		url.QueryEscape(redirectURI),
	)

	writeJSON(w, http.StatusOK, map[string]string{"url": authURL})
}

func (h *slackOAuthHandler) handleCallback(w http.ResponseWriter, r *http.Request) {
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

	valid, err := h.stateStore.Validate("slack:" + state)
	if err != nil {
		logger.Error("slack oauth: state validation error", "error", err)
		h.redirectResult(w, r, "error", "state_validation_failed")
		return
	}
	if !valid {
		h.redirectResult(w, r, "error", "invalid_or_expired_state")
		return
	}

	clientID, clientSecret := h.getSlackAppCredentials()
	if clientID == "" || clientSecret == "" {
		h.redirectResult(w, r, "error", "slack_app_not_configured")
		return
	}

	redirectURI := h.getRedirectURI(r)
	tokenResp, err := h.exchangeCode(code, clientID, clientSecret, redirectURI)
	if err != nil {
		logger.Error("slack oauth: token exchange failed", "error", err)
		h.redirectResult(w, r, "error", "token_exchange_failed")
		return
	}

	if !tokenResp.OK {
		h.redirectResult(w, r, "error", "slack_refused:"+tokenResp.Error)
		return
	}

	if h.integrationStore == nil {
		h.redirectResult(w, r, "error", "integration_store_unavailable")
		return
	}

	existing, err := h.integrationStore.Get()
	if err != nil {
		logger.Error("slack oauth: failed to load existing config", "error", err)
		h.redirectResult(w, r, "error", "internal_error")
		return
	}
	if existing == nil {
		existing = &store.IntegrationConfig{}
	}

	existing.SlackBotToken = tokenResp.AccessToken
	existing.SlackWorkspaceName = tokenResp.Team.Name
	existing.SlackWorkspaceID = tokenResp.Team.ID
	if clientID != "" {
		existing.SlackClientID = clientID
	}
	if clientSecret != "" {
		existing.SlackClientSecret = clientSecret
	}

	if err := h.integrationStore.Save(*existing); err != nil {
		logger.Error("slack oauth: failed to save config", "error", err)
		h.redirectResult(w, r, "error", "save_failed")
		return
	}

	h.cfg.SlackBotToken = tokenResp.AccessToken
	h.cfg.SlackWorkspaceName = tokenResp.Team.Name
	h.cfg.SlackWorkspaceID = tokenResp.Team.ID
	h.slackClient.Reconfigure(tokenResp.AccessToken)
	if h.rebuildFn != nil {
		h.rebuildFn()
	}

	h.redirectResult(w, r, "success", "")
}

type slackTokenResponse struct {
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
	AccessToken string `json:"access_token,omitempty"`
	Team        struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	} `json:"team,omitempty"`
	BotUserID string `json:"bot_user_id,omitempty"`
}

func (h *slackOAuthHandler) exchangeCode(code, clientID, clientSecret, redirectURI string) (*slackTokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/oauth.v2.access", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token exchange request: %w", err)
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

	var result slackTokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}

func (h *slackOAuthHandler) getRedirectURI(r *http.Request) string {
	if h.cfg.SlackOAuthRedirectURL != "" {
		return h.cfg.SlackOAuthRedirectURL
	}
	scheme := "https"
	if r.Header.Get("X-Forwarded-Proto") == "http" {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/api/v1/integrations/slack/oauth/callback", scheme, r.Host)
}

func (h *slackOAuthHandler) redirectResult(w http.ResponseWriter, r *http.Request, status, message string) {
	u := "/integrations?slack_oauth=" + url.QueryEscape(status)
	if message != "" {
		u += "&message=" + url.QueryEscape(message)
	}

	http.Redirect(w, r, u, http.StatusFound)
}

func (h *slackOAuthHandler) getSlackAppCredentials() (string, string) {
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
