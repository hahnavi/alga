package rabbitmq

import (
	"context"
	"errors"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestIsPreconditionFailed(t *testing.T) {
	if !isPreconditionFailed(&amqp.Error{Code: amqp.PreconditionFailed, Reason: "inequivalent arg"}) {
		t.Error("expected PreconditionFailed to be detected")
	}
	if isPreconditionFailed(&amqp.Error{Code: amqp.ResourceLocked, Reason: "locked"}) {
		t.Error("non-PreconditionFailed error should not match")
	}
	if isPreconditionFailed(nil) {
		t.Error("nil error should not match")
	}
	if isPreconditionFailed(errors.New("plain error")) {
		t.Error("plain error should not match")
	}
}

func TestInjectTraceHeaders(t *testing.T) {
	sc := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    mustTraceID(t, "4bf92f3577b34da6a3ce929d0e0e4736"),
		SpanID:     mustSpanID(t, "00f067aa0ba902b7"),
		TraceFlags: oteltrace.FlagsSampled,
	})
	ctx := oteltrace.ContextWithSpanContext(context.Background(), sc)
	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})

	headers := injectTraceHeaders(ctx, prop, nil)
	carrier := NewAMQPCarrier(headers)
	tp := carrier.Get("traceparent")
	if tp == "" {
		t.Fatal("traceparent header not injected")
	}
	if !contains(tp, "4bf92f3577b34da6a3ce929d0e0e4736") {
		t.Errorf("traceparent = %q, want it to carry the trace id", tp)
	}

	// Injection must preserve existing headers.
	base := amqp.Table{"custom": "value"}
	merged := injectTraceHeaders(ctx, prop, base)
	if merged["custom"] != "value" {
		t.Errorf("existing header lost: %v", merged)
	}
	if NewAMQPCarrier(merged).Get("traceparent") == "" {
		t.Error("traceparent not injected alongside existing headers")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func mustTraceID(t *testing.T, s string) oteltrace.TraceID {
	t.Helper()
	id, err := oteltrace.TraceIDFromHex(s)
	if err != nil {
		t.Fatalf("bad trace id %q: %v", s, err)
	}
	return id
}

func mustSpanID(t *testing.T, s string) oteltrace.SpanID {
	t.Helper()
	id, err := oteltrace.SpanIDFromHex(s)
	if err != nil {
		t.Fatalf("bad span id %q: %v", s, err)
	}
	return id
}
