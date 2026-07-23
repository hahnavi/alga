package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestContextHandlerInjectsFields(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{})
	l := slog.New(&contextHandler{inner: inner})

	ctx := WithRequestID(context.Background(), "req-1")
	ctx = WithUser(ctx, "user-9")
	ctx = WithTrace(ctx, "trace-abc")
	ctx = WithSpan(ctx, "span-def")

	l.InfoContext(ctx, "hello")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, buf.String())
	}
	if rec["msg"] != "hello" {
		t.Errorf("msg = %v", rec["msg"])
	}
	if rec["request_id"] != "req-1" {
		t.Errorf("request_id = %v", rec["request_id"])
	}
	if rec["user_id"] != "user-9" {
		t.Errorf("user_id = %v", rec["user_id"])
	}
	if rec["trace_id"] != "trace-abc" {
		t.Errorf("trace_id = %v", rec["trace_id"])
	}
	if rec["span_id"] != "span-def" {
		t.Errorf("span_id = %v", rec["span_id"])
	}
}

func TestContextHandlerNoFieldsWhenAbsent(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{})
	l := slog.New(&contextHandler{inner: inner})
	l.InfoContext(context.Background(), "plain")
	if len(buf.Bytes()) == 0 {
		t.Fatal("no output")
	}
}
