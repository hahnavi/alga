package trace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	tracesdk "go.opentelemetry.io/otel/sdk/trace"

	"alga/config"
	"alga/logger"
)

// newOTLPExporter builds an OTLP/HTTP exporter that POSTs spans as OTLP-JSON to
// the configured collector. It intentionally uses the OTLP/HTTP JSON encoding
// (Content-Type application/json on /v1/traces) rather than the gRPC/protobuf
// path so the backend avoids pulling in the protobuf/grpc-gateway dependency
// chain, which conflicts with an older module in this repo's workspace.
//
// Endpoints are resolved from (in priority order):
//   - OTEL_EXPORTER_OTLP_TRACES_ENDPOINT
//   - OTEL_EXPORTER_OTLP_ENDPOINT (a gRPC :4317 endpoint is rewritten to :4318)
//   - the alga config field OTELExporterOTLPEndpoint
//
// A missing scheme defaults to http. Optional per-request headers
// (e.g. Authorization) are taken from OTEL_EXPORTER_OTLP_HEADERS
// ("k1=v1,k2=v2") and OTEL_EXPORTER_OTLP_TRACES_HEADERS.
func newOTLPExporter(cfg *config.Config) (tracesdk.SpanExporter, error) {
	endpoint := firstNonEmpty(
		os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"),
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		cfg.OTELExporterOTLPEndpoint,
	)
	if endpoint == "" {
		return nil, fmt.Errorf("otlp exporter requires an endpoint (OTEL_EXPORTER_OTLP_ENDPOINT / OTEL_EXPORTER_OTLP_TRACES_ENDPOINT)")
	}

	tracesURL, err := resolveTracesURL(endpoint)
	if err != nil {
		return nil, fmt.Errorf("resolve otlp traces url: %w", err)
	}

	headers := parseOtlpHeaders(
		os.Getenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS"),
		os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"),
	)

	return &otlpHTTPExporter{
		client:  &http.Client{Timeout: 10 * time.Second},
		url:     tracesURL,
		headers: headers,
	}, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// resolveTracesURL turns a possibly-gRPC OTLP endpoint into an OTLP/HTTP
// /v1/traces URL. A bare host:port is treated as HTTP on port 4318. A gRPC
// :4317 endpoint is rewritten to :4318. Any existing path is replaced with
// /v1/traces.
func resolveTracesURL(endpoint string) (string, error) {
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	host := u.Host
	if u.Port() == "4317" {
		host = u.Hostname() + ":4318"
	} else if u.Port() == "" {
		host = u.Hostname() + ":4318"
	}
	scheme := u.Scheme
	if scheme == "" {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/v1/traces", scheme, host), nil
}

func parseOtlpHeaders(pairs ...string) map[string]string {
	headers := map[string]string{}
	for _, p := range pairs {
		if p == "" {
			continue
		}
		for _, kv := range strings.Split(p, ",") {
			kv = strings.TrimSpace(kv)
			if kv == "" {
				continue
			}
			eq := strings.Index(kv, "=")
			if eq < 0 {
				continue
			}
			headers[strings.TrimSpace(kv[:eq])] = strings.TrimSpace(kv[eq+1:])
		}
	}
	return headers
}

type otlpHTTPExporter struct {
	client  *http.Client
	url     string
	headers map[string]string
}

func (e *otlpHTTPExporter) ExportSpans(ctx context.Context, spans []tracesdk.ReadOnlySpan) error {
	if len(spans) == 0 {
		return nil
	}
	payload := buildTracesPayload(spans)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal otlp payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build otlp request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range e.headers {
		req.Header.Set(k, v)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("otlp export: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	detail, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	logger.Warn("otlp export rejected by collector",
		"component", "trace", "status", resp.StatusCode, "detail", string(detail))
	if resp.StatusCode >= 500 {
		// Retryable: signal the batcher to retry.
		return fmt.Errorf("otlp collector returned %d", resp.StatusCode)
	}
	return nil
}

func (e *otlpHTTPExporter) Shutdown(ctx context.Context) error {
	return nil
}

// buildTracesPayload converts OpenTelemetry SDK spans into the OTLP/JSON
// representation, grouped by resource.
func buildTracesPayload(spans []tracesdk.ReadOnlySpan) map[string]any {
	resourceSpans := make([]map[string]any, 0, len(spans))
	groups := map[string][]tracesdk.ReadOnlySpan{}

	for _, s := range spans {
		resName := ""
		if res := s.Resource(); res != nil {
			if a, ok := res.Set().Value("service.name"); ok {
				resName = a.AsString()
			}
		}
		groups[resName] = append(groups[resName], s)
	}

	for resName, group := range groups {
		rs := map[string]any{}
		if resName != "" {
			rs["resource"] = map[string]any{
				"attributes": []map[string]any{{
					"key":   "service.name",
					"value": map[string]any{"stringValue": resName},
				}},
			}
		}
		scopeSpans := make([]map[string]any, 0, len(group))
		for _, s := range group {
			scope := s.InstrumentationScope()
			ss := map[string]any{
				"scope": map[string]any{
					"name":    scope.Name,
					"version": scope.Version,
				},
				"spans": []map[string]any{spanToOTLP(s)},
			}
			if len(scope.Attributes.ToSlice()) > 0 {
				ss["scope"].(map[string]any)["attributes"] = attributeKeyValuesToOTLP(scope.Attributes.ToSlice())
			}
			scopeSpans = append(scopeSpans, ss)
		}
		rs["scopeSpans"] = scopeSpans
		resourceSpans = append(resourceSpans, rs)
	}

	return map[string]any{"resourceSpans": resourceSpans}
}

func spanToOTLP(s tracesdk.ReadOnlySpan) map[string]any {
	sc := s.SpanContext()
	tid := sc.TraceID()
	sid := sc.SpanID()
	span := map[string]any{
		"traceId":           hexEncode(tid[:]),
		"spanId":            hexEncode(sid[:]),
		"name":              s.Name(),
		"kind":              int64(s.SpanKind()),
		"startTimeUnixNano": strconv.FormatInt(s.StartTime().UnixNano(), 10),
		"endTimeUnixNano":   strconv.FormatInt(s.EndTime().UnixNano(), 10),
		"attributes":        attributeKeyValuesToOTLP(s.Attributes()),
	}

	if parent := s.Parent(); parent.IsValid() {
		psid := parent.SpanID()
		span["parentSpanId"] = hexEncode(psid[:])
	}

	status := s.Status()
	code := int64(codes.Unset)
	switch status.Code {
	case codes.Error:
		code = int64(codes.Error)
	case codes.Ok:
		code = int64(codes.Ok)
	}
	st := map[string]any{"code": code}
	if status.Description != "" {
		st["message"] = status.Description
	}
	span["status"] = st

	if evs := s.Events(); len(evs) > 0 {
		events := make([]map[string]any, 0, len(evs))
		for _, ev := range evs {
			events = append(events, map[string]any{
				"name":                   ev.Name,
				"timeUnixNano":           strconv.FormatInt(ev.Time.UnixNano(), 10),
				"attributes":             attributeKeyValuesToOTLP(ev.Attributes),
				"droppedAttributesCount": int64(ev.DroppedAttributeCount),
			})
		}
		span["events"] = events
	}

	span["droppedAttributesCount"] = int64(s.DroppedAttributes())
	span["droppedEventsCount"] = int64(s.DroppedEvents())
	span["droppedLinksCount"] = int64(s.DroppedLinks())

	if links := s.Links(); len(links) > 0 {
		ls := make([]map[string]any, 0, len(links))
		for _, l := range links {
			ltid := l.SpanContext.TraceID()
			lsid := l.SpanContext.SpanID()
			ls = append(ls, map[string]any{
				"traceId":    hexEncode(ltid[:]),
				"spanId":     hexEncode(lsid[:]),
				"attributes": attributeKeyValuesToOTLP(l.Attributes),
			})
		}
		span["links"] = ls
	}

	return span
}

func attributeKeyValuesToOTLP(kvs []attribute.KeyValue) []map[string]any {
	out := make([]map[string]any, 0, len(kvs))
	for _, kv := range kvs {
		out = append(out, map[string]any{
			"key":   string(kv.Key),
			"value": attributeValueToOTLP(kv.Value),
		})
	}
	return out
}

func attributeValueToOTLP(v attribute.Value) map[string]any {
	switch v.Type() {
	case attribute.SLICE:
		arr := v.AsSlice()
		vals := make([]map[string]any, 0, len(arr))
		for _, e := range arr {
			vals = append(vals, attributeValueToOTLP(e))
		}
		return map[string]any{"arrayValue": map[string]any{"values": vals}}
	case attribute.BOOL:
		return map[string]any{"boolValue": v.AsBool()}
	case attribute.INT64:
		return map[string]any{"intValue": strconv.FormatInt(v.AsInt64(), 10)}
	case attribute.FLOAT64:
		return map[string]any{"doubleValue": v.AsFloat64()}
	case attribute.STRING:
		return map[string]any{"stringValue": v.AsString()}
	default:
		return map[string]any{"stringValue": v.String()}
	}
}

func hexEncode(b []byte) string {
	const hextable = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, c := range b {
		out[i*2] = hextable[c>>4]
		out[i*2+1] = hextable[c&0x0f]
	}
	return string(out)
}
