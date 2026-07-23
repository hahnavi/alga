package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"alga/logger"
	"alga/store"
)

// DisableWriteDeadline clears the per-connection write deadline that the
// http.Server applies via WriteTimeout. Long-lived streaming handlers (SSE)
// must call this before entering their write loop, otherwise the server's
// WriteTimeout would truncate the stream. The request context remains the
// authoritative cancellation source. This is the standard per-handler
// override documented for net/http. A zero time.Time clears the deadline;
// non-ResponseWriter writers are no-ops.
func DisableWriteDeadline(w http.ResponseWriter) {
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})
}

// WriteEvent marshals event.Data and writes a single Server-Sent Event frame
// to w using direct writes (no intermediate string allocation). It is the hot
// path for every event delivered to every connected SSE client.
func WriteEvent(w io.Writer, event Event) error {
	data, err := json.Marshal(event.Data)
	if err != nil {
		return err
	}
	if event.ID != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", event.ID); err != nil {
			return err
		}
	}
	if event.Type != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event.Type); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}

type contextKey string

const userContextKey contextKey = "user"

func userFromBrokerContext(ctx context.Context) *store.UserRecord {
	u, _ := ctx.Value(userContextKey).(*store.UserRecord)
	return u
}

func SetUserContext(ctx context.Context, user *store.UserRecord) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

func (b *Broker) Subscribe(clientID string) chan Event {
	ch := make(chan Event, 32)
	b.mu.Lock()
	b.clients[clientID] = ch
	b.mu.Unlock()
	return ch
}

func (b *Broker) Unsubscribe(clientID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.clients[clientID]; ok {
		close(ch)
		delete(b.clients, clientID)
	}
	for _, clients := range b.userClients {
		delete(clients, clientID)
	}
	for _, clients := range b.agentClients {
		delete(clients, clientID)
	}
}

func (b *Broker) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		// Clear the server-wide WriteTimeout for this long-lived stream;
		// the request context is the cancellation source for SSE.
		DisableWriteDeadline(w)

		clientID := uuid.New().String()
		ch := b.Subscribe(clientID)
		defer b.Unsubscribe(clientID)

		var userID string
		if user := userFromBrokerContext(r.Context()); user != nil {
			userID = user.ID.String()
			b.SubscribeUser(userID, clientID, ch)
			defer b.UnsubscribeUser(userID, clientID)
		}

		logger.Debug("SSE client connected", "component", "sse", "client_id", clientID, "user_id", userID, "remote", r.RemoteAddr)

		if err := WriteEvent(w, Event{Type: "connected", Data: map[string]string{"client_id": clientID}}); err != nil {
			logger.Warn("failed to write sse connected frame", "component", "sse", "error", err)
			return
		}
		flusher.Flush()

		keepalive := time.NewTicker(30 * time.Second)
		defer keepalive.Stop()

		for {
			select {
			case <-r.Context().Done():
				logger.Debug("SSE client disconnected", "component", "sse", "client_id", clientID)
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				if err := WriteEvent(w, event); err != nil {
					logger.Error("Failed to write SSE event", "component", "sse", "error", err)
					continue
				}
				flusher.Flush()
			case <-keepalive.C:
				if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
					logger.Debug("SSE keepalive write failed; closing stream", "component", "sse", "error", err)
					return
				}
				flusher.Flush()
			}
		}
	}
}
