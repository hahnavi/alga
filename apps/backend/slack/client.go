package slack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"alga/internal/httpclient"
)

type Client struct {
	mu          sync.RWMutex
	botToken    string
	httpClient  *http.Client
	avatarCache map[string]avatarEntry
}

type avatarEntry struct {
	url       string
	expiresAt time.Time
}

func NewClient(botToken string) *Client {
	return &Client{
		botToken:    botToken,
		httpClient:  httpclient.NewTimeoutClient(15 * time.Second),
		avatarCache: make(map[string]avatarEntry),
	}
}

func (c *Client) Enabled() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.botToken != ""
}

func (c *Client) Reconfigure(botToken string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.botToken = botToken
}

func (c *Client) TestConnection(ctx context.Context) error {
	if !c.Enabled() {
		return errors.New("slack bot token not configured")
	}
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	values := url.Values{}
	if err := c.postForm(ctx, "auth.test", values, &res); err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	if !res.OK {
		return fmt.Errorf("slack auth.test failed: %s", res.Error)
	}
	return nil
}

func (c *Client) GetBotUserID(ctx context.Context) (string, error) {
	if !c.Enabled() {
		return "", errors.New("slack bot token not configured")
	}
	var res struct {
		OK     bool   `json:"ok"`
		Error  string `json:"error"`
		UserID string `json:"user_id"`
	}
	values := url.Values{}
	if err := c.postForm(ctx, "auth.test", values, &res); err != nil {
		return "", fmt.Errorf("failed to get bot user id: %w", err)
	}
	if !res.OK {
		return "", fmt.Errorf("slack auth.test failed: %s", res.Error)
	}
	return res.UserID, nil
}

func (c *Client) postForm(ctx context.Context, method string, values url.Values, out any) error {
	c.mu.RLock()
	token := c.botToken
	c.mu.RUnlock()
	if token == "" {
		return errors.New("slack bot token is not configured")
	}

	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/x-www-form-urlencoded",
	}
	urlStr := "https://slack.com/api/" + method
	body := strings.NewReader(values.Encode())

	_, respBody, err := httpclient.DoJSON(ctx, c.httpClient, http.MethodPost, urlStr, headers, body)
	if err != nil {
		return fmt.Errorf("slack request failed: %w", err)
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("failed decoding slack response: %w", err)
	}
	return nil
}

// PostThreadReply posts a message in an existing thread (Slack root message ts as threadTS).
// The returned messageTS is the new message timestamp (for chat.update); empty if the API omits it.
func (c *Client) PostThreadReply(ctx context.Context, channelID, threadTS, text string, customize ...PostCustomize) (messageTS string, err error) {
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		TS    string `json:"ts"`
	}
	values := url.Values{
		"channel":   {channelID},
		"thread_ts": {threadTS},
		"text":      {text},
	}
	if len(customize) > 0 {
		cz := customize[0]
		if cz.Username != "" {
			values.Set("username", cz.Username)
		}
		if cz.IconURL != "" {
			values.Set("icon_url", cz.IconURL)
		}
	}
	if err := c.postForm(ctx, "chat.postMessage", values, &res); err != nil {
		return "", err
	}
	if !res.OK {
		return "", fmt.Errorf("slack chat.postMessage (thread) failed: %s", res.Error)
	}
	return res.TS, nil
}

func (c *Client) PostMessage(ctx context.Context, channelID, text string) (string, error) {
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		TS    string `json:"ts"`
	}
	values := url.Values{
		"channel": {channelID},
		"text":    {text},
	}
	if err := c.postForm(ctx, "chat.postMessage", values, &res); err != nil {
		return "", err
	}
	if !res.OK {
		return "", fmt.Errorf("slack chat.postMessage failed: %s", res.Error)
	}
	return res.TS, nil
}

func (c *Client) PostMessageWithBlocks(ctx context.Context, channelID, fallbackText string, blocks []Block) (string, error) {
	blocksJSON, err := EncodeBlocks(blocks)
	if err != nil {
		return "", fmt.Errorf("failed to encode blocks: %w", err)
	}
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		TS    string `json:"ts"`
	}
	values := url.Values{
		"channel": {channelID},
		"text":    {fallbackText},
		"blocks":  {blocksJSON},
	}
	if err := c.postForm(ctx, "chat.postMessage", values, &res); err != nil {
		return "", err
	}
	if !res.OK {
		return "", fmt.Errorf("slack chat.postMessage (blocks) failed: %s", res.Error)
	}
	return res.TS, nil
}

func (c *Client) PostMessageWithAttachments(ctx context.Context, channelID, fallbackText string, attachments []Attachment) (string, error) {
	attachmentsJSON, err := EncodeAttachments(attachments)
	if err != nil {
		return "", fmt.Errorf("failed to encode attachments: %w", err)
	}
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		TS    string `json:"ts"`
	}
	values := url.Values{
		"channel":     {channelID},
		"text":        {fallbackText},
		"attachments": {attachmentsJSON},
	}
	if err := c.postForm(ctx, "chat.postMessage", values, &res); err != nil {
		return "", err
	}
	if !res.OK {
		return "", fmt.Errorf("slack chat.postMessage (attachments) failed: %s", res.Error)
	}
	return res.TS, nil
}

func (c *Client) PostThreadReplyWithBlocks(ctx context.Context, channelID, threadTS, fallbackText string, blocks []Block) (string, error) {
	blocksJSON, err := EncodeBlocks(blocks)
	if err != nil {
		return "", fmt.Errorf("failed to encode blocks: %w", err)
	}
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		TS    string `json:"ts"`
	}
	values := url.Values{
		"channel":   {channelID},
		"thread_ts": {threadTS},
		"text":      {fallbackText},
		"blocks":    {blocksJSON},
	}
	if err := c.postForm(ctx, "chat.postMessage", values, &res); err != nil {
		return "", err
	}
	if !res.OK {
		return "", fmt.Errorf("slack chat.postMessage (thread+blocks) failed: %s", res.Error)
	}
	return res.TS, nil
}

func (c *Client) UpdateMessageWithBlocks(ctx context.Context, channelID, ts, fallbackText string, blocks []Block) error {
	blocksJSON, err := EncodeBlocks(blocks)
	if err != nil {
		return fmt.Errorf("failed to encode blocks: %w", err)
	}
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	values := url.Values{
		"channel": {channelID},
		"ts":      {ts},
		"text":    {fallbackText},
		"blocks":  {blocksJSON},
	}
	if err := c.postForm(ctx, "chat.update", values, &res); err != nil {
		return err
	}
	if !res.OK {
		return fmt.Errorf("slack chat.update (blocks) failed: %s", res.Error)
	}
	return nil
}

func (c *Client) UpdateMessageWithAttachments(ctx context.Context, channelID, ts, fallbackText string, attachments []Attachment) error {
	attachmentsJSON, err := EncodeAttachments(attachments)
	if err != nil {
		return fmt.Errorf("failed to encode attachments: %w", err)
	}
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	values := url.Values{
		"channel":     {channelID},
		"ts":          {ts},
		"text":        {fallbackText},
		"attachments": {attachmentsJSON},
		"blocks":      {"[]"},
	}
	if err := c.postForm(ctx, "chat.update", values, &res); err != nil {
		return err
	}
	if !res.OK {
		return fmt.Errorf("slack chat.update (attachments) failed: %s", res.Error)
	}
	return nil
}

func (c *Client) UpdateMessage(ctx context.Context, channelID, ts, text string, customize ...PostCustomize) error {
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	values := url.Values{
		"channel": {channelID},
		"ts":      {ts},
		"text":    {text},
	}
	if len(customize) > 0 {
		cz := customize[0]
		if cz.Username != "" {
			values.Set("username", cz.Username)
		}
		if cz.IconURL != "" {
			values.Set("icon_url", cz.IconURL)
		}
	}
	if err := c.postForm(ctx, "chat.update", values, &res); err != nil {
		return err
	}
	if !res.OK {
		return fmt.Errorf("slack chat.update failed: %s", res.Error)
	}
	return nil
}

func (c *Client) GetPermalink(ctx context.Context, channelID, messageTS string) (string, error) {
	var res struct {
		OK        bool   `json:"ok"`
		Error     string `json:"error"`
		Permalink string `json:"permalink"`
	}
	values := url.Values{
		"channel":    {channelID},
		"message_ts": {messageTS},
	}
	if err := c.postForm(ctx, "chat.getPermalink", values, &res); err != nil {
		return "", fmt.Errorf("slack chat.getPermalink failed: %w", err)
	}
	if !res.OK {
		return "", fmt.Errorf("slack chat.getPermalink failed: %s", res.Error)
	}
	return res.Permalink, nil
}

type PostCustomize struct {
	Username string
	IconURL  string
}

type Channel struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *Client) ResolveChannel(ctx context.Context, channel string) (string, error) {
	if len(channel) > 0 && (channel[0] == 'C' || channel[0] == 'G') && !strings.Contains(channel, "#") {
		return channel, nil
	}
	name := strings.TrimPrefix(channel, "#")
	channels, err := c.ListChannels(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list channels: %w", err)
	}
	for _, ch := range channels {
		if ch.Name == name {
			return ch.ID, nil
		}
	}
	return "", fmt.Errorf("channel %q not found", channel)
}

func (c *Client) ListChannels(ctx context.Context) ([]Channel, error) {
	var all []Channel
	cursor := ""
	for {
		var res struct {
			OK               bool      `json:"ok"`
			Error            string    `json:"error"`
			Channels         []Channel `json:"channels"`
			ResponseMetadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		values := url.Values{
			"limit":            {"200"},
			"exclude_archived": {"true"},
		}
		if cursor != "" {
			values.Set("cursor", cursor)
		}
		if err := c.postForm(ctx, "conversations.list", values, &res); err != nil {
			return nil, err
		}
		if !res.OK {
			return nil, fmt.Errorf("slack conversations.list failed: %s", res.Error)
		}
		all = append(all, res.Channels...)
		cursor = res.ResponseMetadata.NextCursor
		if cursor == "" {
			break
		}
	}
	return all, nil
}

func (c *Client) CreateChannel(ctx context.Context, name string, isPrivate bool) (string, error) {
	var res struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Channel struct {
			ID string `json:"id"`
		} `json:"channel"`
	}
	values := url.Values{
		"name": {name},
	}
	if isPrivate {
		values.Set("is_private", "true")
	}
	if err := c.postForm(ctx, "conversations.create", values, &res); err != nil {
		return "", fmt.Errorf("slack conversations.create failed: %w", err)
	}
	if !res.OK {
		return "", fmt.Errorf("slack conversations.create failed: %s", res.Error)
	}
	return res.Channel.ID, nil
}

func (c *Client) SetChannelTopic(ctx context.Context, channelID, topic string) error {
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	values := url.Values{
		"channel": {channelID},
		"topic":   {topic},
	}
	if err := c.postForm(ctx, "conversations.setTopic", values, &res); err != nil {
		return fmt.Errorf("slack conversations.setTopic failed: %w", err)
	}
	if !res.OK {
		return fmt.Errorf("slack conversations.setTopic failed: %s", res.Error)
	}
	return nil
}

func (c *Client) SetChannelPurpose(ctx context.Context, channelID, purpose string) error {
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	values := url.Values{
		"channel": {channelID},
		"purpose": {purpose},
	}
	if err := c.postForm(ctx, "conversations.setPurpose", values, &res); err != nil {
		return fmt.Errorf("slack conversations.setPurpose failed: %w", err)
	}
	if !res.OK {
		return fmt.Errorf("slack conversations.setPurpose failed: %s", res.Error)
	}
	return nil
}

func (c *Client) InviteUsers(ctx context.Context, channelID string, userIDs []string) error {
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	values := url.Values{
		"channel": {channelID},
		"users":   {strings.Join(userIDs, ",")},
	}
	if err := c.postForm(ctx, "conversations.invite", values, &res); err != nil {
		return fmt.Errorf("slack conversations.invite failed: %w", err)
	}
	if !res.OK {
		return fmt.Errorf("slack conversations.invite failed: %s", res.Error)
	}
	return nil
}

func (c *Client) ArchiveChannel(ctx context.Context, channelID string) error {
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	values := url.Values{
		"channel": {channelID},
	}
	if err := c.postForm(ctx, "conversations.archive", values, &res); err != nil {
		return fmt.Errorf("slack conversations.archive failed: %w", err)
	}
	if !res.OK {
		return fmt.Errorf("slack conversations.archive failed: %s", res.Error)
	}
	return nil
}

func (c *Client) UnarchiveChannel(ctx context.Context, channelID string) error {
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	values := url.Values{
		"channel": {channelID},
	}
	if err := c.postForm(ctx, "conversations.unarchive", values, &res); err != nil {
		return fmt.Errorf("slack conversations.unarchive failed: %w", err)
	}
	if !res.OK {
		return fmt.Errorf("slack conversations.unarchive failed: %s", res.Error)
	}
	return nil
}

func (c *Client) OpenConversation(ctx context.Context, users []string) (string, error) {
	var res struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		Channel struct {
			ID string `json:"id"`
		} `json:"channel"`
	}
	values := url.Values{
		"users": {strings.Join(users, ",")},
	}
	if err := c.postForm(ctx, "conversations.open", values, &res); err != nil {
		return "", fmt.Errorf("slack conversations.open failed: %w", err)
	}
	if !res.OK {
		return "", fmt.Errorf("slack conversations.open failed: %s", res.Error)
	}
	return res.Channel.ID, nil
}

func (c *Client) PostEphemeral(ctx context.Context, channelID, userID, text string) error {
	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	values := url.Values{
		"channel": {channelID},
		"user":    {userID},
		"text":    {text},
	}
	if err := c.postForm(ctx, "chat.postEphemeral", values, &res); err != nil {
		return fmt.Errorf("slack chat.postEphemeral failed: %w", err)
	}
	if !res.OK {
		return fmt.Errorf("slack chat.postEphemeral failed: %s", res.Error)
	}
	return nil
}

func (c *Client) GetUserAvatarURL(ctx context.Context, slackUserID string) string {
	if slackUserID == "" {
		return ""
	}

	c.mu.RLock()
	if entry, ok := c.avatarCache[slackUserID]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.RUnlock()
		return entry.url
	}
	c.mu.RUnlock()

	var res struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		User  struct {
			Profile struct {
				Image192 string `json:"image_192"`
			} `json:"profile"`
		} `json:"user"`
	}
	if err := c.postForm(ctx, "users.info", url.Values{"user": {slackUserID}}, &res); err != nil || !res.OK {
		return ""
	}

	avatarURL := res.User.Profile.Image192
	c.mu.Lock()
	c.avatarCache[slackUserID] = avatarEntry{
		url:       avatarURL,
		expiresAt: time.Now().Add(1 * time.Hour),
	}
	c.mu.Unlock()
	return avatarURL
}
