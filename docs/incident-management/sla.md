---
title: SLA Tracking
description: SLA metrics — response, resolution, acknowledgement — with breach detection, compliance scoring, and priority-based targets.
---

# SLA Tracking

Alga tracks Service Level Agreement (SLA) compliance with automatic breach detection.

## How SLA Works

When an incident is created, Alga computes two deadlines based on severity or service configuration:

- **`sla_target_respond_at`** — Time by which the incident must be acknowledged
- **`sla_target_resolve_at`** — Time by which the incident must be resolved

These deadlines are tracked in Valkey sorted sets for efficient sweep operations.

## SLA Metrics

| Metric | Abbreviation | Description |
|--------|-------------|-------------|
| Mean Time To Acknowledge | MTTA | Time from creation to first acknowledgement |
| Mean Time To Resolve | MTTR | Time from creation to resolution |
| Mean Time To Mitigate | MTTM | Time from creation to mitigation |
| SLA Compliance | — | Percentage of incidents meeting response and resolution targets |

## Configuration

SLA targets are computed from:
1. **Service-level configuration** — custom response/resolution targets per service
2. **Severity defaults** — fallback based on incident severity

### Valkey Sorted Sets

```
alga:sla:response   → {incident_id: deadline_timestamp}
alga:sla:resolve    → {incident_id: deadline_timestamp}
```

A periodic sweep reads entries past the current timestamp and fires breach events.

## Breach Detection

When an SLA deadline passes:

1. The sweep detects the breach
2. An `incident_sla_breach` audit event is logged
3. An escalation event is triggered (if escalation policy is configured)
4. A timeline entry is automatically created on the incident

## Metrics API

```sh
curl -b cookies.txt "http://localhost:8080/api/v1/incidents/metrics?start_date=2026-01-01&end_date=2026-05-10&group_by=service_id"
```

### Query Parameters

| Parameter | Description |
|-----------|-------------|
| `service_id` | Filter by service |
| `team_id` | Filter by team |
| `severity` | Filter by severity |
| `start_date` | Start of date range |
| `end_date` | End of date range |
| `group_by` | Group results by `service_id`, `team_id`, or `severity` |

### Response

```json
{
  "total_created": 42,
  "total_resolved": 38,
  "mtta_minutes": 8.5,
  "mttr_minutes": 127.3,
  "mttm_minutes": 45.2,
  "response_breaches": 3,
  "resolve_breaches": 5,
  "total_with_sla": 40,
  "sla_compliance": {
    "response_sla_compliance_pct": 92.5,
    "resolve_sla_compliance_pct": 87.5
  },
  "by_severity": {
    "critical": {"mtta_minutes": 3.2, "mttr_minutes": 89.1},
    "warning": {"mtta_minutes": 12.1, "mttr_minutes": 156.7}
  },
  "by_service": {
    "payment-service": {"mtta_minutes": 4.1, "mttr_minutes": 72.3, "total": 12, "breaches": 1},
    "auth-service": {"mtta_minutes": 6.8, "mttr_minutes": 95.7, "total": 8, "breaches": 2}
  },
  "trend": [
    {"period": "2026-01", "mtta_minutes": 9.2, "mttr_minutes": 145.0, "total": 8},
    {"period": "2026-02", "mtta_minutes": 8.5, "mttr_minutes": 132.1, "total": 10},
    {"period": "2026-03", "mtta_minutes": 7.8, "mttr_minutes": 121.4, "total": 9},
    {"period": "2026-04", "mtta_minutes": 7.1, "mttr_minutes": 115.8, "total": 7},
    {"period": "2026-05", "mtta_minutes": 6.5, "mttr_minutes": 108.2, "total": 8}
  ]
}
```

## See Also

- [Incident Lifecycle](/incident-management/lifecycle) — state transitions
- [Escalation Policies](/on-call/escalation-policies) — policy-driven escalation
- [Incident Overview](/incident-management/) — creation and management
