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
| `GET /debug/vars`       | expvar metrics (secondary)                        | None                   |

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

All metrics are exposed in Prometheus format at `/metrics` (the primary scrape target for Prometheus). The same metrics are also available in expvar format at `/debug/vars` as a secondary endpoint. Both endpoints are unauthenticated — restrict access at the network level.

### Correlator Metrics

| Metric                                 | Description                                |
| -------------------------------------- | ------------------------------------------ |
| `alga_correlator_alerts_total`         | Total alerts received                      |
| `alga_correlator_alerts_merged_total`  | Alerts merged into existing windows        |
| `alga_correlator_published_total`      | Investigations published after correlation |
| `alga_correlator_dropped_total`        | Alerts dropped                             |
| `alga_correlator_fail_closed_total`    | Fail-closed events                         |
| `alga_correlator_windows_opened_total` | New correlation windows opened             |
| `alga_correlator_flushes_total`        | Windows flushed (expired)                  |

### Scheduler Metrics

| Metric                                        | Description                            |
| --------------------------------------------- | -------------------------------------- |
| `alga_scheduler_pending`                      | Current pending investigation depth    |
| `alga_scheduler_online_agents`                | Agents currently online (all replicas) |
| `alga_scheduler_agent_capacity_used`          | Aggregate used capacity                |
| `alga_scheduler_agent_capacity_total`         | Aggregate total capacity               |
| `alga_scheduler_scheduled_total`              | Successful investigation binds         |
| `alga_scheduler_no_candidate_total`           | Pending with no eligible agent         |
| `alga_scheduler_bind_failed_total`            | Atomic claim or forward failures       |
| `alga_scheduler_skip_backoff_total`           | Skipped due to failure backoff         |
| `alga_scheduler_tick_total`                   | Total scheduler ticks                  |
| `alga_scheduler_tick_duration_ms`             | Last tick duration                     |
| `alga_scheduler_is_leader`                    | 1 on leader replica, 0 elsewhere       |
| `alga_scheduler_stale_alerts_swept`           | Uninvestigated alerts found per sweep  |
| `alga_scheduler_stale_investigations_created` | Investigations from stale alerts       |
| `alga_scheduler_stale_sweep_tick_total`       | Total stale sweep ticks                |
| `alga_scheduler_nudge_total`                  | Scheduler nudge events                 |

### Webhook Metrics

| Metric                                            | Description                                |
| ------------------------------------------------- | ------------------------------------------ |
| `alga_webhook_alert_publish_queued_total`         | Webhook alerts queued for publishing       |
| `alga_webhook_alert_publish_sync_fallback_total`  | Webhook alerts published via sync fallback |
| `alga_webhook_alert_publish_sync_processed_total` | Webhook alerts processed synchronously     |

### Escalation Metrics

| Metric                           | Description                  |
| -------------------------------- | ---------------------------- |
| `alga_escalations_fired_total`   | Total escalations fired      |
| `alga_sla_breach_response_total` | SLA response time breaches   |
| `alga_sla_breach_resolve_total`  | SLA resolution time breaches |

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

### Triage Metrics

| Metric                            | Description                        |
| --------------------------------- | ---------------------------------- |
| `alga_triage_total`               | Total triage evaluations           |
| `alga_triage_decision_*`          | Per-decision outcome counters      |
| `alga_triage_confirmed_total`     | Triage decisions confirmed         |
| `alga_triage_overridden_total`    | Triage decisions overridden        |
| `alga_triage_short_circuit_total` | Triage evaluations short-circuited |
| `alga_triage_llm_total`           | LLM-based triage evaluations       |
| `alga_triage_llm_duration_ms`     | LLM triage duration                |

### Playbook Metrics

| Metric                                 | Description                         |
| -------------------------------------- | ----------------------------------- |
| `alga_playbooks_matched_total`         | Playbooks matched to investigations |
| `alga_playbooks_steps_completed_total` | Playbook steps completed            |

### ICS Metrics

| Metric                          | Description           |
| ------------------------------- | --------------------- |
| `alga_ics_roles_assigned_total` | ICS roles assigned    |
| `alga_ics_handoffs_total`       | IC handoffs performed |

### Handoff Metrics

| Metric                                    | Description            |
| ----------------------------------------- | ---------------------- |
| `alga_oncall_handoffs_total`              | On-call handoffs       |
| `alga_oncall_handoffs_acknowledged_total` | Handoffs acknowledged  |
| `alga_oncall_reminders_sent_total`        | On-call reminders sent |

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

Alga exports distributed traces via OTLP/HTTP when tracing is enabled. Tracing activates when `ALGA_OTEL_ENABLED=true` or when an OTLP endpoint is configured.

| Variable                      | Description                                                                               |
| ----------------------------- | ----------------------------------------------------------------------------------------- |
| `ALGA_OTEL_ENABLED`           | Set to `true` to enable trace export                                                      |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector endpoint (a gRPC `:4317` endpoint is rewritten to `:4318` for HTTP export) |
| `ALGA_OTEL_SAMPLE_RATIO`      | Sampling ratio for the ParentBased(TraceIDRatioBased) sampler (0.0–1.0)                   |

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
