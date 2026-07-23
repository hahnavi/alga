package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valkey-io/valkey-go"

	"alga/logger"
)

type Event struct {
	ID   string `json:"id,omitempty"`
	Type string `json:"type"`
	Data any    `json:"data"`
}

type Broker struct {
	mu           sync.RWMutex
	clients      map[string]chan Event
	userClients  map[string]map[string]chan Event
	agentClients map[string]map[string]chan Event
	nextID       atomic.Int64
}

func NewBroker() *Broker {
	return &Broker{
		clients:      make(map[string]chan Event),
		userClients:  make(map[string]map[string]chan Event),
		agentClients: make(map[string]map[string]chan Event),
	}
}

func safeSend(ch chan Event, event Event, dropMsg string, dropFields ...any) {
	defer func() {
		_ = recover() // a panic means the channel was closed; safe to ignore
	}()
	select {
	case ch <- event:
	default:
		logger.Warn(dropMsg, dropFields...)
	}
}

func (b *Broker) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	event.ID = strconv.FormatInt(b.nextID.Add(1), 10)

	for id, ch := range b.clients {
		safeSend(ch, event, "SSE client buffer full, dropping event", "component", "sse", "client_id", id)
	}
}

func (b *Broker) SubscribeUser(userID, clientID string, ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.userClients[userID] == nil {
		b.userClients[userID] = make(map[string]chan Event)
	}
	b.userClients[userID][clientID] = ch
}

func (b *Broker) UnsubscribeUser(userID, clientID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if clients, ok := b.userClients[userID]; ok {
		if ch, exists := clients[clientID]; exists {
			close(ch)
			delete(clients, clientID)
		}
		if len(clients) == 0 {
			delete(b.userClients, userID)
		}
	}
	delete(b.clients, clientID)
}

func (b *Broker) PublishToUser(userID string, event Event) {
	event.ID = strconv.FormatInt(b.nextID.Add(1), 10)

	b.mu.RLock()
	var sinks []chan Event
	for _, ch := range b.userClients[userID] {
		sinks = append(sinks, ch)
	}
	b.mu.RUnlock()

	for _, ch := range sinks {
		safeSend(ch, event, "SSE user client buffer full, dropping event", "component", "sse", "user_id", userID)
	}
}

func startSubscription(ctx context.Context, client valkey.Client, cmd valkey.Completed, handler func(valkey.PubSubMessage), name string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("sse broker panic recovered", "panic", r)
			}
		}()
		backoff := time.Second
		const maxBackoff = 30 * time.Second
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if err := client.Receive(ctx, cmd, handler); err != nil {
				if ctx.Err() != nil {
					return
				}
				logger.Error("Valkey pub/sub disconnected, reconnecting", "component", "sse", "name", name, "backoff", backoff, "error", err)
				timer := time.NewTimer(backoff)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
				backoff *= 2
				backoff = min(backoff, maxBackoff)
				continue
			}
			backoff = time.Second
		}
	}()
}

func publishToValkeyChannel(ctx context.Context, client valkey.Client, channel string, event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal SSE event: %w", err)
	}
	return client.Do(ctx, client.B().Publish().Channel(channel).Message(string(data)).Build()).Error()
}

func (b *Broker) StartValkeySubscription(ctx context.Context, client valkey.Client) {
	startSubscription(ctx, client, client.B().Subscribe().Channel("alga:events").Build(), func(msg valkey.PubSubMessage) {
		var event Event
		if err := json.Unmarshal([]byte(msg.Message), &event); err != nil {
			logger.Error("Failed to unmarshal Valkey pub/sub message", "component", "sse", "error", err)
			return
		}
		b.Publish(event)
	}, "events")
	logger.Info("SSE broker subscribed to Valkey pub/sub channel 'alga:events'")
}

func (b *Broker) StartValkeyUserSubscription(ctx context.Context, client valkey.Client) {
	startSubscription(ctx, client, client.B().Psubscribe().Pattern("alga:user_events:*").Build(), func(msg valkey.PubSubMessage) {
		channel := msg.Channel
		if len(channel) > len("alga:user_events:") {
			userID := channel[len("alga:user_events:"):]
			var event Event
			if err := json.Unmarshal([]byte(msg.Message), &event); err != nil {
				logger.Error("Failed to unmarshal Valkey user event", "component", "sse", "error", err)
				return
			}
			b.PublishToUser(userID, event)
		}
	}, "user events")
	logger.Info("SSE broker subscribed to Valkey user events pattern 'alga:user_events:*'")
}

func PublishToValkey(ctx context.Context, client valkey.Client, event Event) error {
	return publishToValkeyChannel(ctx, client, "alga:events", event)
}

func (b *Broker) SubscribeAgent(agentTokenID, clientID string, ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.agentClients[agentTokenID] == nil {
		b.agentClients[agentTokenID] = make(map[string]chan Event)
	}
	b.agentClients[agentTokenID][clientID] = ch
}

func (b *Broker) UnsubscribeAgent(agentTokenID, clientID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if clients, ok := b.agentClients[agentTokenID]; ok {
		if ch, exists := clients[clientID]; exists {
			close(ch)
			delete(clients, clientID)
		}
		if len(clients) == 0 {
			delete(b.agentClients, agentTokenID)
		}
	}
}

func (b *Broker) PublishToAgent(agentTokenID string, event Event) error {
	event.ID = strconv.FormatInt(b.nextID.Add(1), 10)

	b.mu.RLock()
	var sinks []chan Event
	for _, ch := range b.agentClients[agentTokenID] {
		sinks = append(sinks, ch)
	}
	b.mu.RUnlock()

	if len(sinks) == 0 {
		return fmt.Errorf("no SSE clients for agent %s", agentTokenID)
	}
	for _, ch := range sinks {
		safeSend(ch, event, "SSE agent client buffer full", "component", "sse", "agent_token_id", agentTokenID)
	}
	return nil
}

func (b *Broker) AgentOnline(agentTokenID string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.agentClients[agentTokenID]) > 0
}

func (b *Broker) BroadcastToAgents(event Event, excludeAgentID string) {
	event.ID = strconv.FormatInt(b.nextID.Add(1), 10)

	b.mu.RLock()
	defer b.mu.RUnlock()
	for agentID, clients := range b.agentClients {
		if agentID == excludeAgentID {
			continue
		}
		for id, ch := range clients {
			safeSend(ch, event, "SSE agent client buffer full, dropping broadcast", "component", "sse", "agent_id", agentID, "client_id", id)
		}
	}
}

func (b *Broker) PublishToAgentAllowDrop(agentTokenID string, event Event) {
	event.ID = strconv.FormatInt(b.nextID.Add(1), 10)

	b.mu.RLock()
	var sinks []chan Event
	for _, ch := range b.agentClients[agentTokenID] {
		sinks = append(sinks, ch)
	}
	b.mu.RUnlock()

	for _, ch := range sinks {
		safeSend(ch, event, "SSE agent client buffer full, dropping event", "component", "sse", "agent_token_id", agentTokenID)
	}
}

func PublishToValkeyAgent(ctx context.Context, client valkey.Client, agentTokenID string, event Event) error {
	return publishToValkeyChannel(ctx, client, fmt.Sprintf("alga:agent_events:%s", agentTokenID), event)
}

func (b *Broker) StartValkeyAgentSubscription(ctx context.Context, client valkey.Client) {
	startSubscription(ctx, client, client.B().Psubscribe().Pattern("alga:agent_events:*").Build(), func(msg valkey.PubSubMessage) {
		channel := msg.Channel
		prefix := "alga:agent_events:"
		if len(channel) > len(prefix) {
			agentTokenID := channel[len(prefix):]
			var event Event
			if err := json.Unmarshal([]byte(msg.Message), &event); err != nil {
				logger.Error("Failed to unmarshal Valkey agent event", "component", "sse", "error", err)
				return
			}
			b.PublishToAgentAllowDrop(agentTokenID, event)
		}
	}, "agent events")
	logger.Info("SSE broker subscribed to Valkey agent events pattern 'alga:agent_events:*'")
}
