---
name: alga-domain-invariants
description: Use when changing Alga alert, incident, investigation, scheduler, RabbitMQ, Valkey, escalation, SLA, notification, or lifecycle behavior.
priority: P0
tags: [domain, invariants, alerts, incidents, investigations, scheduler, rabbitmq, valkey]
---

# Alga Domain Invariants

Use this before designing or reviewing behavior in the core incident-response domain. If a requested change conflicts with an invariant, stop and make the tradeoff explicit.

## Alerts

- Alert fingerprints are deduplication keys, not unique IDs.
- `alert_number` is the true unique alert identifier.
- Resolved alerts are never auto-reopened.
- A new firing alert with a resolved fingerprint creates a new alert record.
- Manual reopen reopens the latest resolved alert and cascades to linked investigations.
- PostgreSQL enforces at most one open alert per fingerprint with a partial unique index.

## Incidents and Investigations

- Incidents sit above investigations.
- Incident lifecycle follows `detected -> triaging -> active -> mitigated -> resolved -> closed`, with cancelled/reopened branches.
- State transitions must persist, audit, and preserve linked investigation behavior.
- The investigation scheduler binds pending investigations atomically to online agents to avoid double assignment.

## Async and Realtime

- RabbitMQ powers async alert processing, notifications, audits, investigations, incidents, escalation, SLA sweeps, and retry/dead-letter flows.
- SSE broadcasts are realtime notifications only; they never replace persistence or audit.
- Valkey is preferred for sessions, refresh rotation, rate limiting, SSE/agent presence, scheduler leader election, dedupe, locks, pub/sub, SLA sorted sets, escalation state, and on-call caches.
- PostgreSQL fallback exists for sessions when Valkey is unavailable.
- SSE broker fans realtime events to the frontend; Valkey pub/sub supports cross-replica fan-out.

## Review Questions

- Does the change preserve the unique identity model for alerts?
- Does it accidentally reopen resolved data?
- Does it keep persistence, audit, and realtime notification responsibilities separate?
- Does async processing remain idempotent and retry-safe?
- Does scheduler or worker logic avoid double assignment and duplicate side effects?
- Does Valkey failure have the same fallback behavior as nearby code?
