---
title: Service Catalog
description: Service catalog with status tracking, priority-weighted scoring, dependency graphs, and automatic status propagation on incident transitions.
---

# Services

Alga includes a service catalog that tracks service status, dependencies, and incident impact.

## Service Status

Service status is **computed** using a **weighted priority-based scoring system**. Active incidents linked to the service are weighted by severity priority:

| Priority | Weight |
|----------|--------|
| P1 | 5 |
| P2 | 4 |
| P3 | 3 |
| P4 | 2 |
| P5 | 1 |

The total score determines the service status:

| Score | Status | Description |
|-------|--------|-------------|
| 0 | `operational` | Fully operational, no active incidents |
| 1–4 | `degraded` | Partial degradation, some features impacted |
| 5–9 | `partial_outage` | Significant partial outage |
| 10+ | `major_outage` | Complete service outage |

For example, a single P1 incident (score 5) results in `partial_outage`, while two P1 incidents (score 10) trigger `major_outage`.

Status is recalculated automatically as incidents are created, mitigated, or resolved. When all incidents are resolved, the service returns to `operational`.

## Service Fields

| Field | Description |
|-------|-------------|
| `name` | Service name |
| `description` | Service description |
| `owner_team_id` | Team responsible for the service |
| `escalation_policy_id` | Escalation policy for incidents on this service |
| `label_matchers` | Labels for matching alerts to this service |
| `sla_response_minutes` | Custom SLA response target in minutes |
| `sla_resolve_minutes` | Custom SLA resolution target in minutes |

## Dependencies

Services can declare dependencies on other services. When a service has an incident:
- Dependent services automatically get timeline entries noting the dependency impact
- Dependency cascade helps teams understand upstream/downstream impact

### Dependency Cascade

Dependency propagation uses **BFS (breadth-first search) traversal** of the service dependency graph:

1. When an incident transitions to a non-terminal state, `PropagateAndCascade()` is called
2. The function traverses all services that depend on the affected service (BFS)
3. For each dependent service, an automatic timeline entry is created noting the upstream impact
4. The cascade continues transitively — if A depends on B and B depends on C, a C incident cascades to both B and A

This ensures teams responsible for downstream services are immediately aware of upstream degradation without manual notification.

## API Endpoints

### Service Management
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/services` | Session | `services:read` | List services |
| `POST` | `/api/v1/services` | Session | `services:write` | Create service |
| `GET` | `/api/v1/services/{id}` | Session | `services:read` | Get service (includes status, dependencies) |
| `PATCH` | `/api/v1/services/{id}` | Session | `services:write` | Update service |
| `DELETE` | `/api/v1/services/{id}` | Session | `services:write` | Delete service |

### Dependencies
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/services/{id}/dependencies` | Session | `services:read` | Get dependency graph |
| `POST` | `/api/v1/services/{id}/dependencies` | Session | `services:write` | Add dependency |
| `DELETE` | `/api/v1/services/{id}/dependencies/{targetId}` | Session | `services:write` | Remove dependency |

### Incidents
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/services/{id}/incidents` | Session | `services:read` | List incidents affecting this service |

## Agent API

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/agent/services` | Bearer | List services |
