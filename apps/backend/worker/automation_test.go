package worker

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"alga/automation"
	"alga/crypto"
	"alga/store"
)

type automationRoundTripper func(*http.Request) (*http.Response, error)

func (f automationRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestAutomationWebhookEnvelopeAndRetryClassification(t *testing.T) {
	t.Setenv("ENCRYPTION_KEYS", "1:"+base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("SECRET_PEPPER", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	keyring, err := crypto.LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := keyring.EncryptString("receiver-secret")
	if err != nil {
		t.Fatal(err)
	}
	claim := store.ClaimedAutomationRun{
		ID: uuid.New(), RuleID: uuid.New(), EventID: uuid.New(), ActionIndex: 0, Attempt: 1,
		EventPayload: json.RawMessage(`{"event_type":"alert.created","entity_type":"alert","entity_id":"11111111-1111-1111-1111-111111111111","entity_number":42,"severity":"critical","labels":{"alertname":"DBDown"}}`),
	}
	action := automation.StoredAction{Type: "webhook", URL: "https://hooks.example.com/alga", BearerTokenEnc: ciphertext}
	w := NewAutomationWorker(nil, automation.HostPolicy{Allowed: []string{"hooks.example.com"}})
	w.keyring = keyring
	statusToReturn := http.StatusAccepted
	w.client = &http.Client{Transport: automationRoundTripper(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.String() != action.URL {
			t.Errorf("request target = %s %s", req.Method, req.URL)
		}
		if req.Header.Get("Authorization") != "Bearer receiver-secret" {
			t.Error("bearer credential missing")
		}
		if req.Header.Get("Idempotency-Key") != claim.ID.String()+"-0" {
			t.Error("unstable idempotency key")
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), `"entity_number":42`) || strings.Contains(string(body), "receiver-secret") {
			t.Errorf("invalid envelope: %s", body)
		}
		return &http.Response{StatusCode: statusToReturn, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})}
	status, code, retry := w.sendWebhook(context.Background(), claim, action)
	if status == nil || *status != http.StatusAccepted || code != "" || retry {
		t.Fatalf("success: status=%v code=%s retry=%v", status, code, retry)
	}
	statusToReturn = http.StatusTooManyRequests
	status, code, retry = w.sendWebhook(context.Background(), claim, action)
	if status == nil || code != "remote_retryable" || !retry {
		t.Fatalf("429: status=%v code=%s retry=%v", status, code, retry)
	}
	statusToReturn = http.StatusBadRequest
	status, code, retry = w.sendWebhook(context.Background(), claim, action)
	if status == nil || code != "remote_rejected" || retry {
		t.Fatalf("400: status=%v code=%s retry=%v", status, code, retry)
	}
	bad := action
	bad.URL = "https://other.example.com/alga"
	status, code, retry = w.sendWebhook(context.Background(), claim, bad)
	if status != nil || code != "destination_not_approved" || retry {
		t.Fatalf("disallowed: status=%v code=%s retry=%v", status, code, retry)
	}
}
