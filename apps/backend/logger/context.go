package logger

import (
	"context"
	"log/slog"

	oteltrace "go.opentelemetry.io/otel/trace"
)

// Context keys for the correlation identifiers injected into every log record by
// contextHandler. They are unexported struct types so they cannot collide with
// keys set by other packages.
type (
	requestIDKey struct{}
	userIDKey    struct{}
	traceIDKey   struct{}
	spanIDKey    struct{}
)

// WithRequestID returns a context carrying the request id, auto-injected into
// logs as request_id.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFrom returns the request id carried in ctx (set by
// RequestIDMiddleware via WithRequestID), or "" when absent.
func RequestIDFrom(ctx context.Context) string {
	return ctxString(ctx, requestIDKey{})
}

// WithUser returns a context carrying the authenticated user id, auto-injected
// into logs as user_id.
func WithUser(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey{}, id)
}

// WithTrace returns a context carrying an explicit trace id, auto-injected into
// logs as trace_id. When unset, contextHandler falls back to the active
// OpenTelemetry span context.
func WithTrace(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, id)
}

// WithSpan returns a context carrying an explicit span id, auto-injected into
// logs as span_id. When unset, contextHandler falls back to the active
// OpenTelemetry span context.
func WithSpan(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, spanIDKey{}, id)
}

func ctxString(ctx context.Context, key any) string {
	if v, ok := ctx.Value(key).(string); ok {
		return v
	}
	return ""
}

// contextHandler is a slog.Handler middleware that injects the correlation
// identifiers (request_id, trace_id, span_id, user_id) carried in the request
// context into every log record. trace_id/span_id fall back to the active
// OpenTelemetry span context so any log emitted inside a span is automatically
// correlated with no developer effort. Identifiers that are absent are simply
// not added, so plain (non-request) logs stay unchanged.
type contextHandler struct {
	inner slog.Handler
}

func (h *contextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *contextHandler) Handle(ctx context.Context, r slog.Record) error {
	if ctx != nil {
		traceID := ctxString(ctx, traceIDKey{})
		spanID := ctxString(ctx, spanIDKey{})
		if traceID == "" || spanID == "" {
			if sc := oteltrace.SpanContextFromContext(ctx); sc.IsValid() {
				if traceID == "" {
					traceID = sc.TraceID().String()
				}
				if spanID == "" {
					spanID = sc.SpanID().String()
				}
			}
		}
		if id := ctxString(ctx, requestIDKey{}); id != "" {
			r.AddAttrs(slog.String("request_id", id))
		}
		if traceID != "" {
			r.AddAttrs(slog.String("trace_id", traceID))
		}
		if spanID != "" {
			r.AddAttrs(slog.String("span_id", spanID))
		}
		if id := ctxString(ctx, userIDKey{}); id != "" {
			r.AddAttrs(slog.String("user_id", id))
		}
	}
	return h.inner.Handle(ctx, r)
}

func (h *contextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &contextHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *contextHandler) WithGroup(name string) slog.Handler {
	return &contextHandler{inner: h.inner.WithGroup(name)}
}
