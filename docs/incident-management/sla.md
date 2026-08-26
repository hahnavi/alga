---
title: SLA Tracking
description: SLA targets — response and resolution — with priority-based defaults, background breach detection, escalation on response breach, and aggregate metrics.
---

# SLA Tracking

Alga tracks Service Level Agreement (SLA) compliance with automatic breach detection and escalation.

## How SLA Works

When an incident is created, Alga computes two deadlines that are stored on the incident:

- **`sla_target_respond_at`** — Time by which the incident must be acknowledged
- **`sla_target_resolve_at`** — Time by which the incident must be resolved

These can be supplied explicitly at creation (`sla_target_respond_at` / `sla_target_resolve_at`). When omitted, they default from the incident priority via `PriorityToSLATargets`:

| Priority  | Respond Within | Resolve Within |
| --------- | -------------- | -------------- |
| P1        | 15 minutes     | 4 hours        |
| P2        | 30 minutes     | 8 hours        |
| P3        | 2 hours        | 24 hours       |
| P4        | 8 hours        | 72 hours       |
| P5        | 24 hours       | 168 hours      |
| (default) | 2 hours        | 24 hours       |

Priority is derived from severity and impact for alert-created incidents.

## Lifecycle Timestamps

SLA metrics are derived from lifecycle timestamps written by status transitions:

| Field                 | Set When                                        |
| --------------------- | ----------------------------------------------- |
| `sla_acknowledged_at` | Incident becomes `active` (acknowledge/promote) |
| `sla_resolved_at`     | Incident becomes `resolved`                     |
| `mitigated_at`        | Incident becomes `mitigated`                    |
| `resolved_at`         | Incident becomes `resolved`                     |

## Breach Detection

SLA sweep ticks are published by the **scheduler leader** on the `alga.sla` exchange every `SLA_SWEEP_INTERVAL` (default 60s); values ≤ 0 disable publication. The `SLAWorker` consumes these ticks and sweeps SLA-eligible incidents — those in `detected`, `triaging`, `active`, or `mitigated` status with a response or resolve target set. For each incident past a deadline:

1. **Response breach** — `sla_target_respond_at` passed and not yet acknowledged (`sla_acknowledged_at` is null):
   - A `sla_breach` timeline entry is added ("Response SLA breached")
   - An `incident_sla_breach` SSE event is published (and to the commander)
   - Escalation is triggered (level 1) if an escalation policy is configured
2. **Resolve breach** — `sla_target_resolve_at` passed and not yet resolved (`sla_resolved_at` is null):
   - A `sla_breach` timeline entry is added ("Resolve SLA breached")
   - An `incident_sla_breach` SSE event is published

Breaches are de-duplicated in Valkey (`alga:sla:breach:{incident}:{type}` with a 24h TTL) so each breach type fires once per incident. The worker also nudges the commander when public status updates go stale after responder activity.

## SLA Metrics

| Metric                   | Abbreviation | Description                                                     |
| ------------------------ | ------------ | --------------------------------------------------------------- |
| Mean Time To Acknowledge | MTTA         | Time from creation to acknowledgement                           |
| Mean Time To Resolve     | MTTR         | Time from creation to resolution                                |
| Mean Time To Mitigate    | MTTM         | Time from creation to mitigation                                |
| SLA Compliance           | —            | Percentage of incidents meeting response and resolution targets |

## Metrics API

```sh
curl -b cookies.txt "http://localhost:8080/api/v1/incidents/metrics?start_date=2026-01-01&end_date=2026-05-10"
```

### Query Parameters

| Parameter    | Description                                     |
| ------------ | ----------------------------------------------- |
| `start_date` | Start of date range (defaults to one month ago) |
| `end_date`   | End of date range (defaults to now)             |

### Response

```json
{
  "mtta_minutes": 8.5,
  "mttr_minutes": 127.3,
  "mttm_minutes": 45.2,
  "total_created": 42,
  "total_resolved": 38,
  "by_severity": {
    "critical": { "count": 12, "mtta_minutes": 3.2, "mttr_minutes": 89.1 },
    "warning": { "count": 30, "mtta_minutes": 12.1, "mttr_minutes": 156.7 }
  },
  "by_priority": {
    "P1": { "count": 8, "mtta_minutes": 4.1, "mttr_minutes": 72.3 }
  },
  "by_service": {
    "payment-service": { "count": 12, "mtta_minutes": 4.1, "mttr_minutes": 72.3 }
  },
  "sla_compliance": {
    "response_sla_compliance_pct": 92.5,
    "resolve_sla_compliance_pct": 87.5,
    "response_breaches": 3,
    "resolve_breaches": 5,
    "total_with_sla": 40
  },
  "trend": [
    {
      "date": "2026-05-01",
      "created": 3,
      "resolved": 2,
      "mtta_minutes": 6.5,
      "mttr_minutes": 108.2
    }
  ]
}
```

## See Also

- [Incident Lifecycle](/incident-management/lifecycle) — state transitions and timestamps
- [Escalation Policies](/on-call/escalation-policies) — policy-driven escalation
- [Incident Overview](/incident-management/) — creation and management
