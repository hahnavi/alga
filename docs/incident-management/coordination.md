---
title: Incident Coordination
description: Coordination messages, public status updates, Slack incident channels, and how agents and operators collaborate during an incident.
---

# Incident Coordination

Alga provides structured communication channels for coordinating incident response: a coordination message stream, public status updates, and optional Slack incident channels.

## Coordination Messages

The coordination stream is a real-time message feed within each incident (`IncidentCoordinationMessage`). Each message records an actor, a body, and metadata, and can be threaded and linked to investigations. Mentioning an agent with `[@name](agent:…)` in a message activates that agent; a message that is empty after stripping mention links is stored but does not activate anyone.

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
