package webhook

import (
	"context"
	"errors"
	"fmt"

	"alga/mattermost"
	"alga/types"
)

type MattermostChatProvider struct {
	client *mattermost.Client
}

func NewMattermostChatProvider(client *mattermost.Client) *MattermostChatProvider {
	return &MattermostChatProvider{client: client}
}

func (m *MattermostChatProvider) SendAlert(ctx context.Context, channel string, alert types.Alert) (string, string, error) {
	if !m.Enabled() {
		return "", "", errors.New("mattermost integration is not configured")
	}
	channelID, err := m.client.GetChannelByName(ctx, channel)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve channel %s: %w", channel, err)
	}
	_, attach := formatAlertAttachment(alert)
	props := map[string]any{
		"attachments": []types.MattermostAttachment{attach},
	}
	postID, err := m.client.CreatePost(ctx, channelID, "", props)
	if err != nil {
		return "", "", err
	}
	return postID, channelID, nil
}

func (m *MattermostChatProvider) ReplyInThread(ctx context.Context, postID string, _ string, message string) (string, error) {
	if !m.Enabled() {
		return "", errors.New("mattermost integration is not configured")
	}
	return m.client.ReplyToPost(ctx, postID, message, nil)
}

func (m *MattermostChatProvider) UpdateAlertPost(ctx context.Context, postID string, _ string, alert types.Alert) error {
	if !m.Enabled() {
		return errors.New("mattermost integration is not configured")
	}
	_, attach := formatAlertAttachment(alert)
	props := map[string]any{
		"attachments": []types.MattermostAttachment{attach},
	}
	return m.client.UpdatePost(ctx, postID, "", props)
}

func (m *MattermostChatProvider) UpdateTextPost(ctx context.Context, postID string, _ string, _ string, text string) error {
	if !m.Enabled() {
		return errors.New("mattermost integration is not configured")
	}
	return m.client.UpdatePost(ctx, postID, text, nil)
}

func (m *MattermostChatProvider) ListChannels(ctx context.Context) ([]map[string]any, error) {
	channels, err := m.client.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, len(channels))
	for i, ch := range channels {
		result[i] = map[string]any{"id": ch.ID, "name": ch.Name}
	}
	return result, nil
}

func (m *MattermostChatProvider) ResolveChannel(ctx context.Context, channel string) (string, error) {
	return m.client.GetChannelByName(ctx, channel)
}

func (m *MattermostChatProvider) TestConnection(ctx context.Context) error {
	return m.client.TestConnection(ctx)
}

func (m *MattermostChatProvider) Enabled() bool {
	return m.client != nil && m.client.Enabled()
}

func (m *MattermostChatProvider) ProviderName() string {
	return "mattermost"
}
