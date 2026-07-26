---
title: Dashboard
description: Real-time operations dashboard with firing alerts, active incidents, investigation status, SLA compliance, alert trends, and a daily AI-generated summary.
---

# Dashboard

The Alga dashboard provides an at-a-glance view of your operations.

## Dashboard Stats

The dashboard shows aggregate counters:

| Stat                         | Description                          |
| ---------------------------- | ------------------------------------ |
| **Firing Alerts**            | Currently active (unresolved) alerts |
| **Active Incidents**         | Incidents in active/mitigated state  |
| **Pending Investigations**   | Investigations waiting for an agent  |
| **Unacknowledged Incidents** | Incidents pending acknowledgement    |

## API Endpoint

```sh
curl -b cookies.txt http://localhost:8080/api/v1/dashboard/stats
```

Response:

```json
{
  "alerts": { "total": 42, "firing": 12, "resolved": 28, "unacknowledged": 3 },
  "alerts_by_severity": [...],
  "alert_trend": [...],
  "investigations": { "total": 15, "pending": 2, "investigating": 1, "complete": 10, "failed": 1, "cancelled": 0, "timed_out": 1, "completion_rate": 0.67 },
  "top_alerts_24h": [...],
  "recent_investigations": [...],
  "active_investigations": [...],
  "incidents": { "total": 5, "active": 3, "mitigated": 1, "resolved": 1, "by_severity": {...} },
  "active_incidents": [...],
  "services": { "total": 8, "by_status": {...} },
  "sla_stats": { "response_breaches": 1, "resolve_breaches": 0, "compliance_pct": 0.95 }
}
```

## Real-Time Updates

Dashboard stats update automatically via SSE. No page refresh needed — counters update as events occur.

## Charts and Trends

The dashboard includes charts for:

- Alert trend — line chart showing created vs resolved alerts over time
- Alerts by severity — doughnut chart
- Investigation completion rate — displayed as a number
- SLA compliance over time

These use Chart.js (`vue-chartjs`) for rendering.

## Daily Summary

The daily summary provides an LLM-generated Markdown digest of alert and incident activity over the past 24 hours.

### API Endpoint

| Method | Path                              | Auth    | Description                        |
| ------ | --------------------------------- | ------- | ---------------------------------- |
| `GET`  | `/api/v1/dashboard/daily-summary` | Session | Get daily summary report           |
| `POST` | `/api/v1/dashboard/daily-summary` | Session | Regenerate daily summary on demand |

```sh
curl -b cookies.txt http://localhost:8080/api/v1/dashboard/daily-summary
```

Response:

```json
{
  "summary": "# Daily Summary\n\n## Alerts\n...\n## Incidents\n...",
  "generated_at": "2026-05-11T08:00:00Z",
  "period": "24h",
  "available": true
}
```
