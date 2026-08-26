---
title: Monitoring & Observability
description: Health endpoints, Prometheus metrics, Grafana dashboards, structured logging, and alerting on Alga itself.
---

# Monitoring & Observability

## Health Endpoints

All health and metrics endpoints are unauthenticated. Gate `/metrics` at the network level (firewall, security group, or internal-only bind) so it is not reachable from the public internet.

| Endpoint                | Purpose                                           | Auth                   |
| ----------------------- | ------------------------------------------------- | ---------------------- |
| `GET /live`             | Liveness only — no dependency checks              | None                   |
| `GET /ready`            | Readiness with dependency checks                  | None                   |
| `GET /health`           | Readiness (alias for `/ready`)                    | None                   |
| `GET /api/v1/readiness` | Readiness (alias for `/ready`)                    | None                   |
| `GET /metrics`          | Prometheus-format metrics (primary scrape target) | None (network-gate it) |

### Liveness

```sh
curl http://localhost:8080/live
```

Returns `200 OK` if the HTTP server is running. Use this for container liveness probes — it does not check downstream dependencies, so a wedged database connection will not cause restarts.

### Readiness

```sh
curl http://localhost:8080/ready
```

Returns JSON with dependency connectivity (PostgreSQL, Valkey, RabbitMQ). `/health` and `/api/v1/readiness` are aliases that return the same response. Use one of these for load balancer health checks and Kubernetes readiness probes.

## Metrics

All metrics are exposed in Prometheus format at `/metrics` (the primary and only metrics endpoint). The endpoint is unauthenticated — restrict access at the network level. Every metric below is exported as a Prometheus **gauge** (all are `expvar.Int` under the hood), so counter-style series must be read with `rate()`/`increase()` knowing resets happen per process restart.

The catalog is regenerated from the registrations in `apps/backend/metrics/*.go`: 48 metrics total, including 21 scheduler gauges.

### Correlator Metrics

| Metric                              | Description                                |
| ----------------------------------- | ------------------------------------------ |
| `alga_correlator_alerts_total`      | Alerts entering correlation                |
| `alga_correlator_merged_total`      | Alerts merged into existing windows        |
| `alga_correlator_published_total`   | Investigations published after correlation |
| `alga_correlator_dropped_total`     | Alerts dropped                             |
| `alga_correlator_window_open_total` | New correlation windows opened             |
| `alga_correlator_window_depth`      | Currently open correlation windows         |
| `alga_correlator_flush_total`       | Windows flushed (expired)                  |
| `alga_correlator_fail_closed_total` | Fail-closed events                         |

### Scheduler Metrics

| Metric                                        | Description                              |
| --------------------------------------------- | ---------------------------------------- |
| `alga_scheduler_pending`                      | Pending investigation queue depth        |
| `alga_scheduler_scheduled_total`              | Successful investigation binds           |
| `alga_scheduler_bind_failed_total`            | Atomic claim or forward failures         |
| `alga_scheduler_no_candidate_total`           | Pending with no eligible agent           |
| `alga_scheduler_skip_active_backoff_total`    | Skips due to per-agent failure backoff   |
| `alga_scheduler_tick_duration_ms`             | Last tick duration                       |
| `alga_scheduler_tick_total`                   | Total scheduler ticks                    |
| `alga_scheduler_agent_capacity_used`          | Aggregate used capacity                  |
| `alga_scheduler_agent_capacity_total`         | Aggregate total capacity                 |
| `alga_scheduler_is_leader`                    | 1 on leader replica, 0 elsewhere         |
| `alga_scheduler_online_agents`                | Agents currently online (all replicas)   |
| `alga_scheduler_nudge_total`                  | Scheduler nudge (wake-up) events         |
| `alga_scheduler_stale_alerts_swept`           | Uninvestigated alerts found per sweep    |
| `alga_scheduler_stale_investigations_created` | Investigations created from stale alerts |
| `alga_scheduler_stale_sweep_tick_total`       | Total stale-alert sweep ticks            |
| `alga_scheduler_incident_sweep_tick_total`    | Total incident sweep ticks               |
| `alga_scheduler_summary_sweep_total`          | Incident-summary sweeps run              |
| `alga_scheduler_summary_dispatched_total`     | Summaries dispatched by the sweep        |
| `alga_scheduler_summary_skipped_total`        | Summary dispatches skipped               |
| `alga_scheduler_dispatch_latency_ms`          | Bind-to-agent-dispatch latency           |
| `alga_scheduler_dlq_total`                    | Messages routed to the scheduler DLQ     |

### Webhook Metrics

| Metric                                            | Description                                |
| ------------------------------------------------- | ------------------------------------------ |
| `alga_webhook_alert_publish_queued_total`         | Webhook alerts queued for publishing       |
| `alga_webhook_alert_publish_sync_fallback_total`  | Webhook alerts published via sync fallback |
| `alga_webhook_alert_publish_sync_processed_total` | Webhook alerts processed synchronously     |

### Escalation / SLA Metrics

| Metric                                      | Description                       |
| ------------------------------------------- | --------------------------------- |
| `alga_escalations_fired_total`              | Total escalations fired           |
| `alga_sla_breach_response_total`            | SLA response time breaches        |
| `alga_sla_breach_resolve_total`             | SLA resolution time breaches      |
| `alga_stuck_investigations_escalated_total` | Stuck investigations escalated    |
| `alga_voice_calls_placed_total`             | Voice escalation calls placed     |
| `alga_voice_calls_suppressed_total`         | Voice escalation calls suppressed |

### Incident Metrics

| Metric                           | Description                |
| -------------------------------- | -------------------------- |
| `alga_incidents_created_total`   | Total incidents created    |
| `alga_incidents_active`          | Currently active incidents |
| `alga_incidents_resolved_total`  | Total incidents resolved   |
| `alga_incidents_mitigated_total` | Total incidents mitigated  |
| `alga_incidents_closed_total`    | Total incidents closed     |
| `alga_incidents_cancelled_total` | Total incidents cancelled  |
| `alga_incidents_reopened_total`  | Total incidents reopened   |

### Worker Metrics

| Metric                                      | Description                           |
| ------------------------------------------- | ------------------------------------- |
| `alga_investigate_worker_create_latency_ms` | Investigation record creation latency |
| `alga_worker_dlq_total`                     | Messages routed to the worker DLQ     |

### Summary Metrics

| Metric                      | Description               |
| --------------------------- | ------------------------- |
| `alga_summary_posted_total` | Incident summaries posted |

## Prometheus Integration

Scrape the `/metrics` endpoint with Prometheus:

```yaml
scrape_configs:
  - job_name: "alga"
    static_configs:
      - targets: ["alga:8080"]
    metrics_path: "/metrics"
    scrape_interval: 15s
```

## Grafana Dashboard

Import `deploy/grafana/alga-dashboard.json` into Grafana for a pre-built monitoring dashboard with panels for:

- Alert ingestion rate
- Correlation window activity
- Scheduler bind rate and latency
- Agent online count and capacity
- Investigation lifecycle (pending → complete → failed)
- SLA breach rate

## OpenTelemetry Tracing

Distributed tracing is implemented but **off by default** — with no tracing configuration Alga installs a no-op tracer provider, so no spans are created or exported and the overhead is effectively zero. Trace export is opt-in: set `ALGA_OTEL_ENABLED=true` or configure an OTLP endpoint (`OTEL_EXPORTER_OTLP_ENDPOINT`, or `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` for the traces signal specifically) to enable OTLP/HTTP export.

| Variable                             | Description                                                                               |
| ------------------------------------ | ----------------------------------------------------------------------------------------- |
| `ALGA_OTEL_ENABLED`                  | Set to `true` to enable trace export                                                      |
| `OTEL_EXPORTER_OTLP_ENDPOINT`        | OTLP collector endpoint (a gRPC `:4317` endpoint is rewritten to `:4318` for HTTP export) |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | Per-signal override for the traces endpoint                                               |
| `ALGA_OTEL_SAMPLE_RATIO`             | Sampling ratio for the ParentBased(TraceIDRatioBased) sampler (0.0–1.0)                   |

### Cross-Broker Trace Propagation

Trace context propagates across the RabbitMQ broker boundary using W3C `traceparent`/`tracestate` headers injected into AMQP message headers (`rabbitmq/trace_carrier.go`). This means a trace started at the webhook or API layer continues through correlation, scheduling, and investigation workers as a single distributed trace.

## Request ID Correlation

Every HTTP request passes through a request ID middleware that:

- Reads an incoming `X-Request-ID` header (or generates one if absent)
- Echoes it back in the response `X-Request-ID` header
- Attaches it to the structured logger context so all log lines for that request share the same ID

Use the request ID to correlate client-side errors with backend log entries.

## Logging

### Configuration

```sh
LOG_LEVEL=info                      # debug, info, warn, error, fatal
LOG_FORMAT=json                     # text (default) or json
LOG_FILE=/var/log/alga/app.log      # Optional file output
```

The Docker Compose deployment sets `LOG_FORMAT=json` for structured log ingestion.

### Log Levels

| Level   | Use Case                     |
| ------- | ---------------------------- |
| `debug` | Development, troubleshooting |
| `info`  | Normal operations (default)  |
| `warn`  | Recoverable issues           |
| `error` | Failures requiring attention |
| `fatal` | Unrecoverable, process exits |

### Viewing Logs

```sh
# Docker Compose
docker compose logs -f backend

# Direct
LOG_LEVEL=debug ./alga
```

The log level can also be changed at runtime without a restart via the [System Configuration API](/configuration/system-config) (`PUT /api/v1/system/config` with `log_level`).

## Alerting on Alga Itself

Monitor these indicators to detect Alga issues:

| Indicator                                     | Threshold        | Alert                     |
| --------------------------------------------- | ---------------- | ------------------------- |
| `alga_scheduler_bind_failed_total` increasing | > 0 sustained    | Agent dispatch failing    |
| `alga_correlator_dropped_total` increasing    | > 0              | Alerts being dropped      |
| Readiness endpoint returns errors             | Any              | Backend unhealthy         |
| `alga_scheduler_pending` growing              | Sustained growth | Investigations backing up |
| `alga_scheduler_no_candidate_total` growing   | > 0              | No agents online          |

## See Also

- [Architecture](/operations/architecture) — system design and data flow
- [Performance & Scaling](/operations/performance) — optimization
- [Backup & Restore](/operations/backup) — data protection
