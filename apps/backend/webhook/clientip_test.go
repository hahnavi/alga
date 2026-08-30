package webhook

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"alga/api/platform"
)

type stubIPExtractor struct{ ip string }

func (s stubIPExtractor) ClientIP(_ *http.Request) string { return s.ip }

func newClientIPTestRequest(remoteAddr, xff string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/webhooks/alerts", nil)
	req.RemoteAddr = remoteAddr
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	return req
}

// Without an injected extractor the receiver keys the rate limiter on
// RemoteAddr only: a spoofed X-Forwarded-For must not rotate the key.
func TestReceiverClientIPFallbackIgnoresXFF(t *testing.T) {
	t.Parallel()

	r := NewReceiver(nil, nil, nil, nil, nil, nil, false)
	req := newClientIPTestRequest("203.0.113.9:443", "198.51.100.1, 198.51.100.2")
	if got := r.clientIP(req); got != "203.0.113.9" {
		t.Fatalf("got %q, want RemoteAddr 203.0.113.9", got)
	}
}

// With an extractor injected (TRUSTED_PROXIES configured) its resolution wins.
func TestReceiverClientIPUsesInjectedExtractor(t *testing.T) {
	t.Parallel()

	r := NewReceiver(nil, nil, nil, nil, nil, nil, false)
	r.SetIPExtractor(stubIPExtractor{ip: "198.51.100.7"})
	req := newClientIPTestRequest("10.0.0.1:5555", "198.51.100.7")
	if got := r.clientIP(req); got != "198.51.100.7" {
		t.Fatalf("got %q, want extractor result 198.51.100.7", got)
	}
}

func TestRemoteAddrHost(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"203.0.113.9:443": "203.0.113.9",
		"[::1]:8080":      "::1",
		"10.1.2.3":        "10.1.2.3",
	}
	for in, want := range cases {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = in
		if got := remoteAddrHost(req); got != want {
			t.Fatalf("remoteAddrHost(%q) = %q, want %q", in, got, want)
		}
	}
}

// The Slack and Mattermost chat-webhook handlers share the same resolution:
// RemoteAddr-only unless an extractor is injected.
func TestChatWebhookHandlerClientIPIgnoresXFFWithoutExtractor(t *testing.T) {
	t.Parallel()

	req := newClientIPTestRequest("203.0.113.9:443", "198.51.100.1")

	slack := &SlackWebhookHandler{}
	if got := slack.clientIP(req); got != "203.0.113.9" {
		t.Fatalf("slack: got %q, want RemoteAddr 203.0.113.9", got)
	}
	slack.SetIPExtractor(stubIPExtractor{ip: "198.51.100.7"})
	if got := slack.clientIP(req); got != "198.51.100.7" {
		t.Fatalf("slack: got %q, want extractor result 198.51.100.7", got)
	}

	mm := &MattermostWebhookHandler{}
	if got := mm.clientIP(req); got != "203.0.113.9" {
		t.Fatalf("mattermost: got %q, want RemoteAddr 203.0.113.9", got)
	}
	mm.SetIPExtractor(platform.IPExtractor(stubIPExtractor{ip: "198.51.100.7"}))
	if got := mm.clientIP(req); got != "198.51.100.7" {
		t.Fatalf("mattermost: got %q, want extractor result 198.51.100.7", got)
	}
}
