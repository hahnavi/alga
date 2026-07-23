// Package llm implements an OpenAI-compatible chat completions client with
// streaming support, tool-call parsing, and retry/backoff for transient errors.
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Message represents a single chat message in the OpenAI format.
type Message struct {
	Role       string     `json:"role"`                   // system | user | assistant | tool
	Content    string     `json:"content,omitempty"`      // may be empty when tool_calls present
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // set on assistant turns
	ToolCallID string     `json:"tool_call_id,omitempty"` // set on tool-role messages
	Name       string     `json:"name,omitempty"`         // tool name for tool-role messages
}

// ToolCall is an assistant-issued tool invocation.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // always "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction carries the function name and serialized arguments.
type ToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // raw JSON string from the model
}

// Choice is one completion choice.
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// Usage tracks token accounting for a completion.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// CompletionResponse is the non-streaming chat completion response.
type CompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Request is a chat completions request body. We serialize manually rather
// than reusing the Message types directly so we can include the "tools" field.
type Request struct {
	Model       string
	Messages    []Message
	Tools       []map[string]any
	MaxTokens   int
	Temperature float64
	Stream      bool
}

// StreamCallback is invoked with each accumulated text delta during streaming.
// Return false to stop consuming the stream early (e.g. on user interrupt).
type StreamCallback func(accumulated string, delta string) bool

// Client is an OpenAI-compatible HTTP client.
type Client struct {
	baseURL string
	apiKey  string
	model   string
	http    *http.Client

	// MaxRetries is the maximum number of retry attempts for transient errors
	// (429, 500, 502, 503, 504, network). Default 3.
	MaxRetries int
	// MaxTokens is the default max_tokens when Request.MaxTokens is 0.
	MaxTokens int
	// Temperature is the default temperature when Request.Temperature is 0.
	Temperature float64
	// RequestTimeout bounds each HTTP request (including streaming). Default 120s.
	RequestTimeout time.Duration
	// RetryBaseDelay is the base for exponential backoff. Default 1s.
	RetryBaseDelay time.Duration
	// Logger is used for retry logging. Defaults to slog.Default().
	Logger *slog.Logger
}

// Option configures a Client.
type Option func(*Client)

// New constructs a Client for the given base URL, API key, and model.
func New(baseURL, apiKey, model string, opts ...Option) *Client {
	c := &Client{
		baseURL:        strings.TrimRight(baseURL, "/"),
		apiKey:         apiKey,
		model:          model,
		http:           &http.Client{},
		MaxRetries:     3,
		MaxTokens:      4096,
		Temperature:    0.3,
		RequestTimeout: 120 * time.Second,
		RetryBaseDelay: 1 * time.Second,
		Logger:         slog.Default(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// WithHTTPClient sets a custom *http.Client (e.g. for test transports).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// WithMaxTokens sets the default max_tokens.
func WithMaxTokens(n int) Option {
	return func(c *Client) { c.MaxTokens = n }
}

// WithTemperature sets the default temperature.
func WithTemperature(t float64) Option {
	return func(c *Client) { c.Temperature = t }
}

// WithMaxRetries sets the max retry count.
func WithMaxRetries(n int) Option {
	return func(c *Client) { c.MaxRetries = n }
}

// WithLogger sets the slog logger.
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) { c.Logger = l }
}

// Model returns the configured model name.
func (c *Client) Model() string { return c.model }

// Complete performs a non-streaming chat completion. Retries transient errors
// with exponential backoff (RetryBaseDelay * 2^attempt, capped at 30s, with
// jitter). Returns the full response or the final error.
func (c *Client) Complete(ctx context.Context, req Request) (*CompletionResponse, error) {
	req.Model = c.model
	if req.MaxTokens == 0 {
		req.MaxTokens = c.MaxTokens
	}
	if req.Temperature == 0 {
		req.Temperature = c.Temperature
	}
	req.Stream = false

	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		resp, retryable, err := c.doComplete(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !retryable || attempt == c.MaxRetries {
			break
		}
		backoff := c.backoff(attempt, err)
		c.Logger.Warn("llm transient error, retrying",
			"attempt", attempt+1, "max", c.MaxRetries, "backoff", backoff, "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
	}
	return nil, fmt.Errorf("llm complete failed after %d retries: %w", c.MaxRetries, lastErr)
}

func (c *Client) doComplete(ctx context.Context, req Request) (*CompletionResponse, bool, error) {
	body, err := c.marshalRequest(req)
	if err != nil {
		return nil, false, err
	}

	ctx, cancel := context.WithTimeout(ctx, c.RequestTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, false, err
	}
	c.setHeaders(httpReq)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, true, &TransportError{Err: err}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024))
	if err != nil {
		return nil, true, &TransportError{Err: err}
	}

	if resp.StatusCode >= 400 {
		retryable := isRetryableStatus(resp.StatusCode)
		return nil, retryable, &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
			RetryAfter: parseRetryAfter(resp.Header),
		}
	}

	var out CompletionResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, false, fmt.Errorf("decode completion: %w", err)
	}
	return &out, false, nil
}

// Stream performs a streaming chat completion, invoking cb for each text delta.
// Returns the full accumulated assistant text. Tool-call turns should use
// Complete (non-streaming) per SPEC §5.2.1; streaming is used only for the
// final no-tool turn to deliver tokens progressively.
func (c *Client) Stream(ctx context.Context, req Request, cb StreamCallback) (string, error) {
	req.Model = c.model
	if req.MaxTokens == 0 {
		req.MaxTokens = c.MaxTokens
	}
	if req.Temperature == 0 {
		req.Temperature = c.Temperature
	}
	req.Stream = true

	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		text, retryable, err := c.doStream(ctx, req, cb)
		if err == nil {
			return text, nil
		}
		lastErr = err
		if !retryable || attempt == c.MaxRetries {
			break
		}
		backoff := c.backoff(attempt, err)
		c.Logger.Warn("llm stream transient error, retrying",
			"attempt", attempt+1, "max", c.MaxRetries, "backoff", backoff, "err", err)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(backoff):
		}
	}
	return "", fmt.Errorf("llm stream failed after %d retries: %w", c.MaxRetries, lastErr)
}

func (c *Client) doStream(ctx context.Context, req Request, cb StreamCallback) (string, bool, error) {
	body, err := c.marshalRequest(req)
	if err != nil {
		return "", false, err
	}

	ctx, cancel := context.WithTimeout(ctx, c.RequestTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", false, err
	}
	c.setHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", true, &TransportError{Err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024))
		return "", isRetryableStatus(resp.StatusCode), &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
			RetryAfter: parseRetryAfter(resp.Header),
		}
	}

	var sb strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		// SSE lines are prefixed with "data: ".
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		payload := bytes.TrimSpace(line[5:])
		if len(payload) == 0 {
			continue
		}
		if bytes.Equal(payload, []byte("[DONE]")) {
			break
		}
		var ev struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(payload, &ev); err != nil {
			// Skip malformed chunks rather than aborting the stream.
			c.Logger.Debug("llm stream: skipping malformed chunk", "err", err)
			continue
		}
		for _, ch := range ev.Choices {
			if ch.Delta.Content != "" {
				sb.WriteString(ch.Delta.Content)
				if cb != nil {
					if !cb(sb.String(), ch.Delta.Content) {
						return sb.String(), false, nil
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return sb.String(), false, ctx.Err()
		}
		return sb.String(), true, &TransportError{Err: err}
	}
	return sb.String(), false, nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
}

func (c *Client) marshalRequest(req Request) ([]byte, error) {
	m := map[string]any{
		"model":       req.Model,
		"messages":    req.Messages,
		"temperature": req.Temperature,
		"max_tokens":  req.MaxTokens,
	}
	if len(req.Tools) > 0 {
		m["tools"] = req.Tools
	}
	if req.Stream {
		m["stream"] = true
	}
	return json.Marshal(m)
}

// backoff computes exponential backoff: base * 2^attempt, capped at 30s, with
// ~20% jitter, honoring RetryAfter from the API error when present.
func (c *Client) backoff(attempt int, err error) time.Duration {
	if ae, ok := err.(*APIError); ok && ae.RetryAfter > 0 {
		return ae.RetryAfter
	}
	base := c.RetryBaseDelay * time.Duration(int64(1)<<attempt)
	if base > 30*time.Second {
		base = 30 * time.Second
	}
	jitter := time.Duration(mathrandInt63n(int64(float64(base) * 0.2)))
	return base + jitter
}

func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func parseRetryAfter(h http.Header) time.Duration {
	raw := strings.TrimSpace(h.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	if secs, err := strconv.Atoi(raw); err == nil {
		if secs < 0 {
			return 0
		}
		// Cap at 10 minutes to defend against absurd server values; LLM
		// retry-after headers should never exceed this in practice.
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

// HasToolCalls reports whether the assistant message requests tool execution.
func HasToolCalls(m Message) bool { return len(m.ToolCalls) > 0 }

// APIError is a non-2xx response from the LLM API. Retryable indicates whether
// callers should retry; RetryAfter is parsed from the response when present.
type APIError struct {
	StatusCode int
	Body       string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("llm api error %d: %s", e.StatusCode, truncateForLog(e.Body, 500))
}

// IsRetryable reports whether the error is retryable per the status code.
func (e *APIError) IsRetryable() bool { return isRetryableStatus(e.StatusCode) }

// TransportError wraps a network/transport error during an LLM call.
type TransportError struct {
	Err error
}

func (e *TransportError) Error() string { return "llm transport error: " + e.Err.Error() }
func (e *TransportError) Unwrap() error { return e.Err }

// UserFacingMessage returns a sanitized message for end users when the LLM
// fails irrecoverably (SPEC §8.3).
func UserFacingMessage(err error) string {
	if err == nil {
		return ""
	}
	return "I'm having trouble thinking right now. Please try again in a moment."
}

func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

// mathrandInt63n returns a non-negative pseudo-random int64 in [0,n) for
// backoff jitter. Uses math/rand/v2 which is auto-seeded (Go 1.22+) and
// goroutine-safe, avoiding the legacy global-source seeding footgun.
func mathrandInt63n(n int64) int64 {
	if n <= 0 {
		return 0
	}
	return rand.Int64N(n)
}
