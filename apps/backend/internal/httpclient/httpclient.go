// Package httpclient provides small shared helpers for outbound HTTP used by
// Alga vendor integration clients (Slack, Mattermost, Telnyx, Twilio).
//
// These helpers keep the four vendor clients from duplicating the same
// *http.Client construction and request-build/Do/body-read boilerplate. They
// are intentionally thin transports: callers retain their own auth-header
// construction, URL building, and non-2xx status handling.
package httpclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MaxResponseBytes caps response reads at 1 MiB to bound memory use.
const MaxResponseBytes = 1 << 20

// DefaultUserAgent is applied when the caller does not provide one. Some vendor
// APIs (e.g. Telnyx) are fronted by Cloudflare, which hard-blocks Go's default
// "Go-http-client/1.1" UA with a 403, so every outbound integration request
// must carry an identifying UA.
const DefaultUserAgent = "alga-integrations/1.0 (+https://alga.app)"

// NewTimeoutClient returns an *http.Client with the given request timeout.
func NewTimeoutClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

// DoJSON builds an http.Request with the given context, applies the provided
// headers, performs the request, and returns the status code and (capped)
// response body. It is a thin transport: it does NOT inspect the status code,
// so callers keep their own non-2xx handling.
//
// On request-build or transport error it returns (0, nil, err) with the error
// wrapped. A read error after a successful Do returns (resp.StatusCode, body,
// err) so callers can still branch on status if needed.
func DoJSON(ctx context.Context, client *http.Client, method, url string, headers map[string]string, body io.Reader) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// Fall back to an identifying UA when the caller didn't set one. Avoids
	// Cloudflare/WAF blocks on the default "Go-http-client/1.1" UA. Header.Get
	// is case-insensitive, so any caller-supplied UA short-circuits this.
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", DefaultUserAgent)
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes))
	if err != nil {
		return resp.StatusCode, respBody, fmt.Errorf("failed to read response body: %w", err)
	}
	return resp.StatusCode, respBody, nil
}
