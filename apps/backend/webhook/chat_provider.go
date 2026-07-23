package webhook

import (
	"context"
	"fmt"

	"alga/sse"
	"alga/types"
	"alga/valkey"
)

type SSEPublisherMixin struct {
	sseBroker    *sse.Broker
	ssePublisher *sse.DualPublisher
	vkClient     *valkey.Client
}

func (m *SSEPublisherMixin) SetSSEBroker(broker *sse.Broker, vkClient *valkey.Client) {
	m.sseBroker = broker
	m.vkClient = vkClient
	if broker != nil {
		m.ssePublisher = &sse.DualPublisher{Broker: broker, VKClient: vkClient}
	}
}

func (m *SSEPublisherMixin) PublishInvestigationEvent(eventType string, data any) {
	if m.ssePublisher == nil {
		return
	}
	m.ssePublisher.Publish(sse.Event{Type: eventType, Data: data})
}

type ChatProvider interface {
	SendAlert(ctx context.Context, channel string, alert types.Alert) (postID string, resolvedChannel string, err error)
	ReplyInThread(ctx context.Context, postID string, channelID string, message string) (replyID string, err error)
	UpdateAlertPost(ctx context.Context, postID string, channelID string, alert types.Alert) error
	UpdateTextPost(ctx context.Context, postID string, channelID string, senderName string, text string) error
	ListChannels(ctx context.Context) ([]map[string]any, error)
	ResolveChannel(ctx context.Context, channel string) (string, error)
	TestConnection(ctx context.Context) error
	Enabled() bool
	ProviderName() string
}

type ChatRouter struct {
	providers map[string]ChatProvider
}

func NewChatRouter(providers ...ChatProvider) *ChatRouter {
	r := &ChatRouter{providers: make(map[string]ChatProvider)}
	for _, p := range providers {
		r.providers[p.ProviderName()] = p
	}
	return r
}

func (r *ChatRouter) Provider(name string) ChatProvider {
	return r.providers[name]
}

func (r *ChatRouter) ForEach(fn func(ChatProvider)) {
	for _, p := range r.providers {
		fn(p)
	}
}

func (r *ChatRouter) SendAlert(ctx context.Context, providerName, channel string, alert types.Alert) (string, string, error) {
	p := r.Provider(providerName)
	if p == nil || !p.Enabled() {
		return "", "", fmt.Errorf("%s integration not configured", providerName)
	}
	return p.SendAlert(ctx, channel, alert)
}

func (r *ChatRouter) ReplyOnDeliveryTarget(ctx context.Context, dt DeliveryTargetRef, message string) error {
	p := r.Provider(dt.Provider)
	if p == nil || !p.Enabled() {
		return fmt.Errorf("%s integration is not configured", dt.Provider)
	}
	_, err := p.ReplyInThread(ctx, dt.PostID, dt.ChannelID, message)
	return err
}

type DeliveryTargetRef struct {
	Provider  string
	PostID    string
	ChannelID string
}
