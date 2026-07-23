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
			name:          "query token",
			target:        "/webhooks/alerts?token=test-token",
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
			receiver := NewReceiver(nil, nil, nil, nil, tokenStore, nil)
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
