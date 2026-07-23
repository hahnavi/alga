package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"alga/logger"
	"alga/valkey"
)

type DualPublisher struct {
	Broker   *Broker
	VKClient *valkey.Client
}

func (dp *DualPublisher) Publish(event Event) {
	if dp.Broker == nil {
		return
	}
	if dp.VKClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		dp.publishToValkey(ctx, "alga:events", event)
	} else {
		dp.Broker.Publish(event)
	}
}

func (dp *DualPublisher) PublishToUser(userID string, event Event) {
	if dp.Broker == nil {
		return
	}
	if dp.VKClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		dp.publishToValkey(ctx, fmt.Sprintf("alga:user_events:%s", userID), event)
	} else {
		dp.Broker.PublishToUser(userID, event)
	}
}

func (dp *DualPublisher) PublishToAgent(agentID string, event Event) {
	if dp.Broker == nil {
		return
	}
	if dp.VKClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		dp.publishToValkey(ctx, fmt.Sprintf("alga:agent_events:%s", agentID), event)
	} else {
		_ = dp.Broker.PublishToAgent(agentID, event)
	}
}

func (dp *DualPublisher) publishToValkey(ctx context.Context, channel string, event Event) {
	if dp.VKClient == nil {
		return
	}
	data, err := json.Marshal(event)
	if err != nil {
		logger.Error("Failed to marshal SSE event for Valkey", "component", "sse", "error", err)
		return
	}
	if err := dp.VKClient.Do(ctx, dp.VKClient.Builder().Publish().Channel(channel).Message(string(data)).Build()).Error(); err != nil {
		logger.Error("Failed to publish SSE event to Valkey channel", "component", "sse", "channel", channel, "error", err)
	}
}
