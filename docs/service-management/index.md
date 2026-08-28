---
title: Service Catalog
description: Service catalog with status tracking, priority-weighted scoring, dependency graphs, and automatic status propagation on incident transitions.
---

# Services

Alga includes a service catalog that tracks service status, dependencies, and incident impact.

## Service Status

Service status is **computed** using a **weighted priority-based scoring system**. Active incidents linked to the service are weighted by severity priority:

| Priority | Weight |
| -------- | ------ |
| P1       | 5      |
| P2       | 4      |
| P3       | 3      |
| P4       | 2      |
| P5       | 1      |

The total score determines the service status:

| Score | Status           | Description                                 |
| ----- | ---------------- | ------------------------------------------- |
| 0     | `operational`    | Fully operational, no active incidents      |
| 1–4   | `degraded`       | Partial degradation, some features impacted |
| 5–9   | `partial_outage` | Significant partial outage                  |
| 10+   | `major_outage`   | Complete service outage                     |

For example, a single P1 incident (score 5) results in `partial_outage`, while two P1 incidents (score 10) trigger `major_outage`.

Status is recalculated automatically as incidents are created, mitigated, or resolved. When all incidents are resolved, the service returns to `operational`.

## Service Fields

| Field                  | Description                                                                                          |
| ---------------------- | ---------------------------------------------------------------------------------------------------- |
| `name`                 | Service name (unique)                                                                                |
| `display_name`         | Human-friendly display name                                                                          |
| `description`          | Service description                                                                                  |
| `owner_team_id`        | Team responsible for the service                                                                     |
| `escalation_policy_id` | Escalation policy for incidents on this service                                                      |
| `label_matchers`       | Stored metadata only — reserved for future auto-matching; NOT evaluated during alert ingestion today |
| `sla_response_minutes` | Custom SLA response target in minutes                                                                |
| `sla_resolve_minutes`  | Custom SLA resolution target in minutes                                                              |
| `status`               | Current computed status (`operational`, `degraded`, `partial_outage`, `major_outage`)                |

## Dependencies

Services can declare dependencies on other services. A dependency is stored as `service_id` → `dependent_on_service_id` with a `dependency_type` (default `depends_on`), forming a directed dependency graph.

### Dependency Cascade

When an incident transitions state, the affected service's status is recomputed and the change cascades through the graph using **BFS (breadth-first search)**:

1. `PropagateAndCascade()` starts at the affected service
2. It recomputes the weighted status for the current service and persists any change
3. It then enqueues every service that depends on the current service (`GetDependents`)
4. Each dependent recomputes its status **from its own linked incidents only** and the walk continues transitively

> **No inherited impact today:** a downstream service's status only changes if an incident is linked to _that_ service (via its `service_id`). The cascade re-evaluates dependents but does not roll upstream impact into their scores — an incident on C does not degrade B or A merely because B depends on C. Link incidents to each affected service explicitly to reflect impact there.

Alert→service linkage is **manual**: attach `service_id` to an incident (at creation or afterward). Service `label_matchers` are stored metadata and are not evaluated during alert ingestion or correlation.

Each status change publishes a `service_status_changed` SSE event so the UI updates live.

## API Endpoints

### Service Management

| Method   | Path                    | Auth    | Permission       | Description                                 |
| -------- | ----------------------- | ------- | ---------------- | ------------------------------------------- |
| `GET`    | `/api/v1/services`      | Session | `services:read`  | List services                               |
| `POST`   | `/api/v1/services`      | Session | `services:write` | Create service                              |
| `GET`    | `/api/v1/services/{id}` | Session | `services:read`  | Get service (includes status, dependencies) |
| `PATCH`  | `/api/v1/services/{id}` | Session | `services:write` | Update service                              |
| `DELETE` | `/api/v1/services/{id}` | Session | `services:write` | Delete service                              |

### Dependencies

| Method   | Path                                            | Auth    | Permission       | Description                               |
| -------- | ----------------------------------------------- | ------- | ---------------- | ----------------------------------------- |
| `GET`    | `/api/v1/services/{id}/dependencies`            | Session | `services:read`  | Get dependency graph                      |
| `POST`   | `/api/v1/services/{id}/dependencies`            | Session | `services:write` | Add dependency                            |
| `DELETE` | `/api/v1/services/{id}/dependencies/{targetId}` | Session | `services:write` | Remove dependency                         |
| `GET`    | `/api/v1/services/{id}/dependents`              | Session | `services:read`  | List services that depend on this service |

### Incidents

| Method | Path                              | Auth    | Permission       | Description                           |
| ------ | --------------------------------- | ------- | ---------------- | ------------------------------------- |
| `GET`  | `/api/v1/services/{id}/incidents` | Session | `incidents:read` | List incidents affecting this service |

## Agent API

| Method | Path                     | Auth   | Description   |
| ------ | ------------------------ | ------ | ------------- |
| `GET`  | `/api/v1/agent/services` | Bearer | List services |
