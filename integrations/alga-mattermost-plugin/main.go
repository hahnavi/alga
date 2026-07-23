package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

const (
	webhookTimeout      = 10 * time.Second
	pluginUserAgent     = "Alga-Mattermost-Plugin/0.0.1"
	maxResponseBodySize = 4096
)

type configuration struct {
	AlgaWebhookURL string
	WebhookSecret  string
}

func (c *configuration) IsValid() error {
	if c.AlgaWebhookURL == "" {
		return fmt.Errorf("AlgaWebhookURL is required")
	}
	if c.WebhookSecret == "" {
		return fmt.Errorf("WebhookSecret is required")
	}
	u, err := url.Parse(c.AlgaWebhookURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook URL must use http or https scheme, got %q", u.Scheme)
	}
	if u.Scheme == "http" {
		// Log warning but allow http for dev environments
		fmt.Printf("[WARN] AlgaWebhookURL uses http instead of https. This is only appropriate for development.\n")
	}
	if u.Host == "" {
		return fmt.Errorf("webhook URL must have a host")
	}
	return nil
}

type ThreadReplyEvent struct {
	PostID    string `json:"post_id"`
	RootID    string `json:"root_id"`
	ChannelID string `json:"channel_id"`
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name"`
	Message   string `json:"message"`
	TeamID    string `json:"team_id"`
	Timestamp int64  `json:"timestamp"`
	EventType string `json:"event_type"`
}

type HealthResponse struct {
	Status string `json:"status"`
}

type AlgaPlugin struct {
	plugin.MattermostPlugin

	mu         sync.RWMutex
	config     *configuration
	httpClient *http.Client
	botUserID  string
}

func (p *AlgaPlugin) OnActivate() error {
	p.httpClient = &http.Client{
		Timeout: webhookTimeout,
	}

	botID, err := p.API.EnsureBotUser(&model.Bot{
		Username:    "alga",
		DisplayName: "Alga",
		Description: "Alga Investigation Sync bot",
	})
	if err != nil {
		return fmt.Errorf("failed to ensure bot user: %w", err)
	}
	p.botUserID = botID

	return p.OnConfigurationChange()
}

func (p *AlgaPlugin) OnConfigurationChange() error {
	var cfg configuration
	if err := p.API.LoadPluginConfiguration(&cfg); err != nil {
		return fmt.Errorf("failed to load plugin configuration: %w", err)
	}

	if err := cfg.IsValid(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	p.mu.Lock()
	p.config = &cfg
	p.mu.Unlock()

	return nil
}

func (p *AlgaPlugin) getConfig() *configuration {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.config
}

func (p *AlgaPlugin) checkPluginSecret(w http.ResponseWriter, r *http.Request) bool {
	cfg := p.getConfig()
	if cfg == nil || cfg.WebhookSecret == "" {
		http.Error(w, "plugin not configured", http.StatusServiceUnavailable)
		return false
	}
	authHeader := r.Header.Get("Authorization")
	if !isValidBearer(authHeader, cfg.WebhookSecret) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func isValidBearer(header, secret string) bool {
	if len(header) < 7 || header[:7] != "Bearer " {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(header[7:]), []byte(secret)) == 1
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (p *AlgaPlugin) MessageHasBeenPosted(c *plugin.Context, post *model.Post) {
	if post.RootId == "" {
		return
	}

	if post.UserId == "" {
		return
	}

	cfg := p.getConfig()
	if cfg == nil || cfg.AlgaWebhookURL == "" {
		return
	}

	if p.botUserID != "" && post.UserId == p.botUserID {
		return
	}

	user, appErr := p.API.GetUser(post.UserId)
	if appErr != nil {
		p.API.LogError("Failed to resolve user for thread reply",
			"user_id", post.UserId,
			"error", appErr.Error(),
		)
		return
	}

	var teamID string
	if channel, appErr := p.API.GetChannel(post.ChannelId); appErr != nil {
		p.API.LogWarn("Failed to resolve channel for thread reply, omitting team_id",
			"channel_id", post.ChannelId,
			"error", appErr.Error(),
		)
	} else {
		teamID = channel.TeamId
	}

	event := ThreadReplyEvent{
		PostID:    post.Id,
		RootID:    post.RootId,
		ChannelID: post.ChannelId,
		UserID:    post.UserId,
		UserName:  user.Username,
		Message:   post.Message,
		TeamID:    teamID,
		Timestamp: post.CreateAt,
		EventType: "thread_reply",
	}

	if err := p.forwardEvent(cfg, &event); err != nil {
		p.API.LogError("Failed to forward thread reply to Alga",
			"post_id", post.Id,
			"root_id", post.RootId,
			"channel_id", post.ChannelId,
			"error", err.Error(),
		)
		return
	}

	p.API.LogDebug("Forwarded thread reply to Alga",
		"post_id", post.Id,
		"root_id", post.RootId,
		"channel_id", post.ChannelId,
	)
}

func (p *AlgaPlugin) forwardEvent(cfg *configuration, event *ThreadReplyEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), webhookTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.AlgaWebhookURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", pluginUserAgent)
	if cfg.WebhookSecret != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.WebhookSecret)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBodySize))
		return fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (p *AlgaPlugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/health":
		p.handleHealth(w, r)
	case r.URL.Path == "/api/v1/post" && r.Method == http.MethodPost:
		if p.checkPluginSecret(w, r) {
			p.handleCreatePost(w, r)
		}
	case r.URL.Path == "/api/v1/reply" && r.Method == http.MethodPost:
		if p.checkPluginSecret(w, r) {
			p.handleReply(w, r)
		}
	case r.URL.Path == "/api/v1/update-post" && r.Method == http.MethodPut:
		if p.checkPluginSecret(w, r) {
			p.handleUpdatePost(w, r)
		}
	case r.URL.Path == "/api/v1/channel":
		if p.checkPluginSecret(w, r) {
			p.handleGetChannelByName(w, r)
		}
	case r.URL.Path == "/api/v1/channels":
		if p.checkPluginSecret(w, r) {
			p.handleListChannels(w, r)
		}
	case r.URL.Path == "/api/v1/username":
		if p.checkPluginSecret(w, r) {
			p.handleGetUsername(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}

func (p *AlgaPlugin) handleHealth(w http.ResponseWriter, _ *http.Request) {
	cfg := p.getConfig()
	status := "configured"
	if cfg == nil || cfg.AlgaWebhookURL == "" {
		status = "not_configured"
	}

	writeJSON(w, http.StatusOK, HealthResponse{Status: status})
}

type createPostRequest struct {
	ChannelID string                 `json:"channel_id"`
	Message   string                 `json:"message"`
	Props     map[string]interface{} `json:"props,omitempty"`
}

func (p *AlgaPlugin) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req createPostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ChannelID == "" {
		writeError(w, http.StatusBadRequest, "channel_id is required")
		return
	}

	post := &model.Post{
		UserId:    p.botUserID,
		ChannelId: req.ChannelID,
		Message:   req.Message,
	}
	if req.Props != nil {
		post.SetProps(req.Props)
	}

	created, appErr := p.API.CreatePost(post)
	if appErr != nil {
		writeError(w, http.StatusInternalServerError, appErr.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"id": created.Id})
}

type replyRequest struct {
	RootPostID string                 `json:"root_post_id"`
	Message    string                 `json:"message"`
	Props      map[string]interface{} `json:"props,omitempty"`
}

func (p *AlgaPlugin) handleReply(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req replyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RootPostID == "" {
		writeError(w, http.StatusBadRequest, "root_post_id is required")
		return
	}

	rootPost, appErr := p.API.GetPost(req.RootPostID)
	if appErr != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("root post not found: %s", appErr.Error()))
		return
	}

	post := &model.Post{
		UserId:    p.botUserID,
		ChannelId: rootPost.ChannelId,
		RootId:    req.RootPostID,
		Message:   req.Message,
	}
	if req.Props != nil {
		post.SetProps(req.Props)
	}

	created, appErr := p.API.CreatePost(post)
	if appErr != nil {
		writeError(w, http.StatusInternalServerError, appErr.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"id": created.Id, "channel_id": rootPost.ChannelId})
}

type updatePostRequest struct {
	PostID  string                 `json:"post_id"`
	Message string                 `json:"message"`
	Props   map[string]interface{} `json:"props,omitempty"`
}

func (p *AlgaPlugin) handleUpdatePost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req updatePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.PostID == "" {
		writeError(w, http.StatusBadRequest, "post_id is required")
		return
	}

	existing, appErr := p.API.GetPost(req.PostID)
	if appErr != nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("post not found: %s", appErr.Error()))
		return
	}

	existing.Message = req.Message
	if req.Props != nil {
		existing.SetProps(req.Props)
	}

	_, appErr = p.API.UpdatePost(existing)
	if appErr != nil {
		writeError(w, http.StatusInternalServerError, appErr.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (p *AlgaPlugin) handleGetChannelByName(w http.ResponseWriter, r *http.Request) {
	channelName := r.URL.Query().Get("name")
	if channelName == "" {
		writeError(w, http.StatusBadRequest, "name query parameter is required")
		return
	}

	teamName := r.URL.Query().Get("team")

	if teamName != "" {
		ch, appErr := p.API.GetChannelByNameForTeamName(teamName, channelName, false)
		if appErr != nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("channel %q not found in team %q: %s", channelName, teamName, appErr.Error()))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"id": ch.Id, "name": ch.Name})
		return
	}

	teams, appErr := p.API.GetTeams()
	if appErr != nil {
		writeError(w, http.StatusInternalServerError, appErr.Error())
		return
	}

	for _, team := range teams {
		ch, appErr := p.API.GetChannelByName(team.Id, channelName, false)
		if appErr == nil && ch != nil {
			writeJSON(w, http.StatusOK, map[string]string{"id": ch.Id, "name": ch.Name})
			return
		}
	}

	writeError(w, http.StatusNotFound, fmt.Sprintf("channel %q not found", channelName))
}

func (p *AlgaPlugin) handleListChannels(w http.ResponseWriter, r *http.Request) {
	teamName := r.URL.Query().Get("team")

	type channel struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	if teamName != "" {
		team, appErr := p.API.GetTeamByName(teamName)
		if appErr != nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("team %q not found: %s", teamName, appErr.Error()))
			return
		}
		channels, appErr := p.API.GetChannelsForTeamForUser(team.Id, p.botUserID, false)
		if appErr != nil {
			writeError(w, http.StatusInternalServerError, appErr.Error())
			return
		}
		result := make([]channel, 0, len(channels))
		for _, ch := range channels {
			result = append(result, channel{ID: ch.Id, Name: ch.Name})
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	teams, appErr := p.API.GetTeams()
	if appErr != nil {
		writeError(w, http.StatusInternalServerError, appErr.Error())
		return
	}

	var allChannels []channel
	for _, team := range teams {
		channels, appErr := p.API.GetChannelsForTeamForUser(team.Id, p.botUserID, false)
		if appErr != nil {
			continue
		}
		for _, ch := range channels {
			allChannels = append(allChannels, channel{ID: ch.Id, Name: ch.Name})
		}
	}

	if allChannels == nil {
		allChannels = []channel{}
	}
	writeJSON(w, http.StatusOK, allChannels)
}

func (p *AlgaPlugin) handleGetUsername(w http.ResponseWriter, r *http.Request) {
	if p.botUserID == "" {
		writeError(w, http.StatusNotFound, "bot user not initialized")
		return
	}

	user, appErr := p.API.GetUser(p.botUserID)
	if appErr != nil {
		writeError(w, http.StatusInternalServerError, appErr.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"username": user.Username})
}

func main() {
	plugin.ClientMain(&AlgaPlugin{})
}
