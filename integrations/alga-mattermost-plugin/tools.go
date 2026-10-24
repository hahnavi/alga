//go:build tools

package main

// Keep fixed OpenTelemetry module versions in this dependency graph. grpc
// v1.84.0 requires v1.44.0; v1.45.0 fixes CVE-2026-45404 in the OpenTracing
// bridge and CVE-2026-81870 in the trace SDK. These tools-only imports are not
// linked into the plugin binary.
import (
	_ "go.opentelemetry.io/otel/bridge/opentracing"
	_ "go.opentelemetry.io/otel/sdk/trace"
)
