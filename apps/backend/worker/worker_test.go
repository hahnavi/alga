package worker

import (
	"context"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func TestExtractTraceContext(t *testing.T) {
	prop := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	headers := amqp.Table{
		"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	}
	ctx := extractTraceContext(context.Background(), prop, headers)
	sc := oteltrace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		t.Fatal("expected a valid remote span context after extraction")
	}
	if got := sc.TraceID().String(); got != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("extracted trace id = %s, want 4bf92f3577b34da6a3ce929d0e0e4736", got)
	}
}

func TestStartConsumeSpanDisabled(t *testing.T) {
	// With tracing disabled, startConsumeSpan must return the parent ctx
	// unchanged and a non-nil span that is safe to End().
	d := amqp.Delivery{Exchange: "ex", RoutingKey: "rk", MessageId: "m1"}
	ctx, span := startConsumeSpan(context.Background(), d)
	if span == nil {
		t.Fatal("nil span")
	}
	span.End()
	if oteltrace.SpanContextFromContext(ctx).IsValid() {
		t.Error("disabled path should not create a valid span")
	}
}
