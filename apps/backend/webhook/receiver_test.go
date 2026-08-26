package webhook

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/logger"
	"alga/store"
)

type receiverWebhookTokenStore struct {
	validToken string
	seenToken  string
}

func (s *receiverWebhookTokenStore) CreateToken(name string, expiresAt *time.Time) (*store.WebhookTokenRecord, error) {
	return nil, nil
}

func (s *receiverWebhookTokenStore) ListTokens() ([]store.WebhookTokenRecord, error) {
	return nil, nil
}

func (s *receiverWebhookTokenStore) RevokeToken(id uuid.UUID) error {
	return nil
}

func (s *receiverWebhookTokenStore) ValidateToken(token string) (bool, error) {
	s.seenToken = token
	return token == s.validToken, nil
}

func (s *receiverWebhookTokenStore) Close() {}

func TestHandleWebhookAuthMethods(t *testing.T) {
	logger.Init("error", "")

	tests := []struct {
		name          string
		target        string
		authHeader    string
		allowQuery    bool
		wantStatus    int
		wantSeenToken string
	}{
		{
			name:          "bearer token",
			target:        "/webhooks/alerts",
			authHeader:    "Bearer test-token",
			wantStatus:    http.StatusOK,
			wantSeenToken: "test-token",
		},
		{
			name:          "lowercase bearer token",
			target:        "/webhooks/alerts",
			authHeader:    "bearer test-token",
			wantStatus:    http.StatusOK,
			wantSeenToken: "test-token",
		},
		{
			name:          "query token denied by default (WP-B3)",
			target:        "/webhooks/alerts?token=test-token",
			wantStatus:    http.StatusUnauthorized,
			wantSeenToken: "",
		},
		{
			name:          "query token allowed with WEBHOOK_ALLOW_QUERY_TOKEN=true",
			target:        "/webhooks/alerts?token=test-token",
			allowQuery:    true,
			wantStatus:    http.StatusOK,
			wantSeenToken: "test-token",
		},
		{
			name:       "missing token",
			target:     "/webhooks/alerts",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokenStore := &receiverWebhookTokenStore{validToken: "test-token"}
			receiver := NewReceiver(nil, nil, nil, nil, tokenStore, nil, false)
			receiver.SetAllowQueryToken(tt.allowQuery)
			req := httptest.NewRequest(http.MethodPost, tt.target, strings.NewReader(`{"alerts":[]}`))
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			rec := httptest.NewRecorder()

			receiver.handleWebhook(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tokenStore.seenToken != tt.wantSeenToken {
				t.Fatalf("validated token = %q, want %q", tokenStore.seenToken, tt.wantSeenToken)
			}
		})
	}
}

func TestMatchLabels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		matchers map[string]string
		labels   map[string]string
		want     bool
	}{
		{
			name:     "empty matchers match every label set (documented catch-all)",
			matchers: map[string]string{},
			labels:   map[string]string{"severity": "critical", "team": "db"},
			want:     true,
		},
		{
			name:     "empty matchers match empty labels",
			matchers: map[string]string{},
			labels:   nil,
			want:     true,
		},
		{
			name:     "concrete matcher hit",
			matchers: map[string]string{"severity": "critical"},
			labels:   map[string]string{"severity": "critical"},
			want:     true,
		},
		{
			name:     "concrete matcher miss",
			matchers: map[string]string{"severity": "critical"},
			labels:   map[string]string{"severity": "warning"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := matchLabels(tt.matchers, tt.labels); got != tt.want {
				t.Fatalf("matchLabels(%v, %v) = %v, want %v", tt.matchers, tt.labels, got, tt.want)
			}
		})
	}
}
