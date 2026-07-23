package mattermost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"alga/internal/httpclient"
	"alga/logger"
)

var channelNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func validChannelName(name string) bool {
	return channelNameRegex.MatchString(name) && len(name) <= 64
}

type Client struct {
	mu         sync.RWMutex
	disabled   bool
	pluginURL  string
	secret     string
	teamSlug   string
	httpClient *http.Client
}

type Channel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func NewClient(pluginURL, secret, teamSlug string) *Client {
	return &Client{
		pluginURL:  pluginURL,
		secret:     secret,
		teamSlug:   teamSlug,
		httpClient: httpclient.NewTimeoutClient(15 * time.Second),
	}
}

func (c *Client) Enabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return !c.disabled && c.pluginURL != "" && c.secret != ""
}

// SetDisabled marks the integration inactive while keeping URL/secret for re‑enable.
func (c *Client) SetDisabled(disabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.disabled = disabled
}

func (c *Client) Reconfigure(pluginURL, secret, teamSlug string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pluginURL = pluginURL
	c.secret = secret
	c.teamSlug = teamSlug
}

func (c *Client) TestConnection(ctx context.Context) error {
	c.mu.RLock()
	pURL := c.pluginURL
	secret := c.secret
	c.mu.RUnlock()
	if pURL == "" || secret == "" {
		return fmt.Errorf("mattermost plugin not configured (url=%q, secret=%q)", pURL, "REDACTED")
	}
	_, _, err := c.doRequest(ctx, http.MethodGet, "/health", nil)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	return nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body any) (int, []byte, error) {
	c.mu.RLock()
	pluginURL := c.pluginURL
	secret := c.secret
	c.mu.RUnlock()

	var bodyReader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(payload)
	}

	baseURL := strings.TrimRight(pluginURL, "/")
	u := baseURL + "/plugins/com.alga.mattermost-plugin" + path
	headers := map[string]string{
		"Authorization": "Bearer " + secret,
		"Content-Type":  "application/json",
	}

	status, respBody, err := httpclient.DoJSON(ctx, c.httpClient, method, u, headers, bodyReader)
	if err != nil {
		return status, respBody, fmt.Errorf("request to %s %s failed: %w", method, path, err)
	}

	if status < 200 || status >= 300 {
		return status, respBody, fmt.Errorf("plugin API returned status %d: %s", status, string(respBody))
	}

	return status, respBody, nil
}

func (c *Client) GetChannelByName(ctx context.Context, channelName string) (string, error) {
	if !validChannelName(channelName) {
		return "", errors.New("invalid channel name: must contain only alphanumeric, dash, or underscore characters")
	}

	c.mu.RLock()
	teamSlug := c.teamSlug
	c.mu.RUnlock()

	path := fmt.Sprintf("/api/v1/channel?name=%s", url.QueryEscape(channelName))
	if teamSlug != "" {
		path += "&team=" + url.QueryEscape(teamSlug)
	}
	status, body, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get channel %q: %w", channelName, err)
	}

	if status != http.StatusOK {
		return "", fmt.Errorf("channel %q not found (status %d): %s", channelName, status, string(body))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to decode channel response: %w", err)
	}

	return result.ID, nil
}

func (c *Client) ListChannels(ctx context.Context) ([]Channel, error) {
	c.mu.RLock()
	teamSlug := c.teamSlug
	c.mu.RUnlock()

	path := "/api/v1/channels"
	if teamSlug != "" {
		path += "?team=" + url.QueryEscape(teamSlug)
	}
	_, body, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list channels: %w", err)
	}

	var channels []Channel
	if err := json.Unmarshal(body, &channels); err != nil {
		return nil, fmt.Errorf("failed to decode channels response: %w", err)
	}
	return channels, nil
}

func (c *Client) CreatePost(ctx context.Context, channelID, message string, props map[string]any) (string, error) {
	postBody := map[string]any{
		"channel_id": channelID,
		"message":    message,
	}
	if props != nil {
		postBody["props"] = props
	}

	_, body, err := c.doRequest(ctx, http.MethodPost, "/api/v1/post", postBody)
	if err != nil {
		return "", fmt.Errorf("failed to create post: %w", err)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to decode post response: %w", err)
	}

	logger.Debug("Created Mattermost post", "component", "mattermost", "post_id", result.ID, "channel_id", channelID)
	return result.ID, nil
}

func (c *Client) UpdatePost(ctx context.Context, postID string, message string, props map[string]any) error {
	updateBody := map[string]any{
		"post_id": postID,
		"message": message,
	}
	if props != nil {
		updateBody["props"] = props
	}

	_, _, err := c.doRequest(ctx, http.MethodPut, "/api/v1/update-post", updateBody)
	if err != nil {
		return fmt.Errorf("failed to update post %s: %w", postID, err)
	}

	logger.Debug("Updated Mattermost post", "component", "mattermost", "post_id", postID)
	return nil
}

func (c *Client) GetUsername(ctx context.Context) (string, error) {
	_, body, err := c.doRequest(ctx, http.MethodGet, "/api/v1/username", nil)
	if err != nil {
		return "", fmt.Errorf("failed to get bot username: %w", err)
	}
	var result struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to decode username response: %w", err)
	}
	return result.Username, nil
}

func (c *Client) ReplyToPost(ctx context.Context, rootPostID string, message string, props map[string]any) (string, error) {
	replyBody := map[string]any{
		"root_post_id": rootPostID,
		"message":      message,
	}
	if props != nil {
		replyBody["props"] = props
	}

	_, respBody, err := c.doRequest(ctx, http.MethodPost, "/api/v1/reply", replyBody)
	if err != nil {
		return "", fmt.Errorf("failed to create reply: %w", err)
	}

	var result struct {
		ID        string `json:"id"`
		ChannelID string `json:"channel_id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to decode reply response: %w", err)
	}

	logger.Debug("Created Mattermost reply", "component", "mattermost", "reply_id", result.ID, "root_post_id", rootPostID)
	return result.ID, nil
}
