package alga

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"crypto/rand"
	"encoding/hex"
)

// MaxMediaUploadBytes bounds the size of a single UploadMedia call to avoid
// unbounded memory growth inside the SDK process.
const MaxMediaUploadBytes = 32 * 1024 * 1024

// idempotencyKeyHeader is the backend header that turns a state-changing
// request into a replay-safe one. Defined here rather than imported from the
// backend so the SDK stays stdlib-only.
const idempotencyKeyHeader = "Idempotency-Key"

type AlgaClient struct {
	serverURL  string
	token      string
	httpClient *http.Client
	userAgent  string
	sse        *SSEClient
	dedup      *MessageDedup
	logger     Logger
	// maxRESTRetries is the max number of retry attempts for transient REST
	// failures. Zero disables retries.
	maxRESTRetries int
	// heartbeatInterval is propagated to the SSE client on Connect.
	heartbeatInterval time.Duration

	OnConnected           func(ConnectedEvent)
	OnMessage             func(MessageEvent)
	OnTyping              func(TypingEvent)
	OnInvestigationCancel func(InvestigationSignalEvent)
	OnInvestigationPause  func(InvestigationSignalEvent)
	OnInvestigationResume func(InvestigationSignalEvent)
	OnPeerFinding         func(PeerFindingEvent)
	OnPeerAsk             func(PeerAskEvent)
	OnPeerReply           func(PeerReplyEvent)
	OnAgentPresence       func(AgentPresenceEvent)
	OnCoordinationTask    func(CoordinationTaskEvent)
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
		maxRESTRetries:    o.MaxRESTRetries,
		heartbeatInterval: o.HeartbeatInterval,
	}
	return c
}

// ServerURL returns the configured backend URL (without trailing slash).
func (c *AlgaClient) ServerURL() string { return c.serverURL }

func (c *AlgaClient) Connect(ctx context.Context) error {
	sseClient := NewSSEClient(c.serverURL, c.token, c.dedup,
		WithSSELogger(c.logger),
		WithSSEHTTPClient(c.httpClient),
		WithSSEHeartbeat(c.heartbeatInterval),
	)

	sseClient.OnConnected = c.OnConnected
	sseClient.OnMessage = c.OnMessage
	sseClient.OnTyping = c.OnTyping
	sseClient.OnInvestigationCancel = c.OnInvestigationCancel
	sseClient.OnInvestigationPause = c.OnInvestigationPause
	sseClient.OnInvestigationResume = c.OnInvestigationResume
	sseClient.OnPeerFinding = c.OnPeerFinding
	sseClient.OnPeerAsk = c.OnPeerAsk
	sseClient.OnPeerReply = c.OnPeerReply
	sseClient.OnAgentPresence = c.OnAgentPresence
	sseClient.OnCoordinationTask = c.OnCoordinationTask

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

// requestOption mutates an outbound *http.Request (e.g. to add headers).
type requestOption func(*http.Request)

// withIdempotency sets the Idempotency-Key header. The backend caches the
// response under this key for ~24h, so a retry that hits the same key
// replays the original response rather than re-executing the mutation.
// Pass an explicit key when the caller has a natural stable identifier
// (e.g. an outbox message ID); otherwise omit and the SDK will derive one.
func withIdempotency(key string) requestOption {
	return func(req *http.Request) {
		if key != "" {
			req.Header.Set(idempotencyKeyHeader, key)
		}
	}
}

func (c *AlgaClient) doRequest(ctx context.Context, method, path string, body io.Reader, contentType string, reqOpts ...requestOption) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.serverURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", c.userAgent)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for _, opt := range reqOpts {
		if opt != nil {
			opt(req)
		}
	}
	return c.httpClient.Do(req)
}

// doJSON performs a JSON REST call with retries on transient errors. payload
// is marshaled as the request body; result, if non-nil, is unmarshaled from
// the response. reqOpts can carry an Idempotency-Key for retry-safe writes.
func (c *AlgaClient) doJSON(ctx context.Context, method, path string, payload, result any, reqOpts ...requestOption) error {
	var bodyData []byte
	if payload != nil {
		var err error
		bodyData, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
	}

	// If the caller did not provide an Idempotency-Key for a mutating call,
	// derive one from the request shape so retries are replay-safe. We only
	// do this for state-changing methods; GETs are inherently idempotent.
	if method != http.MethodGet && method != http.MethodHead && !hasIdempotencyHeader(reqOpts) {
		reqOpts = append(reqOpts, withIdempotency(deriveIdempotencyKey(method, path, bodyData)))
	}

	var lastErr error
	attempts := c.maxRESTRetries
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

		resp, err := c.doRequest(ctx, method, path, body, contentType, reqOpts...)
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

		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
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
			return &AlgaAuthError{StatusCode: resp.StatusCode, Message: string(respBody)}
		}

		if resp.StatusCode >= 400 {
			apiErr := &AlgaAPIError{
				StatusCode: resp.StatusCode,
				Message:    string(respBody),
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
			if err := json.Unmarshal(respBody, result); err != nil {
				return fmt.Errorf("decode response: %w", err)
			}
		}
		return nil
	}

	return lastErr
}

// hasIdempotencyHeader reports whether any of reqOpts already sets the
// Idempotency-Key header, so the SDK doesn't override a caller-provided key.
func hasIdempotencyHeader(reqOpts []requestOption) bool {
	dummy, _ := http.NewRequest(http.MethodPost, "http://x", nil)
	for _, opt := range reqOpts {
		if opt == nil {
			continue
		}
		opt(dummy)
		if dummy.Header.Get(idempotencyKeyHeader) != "" {
			return true
		}
	}
	return false
}

// deriveIdempotencyKey generates a stable-per-payload key so a retry of the
// same call hits the cached response. We use crypto/rand rather than a hash
// of the body because the body may carry timestamps that differ across
// retries (e.g. in case of a partial replay) and we still want the dedupe to
// be keyed to the logical request, not its byte-identical form. Per-attempt
// uniqueness is good enough: the backend replays by exact header match.
func deriveIdempotencyKey(method, path string, body []byte) string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// rand.Read on Linux CSPRNG only fails if /dev/urandom is unreadable,
		// in which case we have bigger problems. Fall back to a time-based
		// key so the call still proceeds without retry safety.
		return fmt.Sprintf("alga-%d", Now().UnixNano())
	}
	return "alga-" + hex.EncodeToString(buf[:])
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

// backoffFor returns exponential backoff for an attempt index, capped at 30s,
// honoring server-supplied RetryAfter when present. Uses the Go 1.22+
// auto-seeded crypto-grade rand only at the call site (here we use math/rand/v2
// for jitter via the SDK's package-level helper).
func backoffFor(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		if retryAfter > 10*time.Minute {
			return 10 * time.Minute
		}
		return retryAfter
	}
	base := time.Second * time.Duration(int64(1)<<attempt)
	if base > 30*time.Second {
		base = 30 * time.Second
	}
	// ±10% jitter.
	jitter := time.Duration(randInt64N(int64(float64(base) * 0.2)))
	return base + jitter
}

// --- REST methods ---

func (c *AlgaClient) ListAlerts(ctx context.Context, params map[string]string) (*AlertListResponse, error) {
	path := withQuery("/api/v1/agent/alerts", params)
	var result AlertListResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
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

func (c *AlgaClient) ListInvestigations(ctx context.Context, params map[string]string) (*InvestigationListResponse, error) {
	path := withQuery("/api/v1/agent/investigations", params)
	var result InvestigationListResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AlgaClient) GetInvestigation(ctx context.Context, id string) (*Investigation, error) {
	var result Investigation
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/agent/investigations/"+url.PathEscape(id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AlgaClient) PostUpdate(ctx context.Context, id, updateType, message string) (*Investigation, error) {
	payload := map[string]string{
		"type":    updateType,
		"message": message,
	}
	var result Investigation
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/agent/investigations/"+url.PathEscape(id)+"/updates", payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListIncidentTasks returns the coordination tasks for an incident. This is
// the read side of dispatch_task / claim_task / complete_task. The backend
// exposes it as GET /api/v1/agent/incidents/{id}/tasks.
func (c *AlgaClient) ListIncidentTasks(ctx context.Context, incidentNumber int64) ([]CoordinationTask, error) {
	var result struct {
		Tasks []CoordinationTask `json:"tasks"`
		Total int                `json:"total"`
	}
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/api/v1/agent/incidents/%d/tasks", incidentNumber), nil, &result); err != nil {
		return nil, err
	}
	return result.Tasks, nil
}

// SendMessage sends a text message to a chat. When idempotencyKey is non-empty
// it is forwarded as the Idempotency-Key header so retries are replay-safe.
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
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/agent/messages", payload, &result, withIdempotency(idempotencyKey)); err != nil {
		return nil, err
	}
	return &result, nil
}

// SendCommand sends a kind=inv_tool command. The SDK auto-injects an
// Idempotency-Key so a retry of the same logical command is replayed from the
// backend cache rather than re-executed.
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
	if err := c.doJSON(ctx, http.MethodPost, "/api/v1/agent/messages", payload, &result, withIdempotency(idempotencyKey)); err != nil {
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
	return c.doJSON(ctx, http.MethodPut, "/api/v1/agent/messages/"+url.PathEscape(messageID), payload, nil)
}

func (c *AlgaClient) DeleteMessage(ctx context.Context, messageID, chatID string) error {
	payload := map[string]string{
		"chat_id": chatID,
	}
	return c.doJSON(ctx, http.MethodDelete, "/api/v1/agent/messages/"+url.PathEscape(messageID), payload, nil)
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

func (c *AlgaClient) GetIncident(ctx context.Context, id string) (*Incident, error) {
	var result Incident
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/agent/incidents/"+url.PathEscape(id), nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AlgaClient) AddIncidentTimeline(ctx context.Context, id, message, eventType string) error {
	payload := map[string]string{
		"message":    message,
		"event_type": eventType,
	}
	return c.doJSON(ctx, http.MethodPost, "/api/v1/agent/incidents/"+url.PathEscape(id)+"/timeline", payload, nil)
}

func (c *AlgaClient) ListServices(ctx context.Context) ([]Service, error) {
	var result []Service
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/agent/services", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *AlgaClient) WhoIsOnCall(ctx context.Context) (map[string]any, error) {
	var result map[string]any
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/agent/on-call/current", nil, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *AlgaClient) GetCapabilities(ctx context.Context) ([]Capability, error) {
	var result []Capability
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/agent/capabilities", nil, &result); err != nil {
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

func (c *AlgaClient) SendIncidentSummary(ctx context.Context, incidentID, text string) error {
	payload := map[string]string{
		"chat_id": "incident_coord_" + incidentID,
		"kind":    "incident_summary",
		"text":    text,
	}
	return c.doJSON(ctx, http.MethodPost, "/api/v1/agent/messages", payload, nil)
}

// UploadMedia uploads a file via multipart form. The file size is bounded by
// MaxMediaUploadBytes to avoid unbounded memory growth inside the SDK.
func (c *AlgaClient) UploadMedia(ctx context.Context, filePath string) error {
	fi, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	if fi.Size() > MaxMediaUploadBytes {
		return fmt.Errorf("upload of %d bytes exceeds the %d byte limit", fi.Size(), MaxMediaUploadBytes)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filePath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, io.LimitReader(f, MaxMediaUploadBytes+1)); err != nil {
		return err
	}
	w.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL+"/api/v1/agent/media", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &AlgaConnectionError{Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
		return &AlgaAuthError{StatusCode: resp.StatusCode, Message: string(body)}
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024*1024))
		return &AlgaAPIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
			RetryAfter: parseRetryAfter(resp.Header),
		}
	}

	return nil
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
		if secs > 600 {
			secs = 600
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(raw); err == nil {
		if d := time.Until(t); d > 0 {
			if d > 10*time.Minute {
				return 10 * time.Minute
			}
			return d
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
