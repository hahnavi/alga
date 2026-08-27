package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"alga/config"
	"alga/store"
)

// stubSlackCallbackUserStore implements only SetSlackIdentity, the sole
// UserStore method the Slack bind callback reaches; other methods panic via
// the embedded nil interface.
type stubSlackCallbackUserStore struct {
	store.UserStore
	setErr   error
	setCalls int
}

func (s *stubSlackCallbackUserStore) SetSlackIdentity(_ context.Context, _ uuid.UUID, _, _ string) error {
	s.setCalls++
	return s.setErr
}

// stubOAuthState forces Validate outcomes without Valkey.
type stubOAuthState struct{ valid bool }

func (s *stubOAuthState) Set(string) error              { return nil }
func (s *stubOAuthState) Validate(string) (bool, error) { return s.valid, nil }

// cannedOAuthTransport answers every outbound OAuth request in-process so the
// callback's Slack token exchange and identity fetch succeed without network.
type cannedOAuthTransport func(*http.Request) (*http.Response, error)

func (f cannedOAuthTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// withCannedSlackOAuth swaps the package OAuth client for a canned transport
// for the duration of the test. Not parallel-safe (package-level override).
func withCannedSlackOAuth(t *testing.T) {
	t.Helper()
	prev := oauthHTTPClient
	oauthHTTPClient = &http.Client{
		Transport: cannedOAuthTransport(func(r *http.Request) (*http.Response, error) {
			body := `{}`
			switch {
			case strings.Contains(r.URL.Path, "oauth.access"):
				body = `{"ok":true,"access_token":"xoxp-test"}`
			case strings.Contains(r.URL.Path, "users.identity"):
				body = `{"ok":true,"user":{"id":"U-DUP","name":"dup"}}`
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}
	t.Cleanup(func() { oauthHTTPClient = prev })
}

func newSlackCallbackRequest(userID uuid.UUID) *http.Request {
	state := "user-slack:" + userID.String() + ":0123456789abcdef"
	return httptest.NewRequest(http.MethodGet,
		"/api/v1/users/me/slack/callback?code=c&state="+url.QueryEscape(state), nil)
}

// TestHandleSlackCallbackDuplicateIdentityConflict pins the WP-C8 mapping: a
// duplicate Slack binding is a 409 conflict, not the generic save_failed
// redirect, and no linked audit event fires for the rejected bind.
func TestHandleSlackCallbackDuplicateIdentityConflict(t *testing.T) {
	withCannedSlackOAuth(t)

	stub := &stubSlackCallbackUserStore{setErr: store.ErrSlackIdentityTaken}
	audit := &recordingAuditStore{}
	h := &userSlackHandler{
		cfg:        &config.Config{SlackClientID: "cid", SlackClientSecret: "sec"},
		userStore:  stub,
		auditStore: audit,
		stateStore: &stubOAuthState{valid: true},
	}

	w := httptest.NewRecorder()
	h.handleCallback(w, newSlackCallbackRequest(uuid.New()))

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (body=%s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "conflict") {
		t.Fatalf("body = %s, want conflict error code", w.Body.String())
	}
	if stub.setCalls != 1 {
		t.Fatalf("SetSlackIdentity calls = %d, want 1", stub.setCalls)
	}
	if len(audit.events) != 0 {
		t.Errorf("audit events = %v, want none on rejected bind", audit.events)
	}
}

// TestHandleSlackCallbackSuccessRedirect keeps the happy path pinned while
// the conflict branch lands beside it.
func TestHandleSlackCallbackSuccessRedirect(t *testing.T) {
	withCannedSlackOAuth(t)

	stub := &stubSlackCallbackUserStore{}
	audit := &recordingAuditStore{}
	h := &userSlackHandler{
		cfg:        &config.Config{SlackClientID: "cid", SlackClientSecret: "sec"},
		userStore:  stub,
		auditStore: audit,
		stateStore: &stubOAuthState{valid: true},
	}

	w := httptest.NewRecorder()
	h.handleCallback(w, newSlackCallbackRequest(uuid.New()))

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 (body=%s)", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "slack_linked=success") {
		t.Fatalf("Location = %s, want slack_linked=success", loc)
	}
	if stub.setCalls != 1 {
		t.Fatalf("SetSlackIdentity calls = %d, want 1", stub.setCalls)
	}
	if len(audit.events) != 1 || audit.events[0] != store.AuditUserSlackLinked {
		t.Errorf("audit events = %v, want [user_slack_linked]", audit.events)
	}
}

// compile-time guard that the stub satisfies the interface
var _ store.UserStore = (*stubSlackCallbackUserStore)(nil)
