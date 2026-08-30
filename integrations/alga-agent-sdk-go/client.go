package alga

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// idempotencyKeyHeader is the backend header that turns a state-changing
// request into a replay-safe one. Defined here rather than imported from the
// backend so the SDK stays stdlib-only.
const idempotencyKeyHeader = "Idempotency-Key"

// agentMessagesPath is the only agent endpoint where the backend honors
// Idempotency-Key (scope "agent:message"). Mutations elsewhere are not
// replay-safe and are therefore never auto-retried by the SDK.
const agentMessagesPath = "/api/v1/agent/messages"

// maxResponseBytes bounds how much of a response body the SDK will read.
const maxResponseBytes = 8 * 1024 * 1024

// maxErrorMessageBytes bounds how much of an error response body is retained
// in error values (and therefore in logs).
const maxErrorMessageBytes = 4 * 1024

type AlgaClient struct {
	serverURL  string
	token      string
	httpClient *http.Client
	userAgent  string
	sse        *SSEClient
	dedup      *MessageDedup
	logger     Logger
	errCh      chan error
	// maxRESTRetries is the max number of retry attempts for transient REST
	// failures. Zero disables retries.
	maxRESTRetries int
	// heartbeatInterval is propagated to the SSE client on Connect.
	heartbeatInterval time.Duration

	OnConnected           func(ConnectedEvent)
	OnMessage             func(MessageEvent)
	OnTyping              func(TypingEvent)
	OnInvestigationResume func(InvestigationSignalEvent)
	OnPeerFinding         func(PeerFindingEvent)
	OnPeerAsk             func(PeerAskEvent)
	OnPeerReply           func(PeerReplyEvent)
	OnSummarizeIncident   func(SummarizeIncidentEvent)
	OnAlertAutoResolved   func(AlertAutoResolvedEvent)
	OnIncidentCommsStale  func(IncidentCommsStaleEvent)
	// OnUnknownEvent receives any SSE event type the SDK has no dedicated
	// handler for, so consumers can react to backend additions without an SDK
	// upgrade.
	OnUnknownEvent func(eventType string, data []byte)
}

// NewAlgaClient constructs an AlgaClient for the given server URL and bearer
// token. Pass functional options to override the HTTP client, dedup cache,
// logger, or retry policy.
func NewAlgaClient(serverURL, token string, opts ...Option) *AlgaClient {
	o := &Options{}
	defaults(o)
	for _, opt := range opts {
		opt(o)
	}
	c := &AlgaClient{
		serverURL:         strings.TrimRight(serverURL, "/"),
		token:             token,
		httpClient:        o.HTTPClient,
		userAgent:         o.UserAgent,
		dedup:             o.Dedup,
		logger:            o.Logger,
		errCh:             make(chan error, 1),
		maxRESTRetries:    o.MaxRESTRetries,
		heartbeatInterval: o.HeartbeatInterval,
	}
	return c
}

// ServerURL returns the configured backend URL (without trailing slash).
func (c *AlgaClient) ServerURL() string { return c.serverURL }

// Err returns the channel that receives terminal errors from the SSE and
// heartbeat loops (e.g. a revoked token). Once an error arrives the client has
// stopped reconnecting; the caller must obtain a valid token and Connect again.
func (c *AlgaClient) Err() <-chan error { return c.errCh }

func (c *AlgaClient) Connect(ctx context.Context) error {
	// The SSE stream is long-lived; the REST client's Timeout (default 30s)
	// would kill it. Copy everything except Timeout so custom Transport,
	// CheckRedirect, and Jar behavior carries over to the stream.
	sseHTTPClient := &http.Client{
		Transport:     c.httpClient.Transport,
		CheckRedirect: c.httpClient.CheckRedirect,
		Jar:           c.httpClient.Jar,
	}

	sseClient := NewSSEClient(c.serverURL, c.token, c.dedup,
		WithSSELogger(c.logger),
		WithSSEHTTPClient(sseHTTPClient),
		WithSSEHeartbeat(c.heartbeatInterval),
	)
	sseClient.ErrChan = c.errCh

	sseClient.OnConnected = c.OnConnected
	sseClient.OnMessage = c.OnMessage
	sseClient.OnTyping = c.OnTyping
	sseClient.OnInvestigationResume = c.OnInvestigationResume
	sseClient.OnPeerFinding = c.OnPeerFinding
	sseClient.OnPeerAsk = c.OnPeerAsk
	sseClient.OnPeerReply = c.OnPeerReply
	sseClient.OnSummarizeIncident = c.OnSummarizeIncident
	sseClient.OnAlertAutoResolved = c.OnAlertAutoResolved
	sseClient.OnIncidentCommsStale = c.OnIncidentCommsStale
	sseClient.OnUnknownEvent = c.OnUnknownEvent

	c.sse = sseClient
	return sseClient.Start(ctx)
}

// Disconnect signals the SSE and heartbeat goroutines to stop and waits for
// them to fully unwind.
func (c *AlgaClient) Disconnect() {
	if c.sse != nil {
		c.sse.Stop()
		c.sse.Wait()
		c.sse = nil
	}
}

// --- HTTP plumbing ---

func (c *AlgaClient) doRequest(ctx context.Context, method, path string, body io.Reader, contentType, idempotencyKey string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.serverURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", c.userAgent)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if idempotencyKey != "" {
		req.Header.Set(idempotencyKeyHeader, idempotencyKey)
	}
	return c.httpClient.Do(req)
}

// doJSON performs a JSON REST call. GETs are retried on transient errors;
// mutations are not (see doJSONIdem for the replay-safe variant).
func (c *AlgaClient) doJSON(ctx context.Context, method, path string, payload, result any) error {
	return c.doJSONIdem(ctx, method, path, payload, result, "")
}

// doJSONIdem is doJSON with an explicit Idempotency-Key. The backend honors
// the key only on POST /api/v1/agent/messages; for that path a key is
// auto-generated when the caller does not supply one, making retries
// replay-safe. Mutations on any other path are performed exactly once — a
// retry there could double-execute the mutation because the backend has no
// replay cache for it.
func (c *AlgaClient) doJSONIdem(ctx context.Context, method, path string, payload, result any, idempotencyKey string) error {
	var bodyData []byte
	if payload != nil {
		var err error
		bodyData, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
	}

	mutating := method != http.MethodGet && method != http.MethodHead
	if mutating && idempotencyKey == "" && path == agentMessagesPath {
		idempotencyKey = newIdempotencyKey()
	}

	attempts := c.maxRESTRetries
	if mutating && idempotencyKey == "" {
		attempts = 0
	}

	var lastErr error
	for attempt := 0; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Fresh body reader per attempt.
		var body io.Reader
		contentType := ""
		if bodyData != nil {
			body = bytes.NewReader(bodyData)
			contentType = "application/json"
		}

		resp, err := c.doRequest(ctx, method, path, body, contentType, idempotencyKey)
		if err != nil {
			wrapped := &AlgaConnectionError{Err: err}
			lastErr = wrapped
			if !wrapped.IsRetryable() || attempt == attempts {
				return wrapped
			}
			c.logger.Warn("alga rest transient error, retrying",
				"method", method, "path", path, "attempt", attempt+1, "max", attempts, "err", err)
			if !c.sleep(ctx, backoffFor(attempt, 0)) {
				return ctx.Err()
			}
			continue
		}

		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		resp.Body.Close()
		if readErr != nil {
			wrapped := &AlgaConnectionError{Err: readErr}
			lastErr = wrapped
			if !wrapped.IsRetryable() || attempt == attempts {
				return wrapped
			}
			c.logger.Warn("alga rest read error, retrying", "method", method, "path", path, "attempt", attempt+1, "err", readErr)
			if !c.sleep(ctx, backoffFor(attempt, 0)) {
				return ctx.Err()
			}
			continue
		}

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return &AlgaAuthError{StatusCode: resp.StatusCode, Message: truncate(string(respBody), maxErrorMessageBytes)}
		}

		if resp.StatusCode >= 400 {
			apiErr := &AlgaAPIError{
				StatusCode: resp.StatusCode,
				Message:    truncate(string(respBody), maxErrorMessageBytes),
				RetryAfter: parseRetryAfter(resp.Header),
			}
			if !apiErr.IsRetryable() || attempt == attempts {
				return apiErr
			}
			lastErr = apiErr
			c.logger.Warn("alga rest retryable status, retrying",
				"method", method, "path", path, "attempt", attempt+1, "status", resp.StatusCode)
			if !c.sleep(ctx, backoffFor(attempt, apiErr.RetryAfter)) {
				return ctx.Err()
			}
			continue
		}

		if result != nil && len(respBody) > 0 {
			if err := unmarshalData(respBody, result); err != nil {
				return fmt.Errorf("decode response: %w", err)
			}
		}
		return nil
	}

	return lastErr
}

// unmarshalData decodes a backend response into out, unwrapping the standard
// {"data": ...} success envelope when present. Some endpoints (message
// edit/delete acks, memory and peer-ask creation, secrets) write flat bodies;
// those fall through to a plain decode.
func unmarshalData(body []byte, out any) error {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err == nil && len(env.Data) > 0 && !bytes.Equal(env.Data, []byte("null")) {
		return json.Unmarshal(env.Data, out)
	}
	return json.Unmarshal(body, out)
}

// newIdempotencyKey generates a random key. It is computed once per logical
// call (before the retry loop), so every retry of that call replays the same
// key and the backend serves the cached response instead of re-executing.
func newIdempotencyKey() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// rand.Read only fails if the OS CSPRNG is unreadable, in which case
		// we have bigger problems. Fall back to a time-based key so the call
		// still proceeds.
		return fmt.Sprintf("alga-%d", Now().UnixNano())
	}
	return "alga-" + hex.EncodeToString(buf[:])
}

// truncate caps s at n bytes, preserving valid UTF-8.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Walk backward from the cutoff to find the last valid rune boundary.
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// sleep waits for d (or ctx cancellation). Returns false if ctx fired first.
func (c *AlgaClient) sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// backoffFor returns exponential backoff for an attempt index, capped at 30s
// plus up to 20% additive jitter, honoring server-supplied RetryAfter when
// present.
func backoffFor(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return min(retryAfter, 10*time.Minute)
	}
	base := time.Second * time.Duration(int64(1)<<attempt)
	base = min(base, 30*time.Second)
	jitter := time.Duration(randInt64N(int64(float64(base) * 0.2)))
	return base + jitter
}

// --- REST methods ---

// ListAlerts returns alerts matching the given query params (status, severity,
// search, limit, skip, sort).
func (c *AlgaClient) ListAlerts(ctx context.Context, params map[string]string) ([]Alert, error) {
	path := withQuery("/api/v1/agent/alerts", params)
	var result []Alert
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *AlgaClient) GetAlert(ctx context.Context, fingerprint string) (*Alert, error) {
	var result Alert
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/agent/alerts/"+url.PathEscape(fingerprint), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AlgaClient) ResolveAlert(ctx context.Context, fingerprint string) error {
	return c.doJSON(ctx, http.MethodPost, "/api/v1/agent/alerts/"+url.PathEscape(fingerprint)+"/resolve", nil, nil)
}

func (c *AlgaClient) ReopenAlert(ctx context.Context, fingerprint string) error {
	return c.doJSON(ctx, http.MethodPost, "/api/v1/agent/alerts/"+url.PathEscape(fingerprint)+"/reopen", nil, nil)
}

// SendMessage sends a text message to a chat. An Idempotency-Key is
// auto-generated so retries are replay-safe.
func (c *AlgaClient) SendMessage(ctx context.Context, chatID, text string, mentions []string) (*SendMessageResponse, error) {
	return c.SendMessageWithKey(ctx, chatID, text, mentions, "")
}

// SendMessageWithKey is the explicit-key variant of SendMessage for callers
// that drive their own outbox.
func (c *AlgaClient) SendMessageWithKey(ctx context.Context, chatID, text string, mentions []string, idempotencyKey string) (*SendMessageResponse, error) {
	payload := map[string]any{
		"chat_id":  chatID,
		"kind":     "text",
		"text":     text,
		"mentions": mentions,
	}
	var result SendMessageResponse
	if err := c.doJSONIdem(ctx, http.MethodPost, agentMessagesPath, payload, &result, idempotencyKey); err != nil {
		return nil, err
	}
	return &result, nil
}

// SendCommand sends a kind=inv_tool command. The SDK auto-injects an
// Idempotency-Key so a retry of the same logical command is replayed from the
// backend cache rather than re-executed. Command failures surface as an
// *AlgaAPIError (404/422/500) whose Message carries the backend outcome JSON.
func (c *AlgaClient) SendCommand(ctx context.Context, chatID string, cmd InvestigationCommand) (*CommandResponse, error) {
	return c.SendCommandWithKey(ctx, chatID, cmd, "")
}

// SendCommandWithKey is the explicit-key variant of SendCommand for callers
// that drive their own outbox.
func (c *AlgaClient) SendCommandWithKey(ctx context.Context, chatID string, cmd InvestigationCommand, idempotencyKey string) (*CommandResponse, error) {
	payload := map[string]any{
		"chat_id": chatID,
		"kind":    "inv_tool",
		"command": cmd,
	}
	var result CommandResponse
	if err := c.doJSONIdem(ctx, http.MethodPost, agentMessagesPath, payload, &result, idempotencyKey); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AlgaClient) EditMessage(ctx context.Context, messageID, chatID, text string) error {
	payload := map[string]string{
		"chat_id": chatID,
		"kind":    "text",
		"text":    text,
	}
	return c.doJSON(ctx, http.MethodPut, agentMessagesPath+"/"+url.PathEscape(messageID), payload, nil)
}

func (c *AlgaClient) DeleteMessage(ctx context.Context, messageID, chatID string) error {
	payload := map[string]string{
		"chat_id": chatID,
	}
	return c.doJSON(ctx, http.MethodDelete, agentMessagesPath+"/"+url.PathEscape(messageID), payload, nil)
}

// SendDraft streams a partial ("draft") message into a chat. Repeated posts
// with the same draftID replace the draft text until a final message is sent.
func (c *AlgaClient) SendDraft(ctx context.Context, chatID, draftID, text string) error {
	payload := map[string]string{
		"chat_id":  chatID,
		"draft_id": draftID,
		"text":     text,
	}
	return c.doJSON(ctx, http.MethodPost, "/api/v1/agent/drafts", payload, nil)
}

func (c *AlgaClient) SendTyping(ctx context.Context, chatID string, active bool) error {
	payload := map[string]any{
		"chat_id": chatID,
		"active":  active,
	}
	return c.doJSON(ctx, http.MethodPost, "/api/v1/agent/typing", payload, nil)
}

func (c *AlgaClient) SendHeartbeat(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodPost, "/api/v1/agent/heartbeat", nil, nil)
}

func (c *AlgaClient) ListKnowledge(ctx context.Context, params map[string]string) (*KnowledgeListResponse, error) {
	path := withQuery("/api/v1/agent/knowledge", params)
	var result KnowledgeListResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AlgaClient) GetKnowledge(ctx context.Context, id string) (*KnowledgeNote, error) {
	var result KnowledgeNote
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/agent/knowledge/"+url.PathEscape(id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// CreateKnowledge creates a knowledge note. The backend requires
// source_investigation_id and confidence.
func (c *AlgaClient) CreateKnowledge(ctx context.Context, params map[string]any) (*KnowledgeNote, error) {
	var result KnowledgeNote
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/agent/knowledge", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AlgaClient) ListMemories(ctx context.Context, params map[string]string) (*MemoryListResponse, error) {
	path := withQuery("/api/v1/agent/memories", params)
	var result MemoryListResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AlgaClient) CreateMemory(ctx context.Context, params map[string]any) (*Memory, error) {
	var result Memory
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/agent/memories", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AlgaClient) GetMemory(ctx context.Context, id string) (*Memory, error) {
	var result Memory
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/agent/memories/"+url.PathEscape(id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AlgaClient) DeleteMemory(ctx context.Context, id string) error {
	return c.doJSON(ctx, http.MethodDelete, "/api/v1/agent/memories/"+url.PathEscape(id), nil, nil)
}

func (c *AlgaClient) ListPeerAsks(ctx context.Context, params map[string]string) (*PeerAskListResponse, error) {
	path := withQuery("/api/v1/agent/peer-ask", params)
	var result PeerAskListResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AlgaClient) CreatePeerAsk(ctx context.Context, params map[string]any) (*PeerAsk, error) {
	var result PeerAsk
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/agent/peer-ask", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AlgaClient) GetPeerAsk(ctx context.Context, id string) (*PeerAsk, error) {
	var result PeerAsk
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/agent/peer-ask/"+url.PathEscape(id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AlgaClient) ReplyPeerAsk(ctx context.Context, id, reply string) (*PeerAsk, error) {
	payload := map[string]string{"reply": reply}
	var result PeerAsk
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/agent/peer-ask/"+url.PathEscape(id)+"/reply", payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AlgaClient) CancelPeerAsk(ctx context.Context, id string) error {
	return c.doJSON(ctx, http.MethodPost, "/api/v1/agent/peer-ask/"+url.PathEscape(id)+"/cancel", nil, nil)
}

// GetIncident returns the incident record plus its role assignments.
func (c *AlgaClient) GetIncident(ctx context.Context, incidentNumber int64) (*IncidentContext, error) {
	var result IncidentContext
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/v1/agent/incidents/%d", incidentNumber), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetIncidentTimeline returns the incident timeline entries, newest ordering
// as defined by the backend.
func (c *AlgaClient) GetIncidentTimeline(ctx context.Context, incidentNumber int64) ([]map[string]any, error) {
	var result []map[string]any
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/v1/agent/incidents/%d/timeline", incidentNumber), nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *AlgaClient) AddIncidentTimeline(ctx context.Context, incidentNumber int64, message, eventType string) error {
	payload := map[string]string{
		"message":    message,
		"event_type": eventType,
	}
	return c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/api/v1/agent/incidents/%d/timeline", incidentNumber), payload, nil)
}

// UpdateIncidentSummary patches the incident summary. The agent must be
// assigned to an investigation within the incident.
func (c *AlgaClient) UpdateIncidentSummary(ctx context.Context, incidentNumber int64, summary string) (*Incident, error) {
	payload := map[string]string{"summary": summary}
	var result Incident
	if err := c.doJSON(ctx, http.MethodPatch, fmt.Sprintf("/api/v1/agent/incidents/%d", incidentNumber), payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AlgaClient) ListServices(ctx context.Context, params map[string]string) (*ServiceListResponse, error) {
	path := withQuery("/api/v1/agent/services", params)
	var result ServiceListResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// WhoIsOnCall returns the users currently on call across schedules. Requires
// the command capability.
func (c *AlgaClient) WhoIsOnCall(ctx context.Context) ([]OnCallEntry, error) {
	var result []OnCallEntry
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/agent/on-call/current", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *AlgaClient) GetPlaybooks(ctx context.Context, alertFingerprint string) ([]Playbook, error) {
	path := withQuery("/api/v1/agent/playbooks", map[string]string{"alert_fingerprint": alertFingerprint})
	var result []Playbook
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// GetSecret fetches an allow-listed shared secret value. Not-found and
// not-allow-listed both surface as 404 by backend design.
func (c *AlgaClient) GetSecret(ctx context.Context, secretID string) (*SecretValue, error) {
	var result SecretValue
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/agent/secrets/"+url.PathEscape(secretID), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SendIncidentSummary posts a kind=incident_summary message into the incident
// coordination thread. Requires the communicate capability.
func (c *AlgaClient) SendIncidentSummary(ctx context.Context, incidentNumber int64, text string) error {
	payload := map[string]string{
		"chat_id": fmt.Sprintf("incident_coord_%d", incidentNumber),
		"kind":    "incident_summary",
		"text":    text,
	}
	return c.doJSON(ctx, http.MethodPost, agentMessagesPath, payload, nil)
}

// --- helpers ---

// withQuery appends url-encoded params to path. Empty values are skipped.
func withQuery(path string, params map[string]string) string {
	q := url.Values{}
	for k, v := range params {
		if v == "" {
			continue
		}
		q.Set(k, v)
	}
	if encoded := q.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

// parseRetryAfter parses the HTTP `Retry-After` header into a duration. Both
// delta-seconds and HTTP-date forms are accepted. Returns 0 when the header is
// absent or unparseable. Capped at 10 minutes to defend against absurd values.
func parseRetryAfter(h http.Header) time.Duration {
	raw := strings.TrimSpace(h.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs < 0 {
			return 0
		}
		return min(time.Duration(secs)*time.Second, 10*time.Minute)
	}
	if t, err := http.ParseTime(raw); err == nil {
		if d := time.Until(t); d > 0 {
			return min(d, 10*time.Minute)
		}
	}
	return 0
}

// IsAuthError reports whether err is an AlgaAuthError. Useful for callers that
// want to distinguish "stop retrying, token is bad" from any other failure.
func IsAuthError(err error) bool {
	var authErr *AlgaAuthError
	return errors.As(err, &authErr)
}
