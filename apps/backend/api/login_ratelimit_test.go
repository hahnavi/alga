package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"alga/config"
	"alga/store"
)

// stubLoginLimiter lets tests force specific limiter outcomes, including the
// deny-with-nil-lock path produced by ValkeyLoginRateLimiter on backend errors.
type stubLoginLimiter struct {
	allowed     bool
	remaining   int
	lockedUntil *time.Time
	resetCalled bool
}

func (l *stubLoginLimiter) CheckLoginAllowed(string) (bool, int, *time.Time) {
	return l.allowed, l.remaining, l.lockedUntil
}

func (l *stubLoginLimiter) Reset(string) { l.resetCalled = true }

func (l *stubLoginLimiter) Stop() {}

// recordingAuditStore captures audit calls so tests can assert fire-and-forget
// logging without a database.
type recordingAuditStore struct {
	events []store.AuditEvent
}

func (s *recordingAuditStore) Log(event store.AuditEvent, _ *uuid.UUID, _ string, _, _ string, _ bool, _ map[string]any) {
	s.events = append(s.events, event)
}

func (s *recordingAuditStore) LogEntity(event store.AuditEvent, _ *uuid.UUID, _ string, _, _ string, _ bool, _ map[string]any, _ string, _ *uuid.UUID) {
	s.events = append(s.events, event)
}

func (s *recordingAuditStore) Query(map[string]any) ([]store.AuditRecord, error) {
	return nil, nil
}

func (s *recordingAuditStore) GetRecentEvents(int) ([]store.AuditRecord, error) {
	return nil, nil
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestHandleLoginRateLimited(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		limiter          stubLoginLimiter
		wantStatus       int
		wantRetryPrefix  string
		wantRetryPresent bool
	}{
		{
			name:             "denied with lock expiry emits Retry-After",
			limiter:          stubLoginLimiter{allowed: false, lockedUntil: ptrTime(time.Now().Add(30 * time.Second))},
			wantStatus:       http.StatusTooManyRequests,
			wantRetryPrefix:  "2",
			wantRetryPresent: true,
		},
		{
			name:             "denied without lock expiry omits Retry-After without panicking",
			limiter:          stubLoginLimiter{allowed: false, lockedUntil: nil},
			wantStatus:       http.StatusTooManyRequests,
			wantRetryPresent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			s := &Server{
				cfg:          &config.Config{},
				loginLimiter: &tt.limiter,
				auditStore:   &recordingAuditStore{},
				ipExtractor:  newIPExtractor(&config.Config{}),
			}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
			req.RemoteAddr = "10.0.0.1:1234"
			w := httptest.NewRecorder()

			s.handleLogin(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			got := w.Header().Get("Retry-After")
			if tt.wantRetryPresent && got == "" {
				t.Fatal("Retry-After header missing, want present")
			}
			if !tt.wantRetryPresent && got != "" {
				t.Fatalf("Retry-After = %q, want omitted", got)
			}
			if tt.wantRetryPresent && !strings.HasPrefix(got, tt.wantRetryPrefix) {
				t.Fatalf("Retry-After = %s, want starting %s", got, tt.wantRetryPrefix)
			}
		})
	}
}

func TestHandleLoginRateLimited_AuditsFailure(t *testing.T) {
	t.Parallel()

	audit := &recordingAuditStore{}
	s := &Server{
		cfg:          &config.Config{},
		loginLimiter: &stubLoginLimiter{allowed: false},
		auditStore:   audit,
		ipExtractor:  newIPExtractor(&config.Config{}),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	w := httptest.NewRecorder()
	s.handleLogin(w, req)

	if len(audit.events) != 1 || audit.events[0] != store.AuditLoginFailed {
		t.Fatalf("audit events = %v, want [login_failed]", audit.events)
	}
}
