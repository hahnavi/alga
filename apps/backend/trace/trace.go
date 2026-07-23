// Package trace wires OpenTelemetry distributed tracing for the Alga backend.
//
// Tracing is ENV-GATED and OFF by default. When disabled (the normal case for
// local dev and tests) a noop TracerProvider is installed, so no spans are
// created or exported and the cost is effectively zero. When an OTLP endpoint is
// configured (OTEL_EXPORTER_OTLP_ENDPOINT / OTEL_EXPORTER_OTLP_TRACES_ENDPOINT)
// or ALGA_OTEL_ENABLED=true, a real TracerProvider exporting via OTLP/HTTP
// (JSON) is installed.
//
// The global text map propagator (W3C TraceContext + Baggage) is always
// configured so cross-process context propagation (HTTP, RabbitMQ) behaves
// uniformly regardless of whether export is enabled.
package trace

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"alga/config"
)

const tracerName = "alga"

var (
	tp      *tracesdk.TracerProvider
	enabled bool
)

// Init installs the global OpenTelemetry TracerProvider and text map propagator.
// It returns a shutdown function that flushes and stops the provider (safe to
// call even when tracing is disabled). When tracing is not enabled a noop
// provider is installed and the shutdown is a no-op.
func Init(cfg *config.Config) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if !isEnabled(cfg) {
		otel.SetTracerProvider(noop.NewTracerProvider())
		enabled = false
		return func(context.Context) error { return nil }, nil
	}
	enabled = true

	ctx := context.Background()

	exp, err := newOTLPExporter(cfg)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	ratio := cfg.OTELSampleRatio
	if ratio <= 0 {
		ratio = 1.0
	}
	sampler := tracesdk.ParentBased(tracesdk.TraceIDRatioBased(ratio))

	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", "alga-backend")),
		resource.WithFromEnv(),
		resource.WithHost(),
		resource.WithProcess(),
	)
	if err != nil {
		return nil, fmt.Errorf("create otel resource: %w", err)
	}

	tp = tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exp),
		tracesdk.WithSampler(sampler),
		tracesdk.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

func isEnabled(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	if cfg.OTELTracingEnabled {
		return true
	}
	if cfg.OTELExporterOTLPEndpoint != "" {
		return true
	}
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		return true
	}
	if os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" {
		return true
	}
	return false
}

// Enabled reports whether a real (exporting) TracerProvider is active.
func Enabled() bool { return enabled }

// Tracer returns the package tracer used for manually created spans.
func Tracer() oteltrace.Tracer { return otel.Tracer(tracerName) }

// Propagator returns the configured W3C text map propagator.
func Propagator() propagation.TextMapPropagator { return otel.GetTextMapPropagator() }

// PGXTracer returns a pgx.QueryTracer that emits a span per SQL query, or nil
// when tracing is disabled so the database driver stays zero-overhead. Attach
// the returned value to pgx.ConnConfig.Tracer.
func PGXTracer() *pgxQueryTracer {
	if !enabled {
		return nil
	}
	return &pgxQueryTracer{}
}

// pgxQueryTracer implements pgx.QueryTracer, creating a client-side span for
// each SQL statement executed through the pgx stdlib driver (used by Ent and
// raw *sql.DB queries).
type pgxQueryTracer struct{}

func (t *pgxQueryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	spanCtx, _ := Tracer().Start(ctx, "db.query",
		oteltrace.WithSpanKind(oteltrace.SpanKindClient),
		oteltrace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.statement", data.SQL),
		),
	)
	return spanCtx
}

func (t *pgxQueryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span := oteltrace.SpanFromContext(ctx)
	if span == nil {
		return
	}
	defer span.End()
	if data.Err != nil && !errors.Is(data.Err, pgx.ErrNoRows) {
		span.RecordError(data.Err)
		span.SetAttributes(attribute.Bool("error", true))
	}
}
