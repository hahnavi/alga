package webhook

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"alga/slack"
	"alga/types"
)

type SlackChatProvider struct {
	client *slack.Client
}

func NewSlackChatProvider(client *slack.Client) *SlackChatProvider {
	return &SlackChatProvider{client: client}
}

func (s *SlackChatProvider) SendAlert(ctx context.Context, channel string, alert types.Alert) (string, string, error) {
	if !s.Enabled() {
		return "", "", errors.New("slack integration is not configured")
	}
	channelID, err := s.client.ResolveChannel(ctx, channel)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve slack channel %s: %w", channel, err)
	}
	attachments, fallback := slack.BuildAlertAttachments(alert)
	postID, err := s.client.PostMessageWithAttachments(ctx, channelID, fallback, attachments)
	if err != nil {
		return "", "", err
	}
	return postID, channelID, nil
}

func (s *SlackChatProvider) ReplyInThread(ctx context.Context, threadTS string, channelID string, message string) (string, error) {
	if !s.Enabled() {
		return "", errors.New("slack integration is not configured")
	}
	return s.client.PostThreadReply(ctx, channelID, threadTS, message)
}

func (s *SlackChatProvider) UpdateAlertPost(ctx context.Context, messageTS string, channelID string, alert types.Alert) error {
	if !s.Enabled() {
		return errors.New("slack integration is not configured")
	}
	attachments, fallback := slack.BuildAlertAttachments(alert)
	return s.client.UpdateMessageWithAttachments(ctx, channelID, messageTS, fallback, attachments)
}

func (s *SlackChatProvider) UpdateTextPost(ctx context.Context, messageTS string, channelID string, senderName string, text string) error {
	if !s.Enabled() {
		return errors.New("slack integration is not configured")
	}
	cz := slack.PostCustomize{
		Username: senderName,
		IconURL:  fmt.Sprintf("https://api.dicebear.com/9.x/bottts-neutral/png?seed=%s&size=128", url.QueryEscape(senderName)),
	}
	return s.client.UpdateMessage(ctx, channelID, messageTS, slack.Mrkdwn(text), cz)
}

func (s *SlackChatProvider) ListChannels(ctx context.Context) ([]map[string]any, error) {
	channels, err := s.client.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]map[string]any, len(channels))
	for i, ch := range channels {
		result[i] = map[string]any{"id": ch.ID, "name": ch.Name}
	}
	return result, nil
}

func (s *SlackChatProvider) ResolveChannel(ctx context.Context, channel string) (string, error) {
	return s.client.ResolveChannel(ctx, channel)
}

func (s *SlackChatProvider) TestConnection(ctx context.Context) error {
	return s.client.TestConnection(ctx)
}

func (s *SlackChatProvider) Enabled() bool {
	return s.client != nil && s.client.Enabled()
}

func (s *SlackChatProvider) ProviderName() string {
	return "slack"
}
