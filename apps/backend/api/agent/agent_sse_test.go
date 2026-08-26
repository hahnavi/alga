package agent_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/api/agent"
	"alga/logger"
	"alga/sse"
	"alga/store"
)

// stubSSEAgentTokenStore satisfies store.AgentTokenStore for the SSE auth
// path; everything except ValidateToken is out of scope here.
type stubSSEAgentTokenStore struct {
	store.AgentTokenStore
	validToken string
	seenToken  string
}

func (s *stubSSEAgentTokenStore) ValidateToken(token string) (*store.AgentTokenRecord, error) {
	s.seenToken = token
	if token == s.validToken {
		return &store.AgentTokenRecord{ID: uuid.New(), Name: "sse-test"}, nil
	}
	return nil, nil
}

// newSSETestHandler builds a handler with a real broker and no-op presence/
// executor so the auth path can be exercised up to (but not through) the
// streaming phase.
func newSSETestHandler(validToken string, allowQuery bool) (*agent.AgentSSEHandler, *stubSSEAgentTokenStore) {
	logger.Init("error", "")
	tokenStore := &stubSSEAgentTokenStore{validToken: validToken}
	executor := agent.NewAgentToolExecutor(nil, nil, nil, nil, nil)
	h := agent.NewAgentSSEHandler(sse.NewBroker(), nil, nil, tokenStore, executor)
	h.SetAllowQueryToken(allowQuery)
	return h, tokenStore
}

// TestAgentSSEQueryTokenDeniedByDefault covers WP-B3: the `?token=` fallback
// on the agent SSE endpoint is deny-by-default; header auth always works.
func TestAgentSSEQueryTokenDeniedByDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		target        string
		authHeader    string
		allowQuery    bool
		wantStatus    int
		wantSeenToken string
	}{
		{
			// Auth succeeds and the stream starts (200 + "connected" frame);
			// the test cancels the request context to end it.
			name:          "header token authenticates",
			target:        "/api/v1/agent/events",
			authHeader:    "Bearer alga_agent_valid",
			wantStatus:    http.StatusOK,
			wantSeenToken: "alga_agent_valid",
		},
		{
			name:          "query token denied under default config",
			target:        "/api/v1/agent/events?token=alga_agent_valid",
			wantStatus:    http.StatusUnauthorized,
			wantSeenToken: "",
		},
		{
			name:          "query token accepted with AGENT_SSE_ALLOW_QUERY_TOKEN=true",
			target:        "/api/v1/agent/events?token=alga_agent_valid",
			allowQuery:    true,
			wantStatus:    http.StatusOK,
			wantSeenToken: "alga_agent_valid",
		},
		{
			name:       "missing token",
			target:     "/api/v1/agent/events",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, tokenStore := newSSETestHandler("alga_agent_valid", tt.allowQuery)
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}
			// A successful auth proceeds into the SSE stream loop, which only
			// ends when the request context is cancelled — cancel after the
			// first flushed frame so the handler returns.
			ctx, cancel := context.WithCancel(req.Context())
			req = req.WithContext(ctx)
			rec := httptest.NewRecorder()
			go func() {
				for rec.Body.Len() == 0 && ctx.Err() == nil {
					time.Sleep(time.Millisecond)
				}
				cancel()
			}()

			h.Handler()(rec, req)
			cancel()

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tokenStore.seenToken != tt.wantSeenToken {
				t.Fatalf("validated token = %q, want %q", tokenStore.seenToken, tt.wantSeenToken)
			}
		})
	}
}
