package trace

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"

	tracesdk "go.opentelemetry.io/otel/sdk/trace"

	"alga/config"
)

func TestOTLPExporterExportsJSON(t *testing.T) {
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1/traces") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected content type %q", r.Header.Get("Content-Type"))
		}
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	exp, err := newOTLPExporter(&config.Config{OTELExporterOTLPEndpoint: srv.URL})
	if err != nil {
		t.Fatalf("newOTLPExporter: %v", err)
	}

	tp := tracesdk.NewTracerProvider(
		tracesdk.WithSyncer(exp),
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
	)
	tr := tp.Tracer("alga-test")
	_, span := tr.Start(context.Background(), "db.query",
		oteltrace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.Int64("count", 3),
			attribute.Bool("ok", true),
		))
	span.SetStatus(codes.Error, "boom")
	span.End()
	if err := tp.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}

	var payload struct {
		ResourceSpans []struct {
			ScopeSpans []struct {
				Spans []struct {
					Name              string `json:"name"`
					Kind              int64  `json:"kind"`
					StartTimeUnixNano string `json:"startTimeUnixNano"`
					Status            struct {
						Code    int64  `json:"code"`
						Message string `json:"message"`
					} `json:"status"`
				} `json:"spans"`
			} `json:"scopeSpans"`
		} `json:"resourceSpans"`
	}
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v\nbody: %s", err, gotBody)
	}
	if len(payload.ResourceSpans) == 0 || len(payload.ResourceSpans[0].ScopeSpans) == 0 {
		t.Fatalf("missing resource/scope spans: %s", gotBody)
	}
	spans := payload.ResourceSpans[0].ScopeSpans[0].Spans
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if spans[0].Name != "db.query" {
		t.Errorf("name = %q", spans[0].Name)
	}
	if spans[0].Status.Code != int64(codes.Error) || spans[0].Status.Message != "boom" {
		t.Errorf("status = %+v", spans[0].Status)
	}
	if spans[0].StartTimeUnixNano == "" {
		t.Errorf("startTimeUnixNano empty")
	}
}

func TestResolveTracesURLRewritesGRPC(t *testing.T) {
	u, err := resolveTracesURL("http://collector:4317")
	if err != nil {
		t.Fatal(err)
	}
	if u != "http://collector:4318/v1/traces" {
		t.Errorf("got %q", u)
	}
	u, _ = resolveTracesURL("collector.example.com")
	if u != "http://collector.example.com:4318/v1/traces" {
		t.Errorf("got %q", u)
	}
}
