package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"alga/api/platform"
	"alga/logger"
)

// TestWebhookErrorsUseJSONEnvelope covers WP-B4: public webhook 4xx/5xx
// responses use the standard {"error":{code,message}} envelope instead of
// plain-text bodies that fed log-forgement channels.
func TestWebhookErrorsUseJSONEnvelope(t *testing.T) {
	logger.Init("error", "")

	t.Run("alerts receiver bad token", func(t *testing.T) {
		tokenStore := &receiverWebhookTokenStore{validToken: "good"}
		receiver := NewReceiver(nil, nil, nil, nil, tokenStore, nil, false)
		req := httptest.NewRequest(http.MethodPost, "/webhooks/alerts", strings.NewReader(`{"alerts":[]}`))
		req.Header.Set("Authorization", "Bearer wrong")
		rec := httptest.NewRecorder()
		receiver.handleWebhook(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Fatalf("content-type = %q, want application/json (body=%q)", ct, rec.Body.String())
		}
		var env struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("body is not the error envelope: %v (%q)", err, rec.Body.String())
		}
		if env.Error.Code != string(platform.ErrorCodeUnauthorized) {
			t.Fatalf("error.code = %q, want %q", env.Error.Code, platform.ErrorCodeUnauthorized)
		}
	})

	t.Run("slack url_verification challenge still plain text", func(t *testing.T) {
		h := NewSlackWebhookHandler(nil, nil, "secret")
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		body := `{"type":"url_verification","challenge":"ch_123"}`
		mac := slackSignature("secret", ts, body)
		req := httptest.NewRequest(http.MethodPost, "/webhooks/slack", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Slack-Request-Timestamp", ts)
		req.Header.Set("X-Slack-Signature", mac)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
			t.Fatalf("challenge content-type = %q, want text/plain (protocol requirement)", ct)
		}
		if strings.TrimSpace(rec.Body.String()) != "ch_123" {
			t.Fatalf("challenge echo = %q, want ch_123", rec.Body.String())
		}
	})
}

func TestRateLimitDiscoveryEnvelopeShape(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	platform.WriteRateLimitExceeded(w, "60")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("429 expected, got %d", w.Code)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("429 body is not JSON envelope: %v", err)
	}
	if env.Error.Code != "rate_limited" {
		t.Fatalf("code = %q, want rate_limited", env.Error.Code)
	}
}

// slackSignature computes the v0-signed Slack signature used by tests.
func slackSignature(secret, ts, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("v0:" + ts + ":" + body))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}
