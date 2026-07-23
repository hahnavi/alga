---
title: Post-Mortems
description: Post-incident reviews with structured content sections, action items with owners and due dates, and workflow from draft through publication.
---

# Post-Mortems

Alga provides a structured post-mortem workflow for incidents, with review/approval processes and trackable action items.

## Post-Mortem Lifecycle

```
draft → in_review → approved → published
```

| Status | Description |
|--------|-------------|
| `draft` | Initial creation, being written |
| `in_review` | Submitted for review |
| `approved` | Reviewed and approved |
| `published` | Publicly visible, incident can be closed |

## Content Sections

A post-mortem includes the following content fields:
- **Summary** — Brief description of what happened
- **Timeline** — Key events during the incident
- **Root Cause** — Technical explanation
- **Contributing Factors** — Additional factors that contributed
- **Impact** — Duration, affected services, user impact
- **What Went Well** — Things that worked during the response
- **What Went Wrong** — Things that did not work or could be improved
- **Lessons Learned** — What was learned and what could be improved
- **Blameless Confirmation** — Acknowledgement that the review is blameless and focused on systemic improvement

## Action Items

Action items are tracked separately and can be:
- Linked to specific post-mortems
- Assigned to users
- Marked as complete

Each action item includes:
- **Type** — one of `prevent`, `mitigate`, `detect`, or `investigate`
- **Priority** — priority level
- **Assignee** — user responsible (`assignee_id`)
- **Due Date** — target completion date
- **Status** — current status

View all open action items globally at `GET /api/v1/action-items`.

## Workflow

1. **Create** post-mortem after incident is resolved
2. **Edit** content as needed (while in `draft`)
3. **Submit for Review** — moves to `in_review`
4. **Approve** — moves to `approved`
5. **Publish** — moves to `published`, incident can be closed
6. **Track Action Items** — ensure follow-up tasks are completed

## API Endpoints

### Post-Mortem Management
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/post-mortems` | Session | `postmortems:read` | List all post-mortems |
| `GET` | `/api/v1/incidents/{id}/post-mortem` | Session | `postmortems:read` | Get post-mortem |
| `POST` | `/api/v1/incidents/{id}/post-mortem` | Session | `postmortems:write` | Create post-mortem |
| `PATCH` | `/api/v1/incidents/{id}/post-mortem` | Session | `postmortems:write` | Update post-mortem |
| `DELETE` | `/api/v1/incidents/{id}/post-mortem` | Session | `postmortems:delete` | Delete post-mortem |

### Workflow Actions
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `POST` | `/api/v1/incidents/{id}/post-mortem/submit-review` | Session | `postmortems:write` | Submit for review |
| `POST` | `/api/v1/incidents/{id}/post-mortem/approve` | Session | `postmortems:write` | Approve post-mortem |
| `POST` | `/api/v1/incidents/{id}/post-mortem/publish` | Session | `postmortems:write` | Publish post-mortem |

### Action Items
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/post-mortem/action-items` | Session | `postmortems:read` | List action items |
| `POST` | `/api/v1/incidents/{id}/post-mortem/action-items` | Session | `postmortems:write` | Create action item |
| `PATCH` | `/api/v1/post-mortem/action-items/{id}` | Session | `postmortems:write` | Update action item |
| `DELETE` | `/api/v1/post-mortem/action-items/{id}` | Session | `postmortems:delete` | Delete action item |

### Global Action Items
| Method | Path | Auth | Permission | Description |
|--------|------|------|------------|-------------|
| `GET` | `/api/v1/action-items` | Session | `postmortems:read` | All open action items |
