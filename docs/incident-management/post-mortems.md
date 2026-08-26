---
title: Post-Mortems
description: Blameless post-incident reviews with structured content sections, an AI draft builder, action items with owners and due dates, and a draft-to-published workflow.
---

# Post-Mortems

Alga provides a structured, blameless post-mortem workflow for incidents, with review/approval processes and trackable action items. Each post-mortem is linked 1:1 to an incident (`incident_id` is unique).

## Post-Mortem Lifecycle

```
draft → in_review → approved → published
```

| Status      | Description                                      |
| ----------- | ------------------------------------------------ |
| `draft`     | Initial creation, being written                  |
| `in_review` | Submitted for review                             |
| `approved`  | Reviewed and approved (records `approved_by_id`) |
| `published` | Publicly visible (records `published_at`)        |

Allowed transitions: `draft → in_review`; `in_review → draft` or `approved`; `approved → in_review` or `published`. Submitting for review notifies the incident commander that a review is requested.

## Content Fields

A post-mortem includes the following fields:

| Field                  | Description                                      |
| ---------------------- | ------------------------------------------------ |
| `title`                | Post-mortem title                                |
| `summary`              | Brief description of what happened               |
| `timeline`             | Key events during the incident (structured JSON) |
| `root_cause`           | Technical explanation                            |
| `contributing_factors` | Additional factors that contributed (list)       |
| `impact`               | Duration, affected services, user impact         |
| `what_went_well`       | Things that worked during the response           |
| `what_went_wrong`      | Things that did not work or could be improved    |
| `lessons_learned`      | What was learned and what could be improved      |
| `blameless_confirmed`  | Acknowledgement that the review is blameless     |
| `blameless_notes`      | Notes supporting the blameless confirmation      |

## AI Draft Builder

When an incident is resolved, Alga can auto-create a post-mortem draft pre-populated from incident context — the incident document sections, coordination messages, timeline entries, status updates, and linked alerts. The same enrichment logic backs both the operator resolve flow and the agent resolve tool.

## Action Items

Action items are tracked per post-mortem and can be assigned, prioritized, and driven to completion.

| Field                           | Description                                               |
| ------------------------------- | --------------------------------------------------------- |
| `description`                   | What needs to be done (required)                          |
| `type`                          | Kind of follow-up (defaults to `investigate`)             |
| `assignee_id` / `assignee_name` | User responsible                                          |
| `status`                        | `open` → `in_progress` → `completed` (defaults to `open`) |
| `priority`                      | Priority level (defaults to `medium`)                     |
| `due_date`                      | Target completion date                                    |

View all open action items globally at `GET /api/v1/action-items`.

## Workflow

1. **Create** the post-mortem (auto-drafted on resolve, or created manually)
2. **Edit** content as needed (while in `draft`)
3. **Submit for Review** — moves to `in_review` and notifies the commander
4. **Approve** — moves to `approved` and records the approver
5. **Publish** — moves to `published`
6. **Track Action Items** — ensure follow-up tasks are completed

## API Endpoints

### Post-Mortem Management

| Method   | Path                                 | Auth    | Permission           | Description           |
| -------- | ------------------------------------ | ------- | -------------------- | --------------------- |
| `GET`    | `/api/v1/post-mortems`               | Session | `postmortems:read`   | List all post-mortems |
| `GET`    | `/api/v1/incidents/{id}/post-mortem` | Session | `postmortems:read`   | Get post-mortem       |
| `POST`   | `/api/v1/incidents/{id}/post-mortem` | Session | `postmortems:write`  | Create post-mortem    |
| `PATCH`  | `/api/v1/incidents/{id}/post-mortem` | Session | `postmortems:write`  | Update post-mortem    |
| `DELETE` | `/api/v1/incidents/{id}/post-mortem` | Session | `postmortems:delete` | Delete post-mortem    |

### Workflow Actions

| Method | Path                                               | Auth    | Permission          | Description                     |
| ------ | -------------------------------------------------- | ------- | ------------------- | ------------------------------- |
| `POST` | `/api/v1/incidents/{id}/post-mortem/submit-review` | Session | `postmortems:write` | Submit for review (`in_review`) |
| `POST` | `/api/v1/incidents/{id}/post-mortem/approve`       | Session | `postmortems:write` | Approve post-mortem             |
| `POST` | `/api/v1/incidents/{id}/post-mortem/publish`       | Session | `postmortems:write` | Publish post-mortem             |

### Action Items

| Method   | Path                                                       | Auth    | Permission           | Description        |
| -------- | ---------------------------------------------------------- | ------- | -------------------- | ------------------ |
| `GET`    | `/api/v1/incidents/{id}/post-mortem/action-items`          | Session | `postmortems:read`   | List action items  |
| `POST`   | `/api/v1/incidents/{id}/post-mortem/action-items`          | Session | `postmortems:write`  | Create action item |
| `PATCH`  | `/api/v1/incidents/{id}/post-mortem/action-items/{itemId}` | Session | `postmortems:write`  | Update action item |
| `DELETE` | `/api/v1/incidents/{id}/post-mortem/action-items/{itemId}` | Session | `postmortems:delete` | Delete action item |

### Global Action Items

| Method | Path                   | Auth    | Permission         | Description           |
| ------ | ---------------------- | ------- | ------------------ | --------------------- |
| `GET`  | `/api/v1/action-items` | Session | `postmortems:read` | All open action items |

## See Also

- [Incident Lifecycle](/incident-management/lifecycle) — state transitions and resolution requirements
- [Incident Overview](/incident-management/) — creation and management
