---
title: Status Pages
description: Public and internal status pages that surface service health to viewers, composed of components linked to your service catalog and active incidents.
---

# Status Pages

Status pages surface the health of your services to viewers — either internally (operators) or publicly (customers/stakeholders). Each page is composed of **components** (optionally linked to services in your [catalog](/service-management/)) and automatically shows active incidents affecting them.

## Why Status Pages?

When an incident is active, stakeholders want answers: *What's affected? How bad is it? When will it be fixed?* A status page gives them a single URL to check, so your team can focus on resolving the incident instead of answering "is it down?" questions in chat.

## Concepts

- **Status Page** — a named page with a unique slug, optional description, visibility setting, an `enabled` toggle, and an owning team
- **Component** — a trackable item on the page (e.g., "API", "Web Dashboard", "Database"). Each component has its own status and can optionally reference a [service](/service-management/) via `service_id`
- **Overall status** — derived automatically as the **worst** component status on the page

## Component Statuses

Components use these statuses, ordered from least to most severe:

| Status | Rank | Meaning |
|--------|------|---------|
| `operational` | 0 | Everything is working normally |
| `maintenance` | 1 | Scheduled maintenance in progress |
| `degraded` | 2 | Some functionality is impaired but available |
| `partial_outage` | 3 | A subset of the component is down |
| `major_outage` | 4 | The component is completely down |

The page-level overall status is the highest-ranked (worst) component status. This means if even one component is at `major_outage`, the entire page shows `major_outage`.

::: tip Component status is manual (for now)
Component statuses are set explicitly via the API or UI. Linking a component to a service (`service_id`) creates a reference for operators — it doesn't automatically propagate the service's status to the component yet.
:::

## Visibility

Each page carries a `visibility` of `internal` (default) or `public`. This marks the page's intended audience:

| Visibility | Intended Audience | Use Case |
|------------|-------------------|----------|
| `internal` | Operators | Internal operations dashboard, team-awareness pages |
| `public` | Customers / stakeholders | Customer-facing status page, stakeholder updates |

::: warning Public access is not yet unauthenticated
All status page API routes currently require authentication (a session or personal access token) and the `statuspages:read` permission. The `public` visibility is a classification marker; Alga does not yet serve an unauthenticated public endpoint for `public` pages.
:::

## Creating a Status Page

1. Go to **Status Pages → Create Status Page**
2. Set the **name** and **slug** (the URL path — e.g., `status.example.com/pages/prod-status`)
3. Choose **visibility** (`internal` or `public`)
4. Assign an **owning team**
5. **Add components**:
   - Name each component (e.g., "API", "Web Dashboard")
   - Optionally link to a service via `service_id`
   - Set the initial status (defaults to `operational`)
6. Save

### Slug Rules

Slugs must be 2–64 characters: lowercase letters, digits, and hyphens (no leading/trailing hyphen). Regex: `^[a-z0-9][a-z0-9-]{0,62}[a-z0-9]$`. Slugs are unique across all status pages.

## Updating Component Status During an Incident

During an active incident, update the affected components to reflect reality:

```sh
curl -b cookies.txt -X PATCH http://localhost:8080/api/v1/status-pages/1/components/1 \
  -H "Content-Type: application/json" \
  -d '{"status": "degraded"}'
```

When the incident is resolved, set components back to `operational`.

## API

### Status Pages

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/status-pages` | `statuspages:read` | List status pages |
| `POST` | `/api/v1/status-pages` | `statuspages:write` | Create status page |
| `GET` | `/api/v1/status-pages/slug/{slug}` | `statuspages:read` | View page by slug (with overall status, components, active incidents) |
| `GET` / `PATCH` / `DELETE` | `/api/v1/status-pages/{id}` | `statuspages:read` / `write` | Manage a page |

### Components

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/status-pages/{id}/components` | `statuspages:read` | List components |
| `POST` | `/api/v1/status-pages/{id}/components` | `statuspages:write` | Create component |
| `GET` / `PATCH` / `DELETE` | `/api/v1/status-pages/{id}/components/{component_id}` | `statuspages:*` | Manage a component |

## See Also

- [Service Catalog](/service-management/) — link components to services
- [Incident Management](/incident-management/) — incidents drive status page updates
- [Teams](/on-call/) — team ownership of status pages
