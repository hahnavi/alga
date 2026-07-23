---
title: Incident Coordination
description: Coordination streams, status updates, incident documents, investigation threads, and how agents and operators collaborate during an incident.
---

# Incident Coordination

Alga provides structured communication channels for coordinating incident response, including coordination streams, status updates, and incident documents.

## Coordination Stream

The coordination stream is a real-time message feed within each incident. It supports multiple message kinds for different communication needs:

| Kind | Purpose |
|------|---------|
| `update` | Status updates and progress reports |
| `decision` | Key decisions made during the incident |
| `action` | Actions taken or planned |
| `note` | Internal notes (not visible to external stakeholders) |

Messages support Markdown formatting and `@mentions` for users and agents. Mentioned users receive in-app notifications.

### API Endpoints

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/coordination` | `incidents:read` | List coordination messages |
| `POST` | `/api/v1/incidents/{id}/coordination` | `incidents:write` | Add coordination message |

```sh
curl -X POST http://localhost:8080/api/v1/incidents/{id}/coordination \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "decision",
    "message": "Decided to failover to us-east-2 region. @alice @bob acknowledged."
  }'
```

## Status Updates

Structured status updates track the incident response phase and are visible to all stakeholders:

| Status | Meaning |
|--------|---------|
| `investigating` | Team is actively looking into the issue |
| `identified` | Root cause has been found |
| `monitoring` | Fix deployed, watching for recovery |
| `resolved` | Incident resolved, service confirmed healthy |

Each status update is recorded on the incident timeline and can trigger notifications to stakeholders.

### API Endpoints

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/status-updates` | `incidents:read` | List status updates (newest first) |
| `POST` | `/api/v1/incidents/{id}/status-updates` | `incidents:command` | Create status update |

```sh
curl -X POST http://localhost:8080/api/v1/incidents/{id}/status-updates \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "status": "identified",
    "message": "Root cause: connection pool leak in auth-service v2.3.1"
  }'
```

## Incident Document

Each incident has an editable document with structured sections for note-taking during response:

| Section | Purpose |
|---------|---------|
| **Impact Assessment** | What is affected, severity, user impact, blast radius |
| **Actions Taken** | Chronological log of remediation steps and outcomes |

### API Endpoints

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/document` | `incidents:read` | Get document sections |
| `PATCH` | `/api/v1/incidents/{id}/document/{section}` | `incidents:write` | Update a section |

## Investigation Thread

Each incident has a technical investigation thread separate from the coordination stream. This thread is used for agent findings, analysis, and technical discussion.

### API Endpoints

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/thread` | `incidents:read` | Get investigation thread |
| `POST` | `/api/v1/incidents/{id}/thread/messages` | `incidents:write` | Add message to thread |

## Slack Integration

Incident coordination messages can be mirrored to a dedicated Slack channel for teams that primarily operate in Slack:

### API Endpoints

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `POST` | `/api/v1/incidents/{id}/slack-channel` | `incidents:command` | Create linked Slack channel |
| `DELETE` | `/api/v1/incidents/{id}/slack-channel` | `incidents:command` | Unlink Slack channel |

When linked, coordination messages and status updates are automatically posted to the Slack channel. Replies in Slack are synced back to the coordination stream.

## Triage Integration

Incidents can enter a triage phase before becoming active. This allows operators to assess severity and gather initial context before committing to a full response.

### API Endpoints

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `POST` | `/api/v1/incidents/{id}/begin-triage` | `incidents:command` | Begin triage |
| `POST` | `/api/v1/incidents/{id}/promote` | `incidents:command` | Promote triaging incident to active |

## See Also

- [Incident Lifecycle](/incident-management/lifecycle) — state machine and transitions
- [ICS Roles](/incident-management/ics-roles) — incident command system and role assignments
