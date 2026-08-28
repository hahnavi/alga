---
title: Incident Coordination
description: Coordination messages, typed coordination tasks, public status updates, Slack incident channels, and how agents and operators collaborate during an incident.
---

# Incident Coordination

Alga provides structured communication channels for coordinating incident response: a coordination message stream, typed coordination tasks for agent work, public status updates, and optional Slack incident channels.

## Coordination Messages

The coordination stream is a real-time message feed within each incident (`IncidentCoordinationMessage`). Each message records an actor, a body, and metadata, and can be threaded and linked to investigations or tasks.

### Message Kinds

| Kind                    | Purpose                                                             |
| ----------------------- | ------------------------------------------------------------------- |
| `chat`                  | Default conversational message (the default when `kind` is omitted) |
| `system`                | System-generated message                                            |
| `decision`              | A key decision made during the incident                             |
| `action`                | An action taken or planned                                          |
| `agent_reply`           | A reply posted by an agent                                          |
| `investigation_summary` | A summarized investigation result                                   |
| `status_update`         | A public status update (see [Status Updates](#status-updates))      |

### Message Fields

| Field                                                       | Description                                           |
| ----------------------------------------------------------- | ----------------------------------------------------- |
| `kind`                                                      | Message kind (defaults to `chat`)                     |
| `actor_type` / `actor_id` / `actor_display_name`            | Who posted the message (`system`, `user`, or `agent`) |
| `body`                                                      | Message text (required)                               |
| `internal`                                                  | Internal-only flag (not for external stakeholders)    |
| `source`                                                    | Origin of the message (defaults to `alga`)            |
| `parent_message_id`                                         | Parent message for threading                          |
| `linked_investigation_id`                                   | Investigation this message relates to                 |
| `linked_coordination_task_id`                               | Coordination task this message relates to             |
| `slack_channel_id` / `slack_message_ts` / `slack_thread_ts` | Slack mirroring references                            |
| `provider_message_id`                                       | External provider message identifier                  |

### API Endpoints

| Method | Path                                           | Permission        | Description                |
| ------ | ---------------------------------------------- | ----------------- | -------------------------- |
| `GET`  | `/api/v1/incidents/{id}/coordination/messages` | `incidents:read`  | List coordination messages |
| `POST` | `/api/v1/incidents/{id}/coordination/messages` | `incidents:write` | Add coordination message   |

```sh
curl -X POST http://localhost:8080/api/v1/incidents/{id}/coordination/messages \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "decision",
    "body": "Decided to failover to us-east-2 region.",
    "internal": false
  }'
```

## Coordination Tasks

Coordination tasks (`CoordinationTask`) are typed, persisted work units that structure agent collaboration. A commander dispatches tasks; responder/communicator agents claim and complete them. Tasks form parent/child trees.

### Task Kinds

`investigate`, `communicate`, `verify`, `mitigate`, `synthesize` (defaults to `investigate`).

### Task Lifecycle

```
pending → assigned → in_progress → complete
                                 → failed
                                 → cancelled
```

| Status        | Meaning                        |
| ------------- | ------------------------------ |
| `pending`     | Created, not yet claimed       |
| `assigned`    | Claimed by an agent            |
| `in_progress` | Actively being worked          |
| `complete`    | Finished with a result         |
| `failed`      | Failed (with `failure_reason`) |
| `cancelled`   | Cancelled                      |

Transitions are validated centrally by the store (`CanTransitionCoordinationTask`);
illegal jumps — for example `pending → complete`, or any move out of a terminal
status — fail with a status-conflict error. Reverting an in-flight task back to
`pending` (agent disconnect, dispatch retry) and cancel/fail from any
non-terminal status are the supported side edges.

### Task Fields

| Field                                       | Description                           |
| ------------------------------------------- | ------------------------------------- |
| `kind`                                      | Task kind (defaults to `investigate`) |
| `assignee_role`                             | Target role (defaults to `responder`) |
| `assignee_agent_id` / `assignee_agent_name` | Assigned agent                        |
| `goal`                                      | What the task must achieve (required) |
| `input_context`                             | Structured input (JSON)               |
| `result` / `result_schema`                  | Structured output and its schema      |
| `linked_investigation_id`                   | Investigation spawned by this task    |
| `priority`                                  | Numeric priority                      |
| `due_at`                                    | Optional deadline                     |
| `dispatch_attempts`                         | Number of dispatch attempts           |
| `parent_task_id`                            | Parent task for sub-task trees        |

### API Endpoints

| Method  | Path                                        | Permission          | Description                                                   |
| ------- | ------------------------------------------- | ------------------- | ------------------------------------------------------------- |
| `GET`   | `/api/v1/incidents/{id}/coordination/tasks` | `incidents:read`    | List coordination tasks (filter by `status`, `assignee_role`) |
| `POST`  | `/api/v1/incidents/{id}/coordination/tasks` | `incidents:command` | Create a coordination task                                    |
| `PATCH` | `/api/v1/incidents/{id}/coordination/tasks` | `incidents:command` | Update a task's status (cancel)                               |

```sh
curl -X POST http://localhost:8080/api/v1/incidents/{id}/coordination/tasks \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "investigate",
    "assignee_role": "responder",
    "goal": "Determine whether the database connection pool is exhausted"
  }'
```

## Status Updates

Status updates are public coordination messages (kind `status_update`) that track the response phase for stakeholders. Each is recorded on the incident timeline.

### Status Levels

| Level           | Meaning                                      |
| --------------- | -------------------------------------------- |
| `investigating` | Team is actively looking into the issue      |
| `identified`    | Root cause has been found                    |
| `mitigated`     | Containment/fix is in place                  |
| `monitoring`    | Fix deployed, watching for recovery          |
| `resolved`      | Incident resolved, service confirmed healthy |

### API Endpoints

| Method | Path                                    | Permission          | Description                        |
| ------ | --------------------------------------- | ------------------- | ---------------------------------- |
| `GET`  | `/api/v1/incidents/{id}/status-updates` | `incidents:read`    | List status updates (newest first) |
| `POST` | `/api/v1/incidents/{id}/status-updates` | `incidents:command` | Create status update               |

```sh
curl -X POST http://localhost:8080/api/v1/incidents/{id}/status-updates \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "status_level": "identified",
    "body": "Root cause: connection pool leak in auth-service v2.3.1"
  }'
```

## Slack Incident Channels

Incidents can get a dedicated Slack channel for teams that primarily operate in Slack. When linked, coordination messages and status updates are posted to the channel, and replies are synced back to the coordination stream.

### API Endpoints

| Method   | Path                                   | Permission          | Description                    |
| -------- | -------------------------------------- | ------------------- | ------------------------------ |
| `POST`   | `/api/v1/incidents/{id}/slack-channel` | `incidents:command` | Create dedicated Slack channel |
| `DELETE` | `/api/v1/incidents/{id}/slack-channel` | `incidents:command` | Delete/unlink Slack channel    |

### Configuration

These system-config keys control incident channel behavior:

| Key                                       | Description                                                             | Default   |
| ----------------------------------------- | ----------------------------------------------------------------------- | --------- |
| `slack_incident_channels_enabled`         | Enable per-incident Slack channels                                      | `false`   |
| `slack_incident_channel_visibility`       | Channel visibility (`public` or `private`)                              | `private` |
| `slack_incident_channel_trigger_status`   | Incident status that triggers channel creation (`active` or `detected`) | `active`  |
| `slack_incident_channel_archive_on_close` | Archive the channel when the incident closes                            | `true`    |

## Investigation Thread

Each incident also has a technical investigation thread separate from the coordination stream, used for agent findings, analysis, and technical discussion.

| Method | Path                                              | Permission        | Description              |
| ------ | ------------------------------------------------- | ----------------- | ------------------------ |
| `GET`  | `/api/v1/incidents/{incident_id}/thread`          | `incidents:read`  | Get investigation thread |
| `POST` | `/api/v1/incidents/{incident_id}/thread/messages` | `incidents:write` | Add message to thread    |

## See Also

- [Incident Lifecycle](/incident-management/lifecycle) — state machine and transitions
- [ICS Roles](/incident-management/ics-roles) — incident command system and role assignments
- [Incident Overview](/incident-management/) — creation, linking, and management
