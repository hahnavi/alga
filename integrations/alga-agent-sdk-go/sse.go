package alga

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SSEClient maintains a persistent connection to /api/v1/agent/events,
// reconstructing the event stream across reconnects with exponential backoff.
// All On* callbacks fire on the SSE goroutine — slow handlers must dispatch
// their own goroutine or they will block the stream and trip the heartbeat.
type SSEClient struct {
	httpBase   string
	token      string
	dedup      *MessageDedup
	logger     Logger
	httpClient *http.Client

	closing bool
	mu      sync.Mutex
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// ErrChan receives terminal errors (auth failure). Transient errors are
	// retried silently with backoff and never sent here. The channel is
	// buffered(1) so a single reader can drain it after Disconnect().
	ErrChan chan error

	// heartbeatInterval is the cadence at which the SSE client posts to
	// /api/v1/agent/heartbeat so the backend marks the agent online.
	heartbeatInterval time.Duration

	OnConnected           func(ConnectedEvent)
	OnMessage             func(MessageEvent)
	OnTyping              func(TypingEvent)
	OnInvestigationResume func(InvestigationSignalEvent)
	OnPeerFinding         func(PeerFindingEvent)
	OnPeerAsk             func(PeerAskEvent)
	OnPeerReply           func(PeerReplyEvent)
	OnCoordinationTask    func(CoordinationTaskEvent)
	OnSummarizeIncident   func(SummarizeIncidentEvent)
	OnAlertAutoResolved   func(AlertAutoResolvedEvent)
	OnIncidentCommsStale  func(IncidentCommsStaleEvent)
	// OnUnknownEvent fires for any event type without a dedicated callback,
	// letting callers observe new backend event types without an SDK update.
	OnUnknownEvent func(eventType string, data []byte)
}

// SSEOption configures an SSEClient.
type SSEOption func(*SSEClient)

// WithSSELogger injects a structured logger into the SSE client.
func WithSSELogger(l Logger) SSEOption {
	return func(c *SSEClient) { c.logger = l }
}

// WithSSEHTTPClient injects a custom *http.Client for the SSE stream and
// heartbeat. The client should have a generous or no Timeout — the SSE
// stream is long-lived.
func WithSSEHTTPClient(h *http.Client) SSEOption {
	return func(c *SSEClient) {
		if h != nil {
			c.httpClient = h
		}
	}
}

// WithSSEHeartbeat overrides the default 30s heartbeat cadence. Values below
// 5s are clamped to protect the backend.
func WithSSEHeartbeat(d time.Duration) SSEOption {
	return func(c *SSEClient) {
		if d > 0 {
			c.heartbeatInterval = d
		}
	}
}

func NewSSEClient(httpBase, token string, dedup *MessageDedup, opts ...SSEOption) *SSEClient {
	c := &SSEClient{
		httpBase:          httpBase,
		token:             token,
		dedup:             dedup,
		logger:            slogDefault(),
		httpClient:        &http.Client{Timeout: 0},
		ErrChan:           make(chan error, 1),
		heartbeatInterval: 30 * time.Second,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	// Bound the heartbeat to a reasonable floor. 1s protects the backend from
	// accidental tight loops without making tests wait 30s for a tick.
	if c.heartbeatInterval < time.Second {
		c.heartbeatInterval = time.Second
	}
	return c
}

func (c *SSEClient) Start(ctx context.Context) error {
	childCtx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancel = cancel
	c.mu.Unlock()

	c.wg.Add(2)
	go func() {
		defer c.wg.Done()
		c.sseLoop(childCtx)
	}()
	go func() {
		defer c.wg.Done()
		c.heartbeatLoop(childCtx)
	}()

	return nil
}

func (c *SSEClient) Stop() {
	c.mu.Lock()
	c.closing = true
	cancel := c.cancel
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Wait blocks until the SSE and heartbeat goroutines have exited. Safe to call
// after Stop(); calling before Stop() will block until the context is canceled.
func (c *SSEClient) Wait() {
	c.wg.Wait()
}

func (c *SSEClient) isClosing() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closing
}

func (c *SSEClient) sseLoop(ctx context.Context) {
	backoff := 2 * time.Second

	for {
		if ctx.Err() != nil || c.isClosing() {
			return
		}

		connected, err := c.connectAndServe(ctx)
		if connected {
			// Any successful connection resets the backoff so a later blip
			// starts from 2s again rather than a stale 60s ceiling.
			backoff = 2 * time.Second
		}
		if err != nil {
			var authErr *AlgaAuthError
			if errors.As(err, &authErr) {
				c.logger.Error("sse auth error, stopping reconnect loop", "status", authErr.StatusCode, "err", authErr.Message)
				select {
				case c.ErrChan <- authErr:
				default:
				}
				return
			}

			// Honor Retry-After when the server gave us one (e.g. 429);
			// otherwise exponential backoff with jitter.
			delay := backoff
			var apiErr *AlgaAPIError
			if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
				delay = apiErr.RetryAfter
			}
			jitter := time.Duration(float64(delay) * (0.9 + 0.2*rand.Float64()))
			c.logger.Warn("sse reconnecting after error", "err", err.Error(), "backoff", jitter)
			select {
			case <-ctx.Done():
				return
			case <-time.After(jitter):
			}

			backoff = min(backoff*2, 60*time.Second)
		}

		if ctx.Err() != nil || c.isClosing() {
			return
		}
	}
}

func (c *SSEClient) connectAndServe(ctx context.Context) (connected bool, err error) {
	url := c.httpBase + "/api/v1/agent/events"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, &AlgaConnectionError{Err: err}
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, &AlgaConnectionError{Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return false, &AlgaAuthError{StatusCode: resp.StatusCode, Message: "authentication failed"}
	}
	if resp.StatusCode != http.StatusOK {
		return false, &AlgaAPIError{
			StatusCode: resp.StatusCode,
			Message:    "unexpected status code",
			RetryAfter: parseRetryAfter(resp.Header),
		}
	}

	// Raise the per-line scanner cap so large investigation / peer-finding
	// payloads don't silently truncate and drop the connection.
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var eventType string
	var dataBuf bytes.Buffer

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return true, nil
		default:
		}

		line := scanner.Text()

		if line == "" {
			if dataBuf.Len() > 0 {
				// Per the SSE spec, a missing `event:` field means "message".
				ev := eventType
				if ev == "" {
					ev = "message"
				}
				c.dispatch(ev, dataBuf.String())
			}
			eventType = ""
			dataBuf.Reset()
			continue
		}

		if strings.HasPrefix(line, ":") {
			continue
		}

		if rest, ok := strings.CutPrefix(line, "event:"); ok {
			eventType = strings.TrimSpace(rest)
			continue
		}

		if rest, ok := strings.CutPrefix(line, "data:"); ok {
			// Per spec, strip exactly one leading space when present.
			rest = strings.TrimPrefix(rest, " ")
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(rest)
			continue
		}

		// `id:` is intentionally ignored: we don't expose a Last-Event-ID
		// resume path. The backend dedups via message_id in the payload.
		if strings.HasPrefix(line, "id:") {
			continue
		}
	}
	if err := scanner.Err(); err != nil {
		// Scanner errors are typically network resets; surface them so the
		// outer loop backs off and reconnects.
		return true, &AlgaConnectionError{Err: err}
	}
	return true, nil
}

func (c *SSEClient) dispatch(eventType, data string) {
	data = strings.TrimSpace(data)

	switch eventType {
	case "connected":
		if c.OnConnected != nil {
			var evt ConnectedEvent
			if json.Unmarshal([]byte(data), &evt) == nil {
				c.OnConnected(evt)
			}
		}

	case "message":
		var evt MessageEvent
		if json.Unmarshal([]byte(data), &evt) != nil {
			return
		}
		if evt.MessageID != "" && c.dedup != nil && c.dedup.IsDuplicate(evt.MessageID) {
			return
		}
		// Skip internal/system messages (leading 🔒 per backend convention).
		if strings.HasPrefix(evt.Text, "🔒") {
			return
		}
		if c.OnMessage != nil {
			c.OnMessage(evt)
		}

	case "typing":
		if c.OnTyping != nil {
			var evt TypingEvent
			if json.Unmarshal([]byte(data), &evt) == nil {
				c.OnTyping(evt)
			}
		}

	case "investigation_resume":
		if c.OnInvestigationResume != nil {
			var evt InvestigationSignalEvent
			if json.Unmarshal([]byte(data), &evt) == nil {
				c.OnInvestigationResume(evt)
			}
		}

	case "peer_finding":
		if c.OnPeerFinding != nil {
			var evt PeerFindingEvent
			if json.Unmarshal([]byte(data), &evt) == nil {
				c.OnPeerFinding(evt)
			}
		}

	case "peer_ask":
		if c.OnPeerAsk != nil {
			var evt PeerAskEvent
			if json.Unmarshal([]byte(data), &evt) == nil {
				c.OnPeerAsk(evt)
			}
		}

	case "peer_reply":
		if c.OnPeerReply != nil {
			var evt PeerReplyEvent
			if json.Unmarshal([]byte(data), &evt) == nil {
				c.OnPeerReply(evt)
			}
		}

	case "coordination_task_dispatched":
		if c.OnCoordinationTask != nil {
			var evt CoordinationTaskEvent
			if json.Unmarshal([]byte(data), &evt) == nil {
				c.OnCoordinationTask(evt)
			}
		}

	case "summarize_incident":
		if c.OnSummarizeIncident != nil {
			var evt SummarizeIncidentEvent
			if json.Unmarshal([]byte(data), &evt) == nil {
				c.OnSummarizeIncident(evt)
			}
		}

	case "alert_auto_resolved":
		if c.OnAlertAutoResolved != nil {
			var evt AlertAutoResolvedEvent
			if json.Unmarshal([]byte(data), &evt) == nil {
				c.OnAlertAutoResolved(evt)
			}
		}

	case "incident_comms_stale":
		if c.OnIncidentCommsStale != nil {
			var evt IncidentCommsStaleEvent
			if json.Unmarshal([]byte(data), &evt) == nil {
				c.OnIncidentCommsStale(evt)
			}
		}

	default:
		if c.OnUnknownEvent != nil {
			c.OnUnknownEvent(eventType, []byte(data))
		}
	}
}

func (c *SSEClient) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(c.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.postHeartbeat(ctx); err != nil {
				var authErr *AlgaAuthError
				if errors.As(err, &authErr) {
					c.logger.Error("heartbeat auth failure, stopping", "status", authErr.StatusCode)
					select {
					case c.ErrChan <- authErr:
					default:
					}
					return
				}
				// Non-auth errors are logged and survived; the SSE loop will
				// observe persistent outages on its own cadence.
				c.logger.Warn("heartbeat failed", "err", err.Error())
			}
		}
	}
}

func (c *SSEClient) postHeartbeat(ctx context.Context) error {
	url := c.httpBase + "/api/v1/agent/heartbeat"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &AlgaConnectionError{Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &AlgaAuthError{StatusCode: resp.StatusCode, Message: "heartbeat auth failed"}
	}
	if resp.StatusCode >= 400 {
		return &AlgaAPIError{StatusCode: resp.StatusCode, Message: "heartbeat non-ok status"}
	}
	return nil
}
