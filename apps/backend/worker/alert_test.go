package worker

import (
	"encoding/json"
	"testing"

	"alga/rabbitmq"
	"alga/types"
)

func TestAlertWorkerQueue(t *testing.T) {
	t.Parallel()
	w := &AlertWorker{}
	if got := w.Queue(); got != rabbitmq.QueueAlertProcess {
		t.Fatalf("Queue() = %q, want %q", got, rabbitmq.QueueAlertProcess)
	}
}

func TestAlertWorkerPrefetchCount(t *testing.T) {
	t.Parallel()
	w := &AlertWorker{}
	if got := w.PrefetchCount(); got != 10 {
		t.Fatalf("PrefetchCount() = %d, want 10", got)
	}
}

func TestAlertMessageRoundTrip(t *testing.T) {
	t.Parallel()
	msg := rabbitmq.AlertMessage{
		Payload: types.GrafanaAlertingPayload{
			Alerts: []types.Alert{
				{
					Fingerprint: "fp1",
					Status:      "firing",
					Labels:      map[string]string{"alertname": "TestAlert"},
					Annotations: map[string]string{"summary": "test"},
				},
			},
		},
		RetryCount: 2,
	}
	body, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded rabbitmq.AlertMessage
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.RetryCount != 2 {
		t.Fatalf("RetryCount = %d, want 2", decoded.RetryCount)
	}
	if len(decoded.Payload.Alerts) != 1 {
		t.Fatalf("len(Alerts) = %d, want 1", len(decoded.Payload.Alerts))
	}
	if decoded.Payload.Alerts[0].Fingerprint != "fp1" {
		t.Fatalf("Fingerprint = %q, want %q", decoded.Payload.Alerts[0].Fingerprint, "fp1")
	}
}

func TestAlertMessageRetryCountPreserved(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		retryCount int
	}{
		{name: "initial", retryCount: 0},
		{name: "first_retry", retryCount: 1},
		{name: "max_retries", retryCount: rabbitmq.MaxAlertRetries},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := rabbitmq.AlertMessage{RetryCount: tc.retryCount}
			body, _ := json.Marshal(msg)
			var decoded rabbitmq.AlertMessage
			if err := json.Unmarshal(body, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if decoded.RetryCount != tc.retryCount {
				t.Fatalf("RetryCount = %d, want %d", decoded.RetryCount, tc.retryCount)
			}
		})
	}
}

func TestAuditWorkerQueue(t *testing.T) {
	t.Parallel()
	w := &AuditWorker{}
	if got := w.Queue(); got != rabbitmq.QueueAuditLog {
		t.Fatalf("Queue() = %q, want %q", got, rabbitmq.QueueAuditLog)
	}
}

func TestAuditWorkerPrefetchCount(t *testing.T) {
	t.Parallel()
	w := &AuditWorker{}
	if got := w.PrefetchCount(); got != 10 {
		t.Fatalf("PrefetchCount() = %d, want 10", got)
	}
}

func TestNotificationWorkerQueue(t *testing.T) {
	t.Parallel()
	w := &NotificationWorker{}
	if got := w.Queue(); got != rabbitmq.QueueNotificationSend {
		t.Fatalf("Queue() = %q, want %q", got, rabbitmq.QueueNotificationSend)
	}
}

func TestNotificationWorkerPrefetchCount(t *testing.T) {
	t.Parallel()
	w := &NotificationWorker{}
	if got := w.PrefetchCount(); got != 10 {
		t.Fatalf("PrefetchCount() = %d, want 10", got)
	}
}

func TestAuditDetailsRoundTrip(t *testing.T) {
	t.Parallel()
	details := AuditDetails{
		Event:     "login_success",
		Username:  "admin",
		IP:        "127.0.0.1",
		UserAgent: "test-agent",
		Success:   true,
	}
	body, err := json.Marshal(details)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded AuditDetails
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Event != "login_success" {
		t.Fatalf("Event = %q, want %q", decoded.Event, "login_success")
	}
	if !decoded.Success {
		t.Fatalf("Success = false, want true")
	}
}
