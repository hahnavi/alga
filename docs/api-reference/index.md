---
title: API Reference
description: Complete REST API reference for Alga — every endpoint, method, path, authentication type, and RBAC permission across alerts, incidents, agents, teams, on-call, and more.
---

# API Reference

All endpoints are relative to `http://your-alga-host:8080`.

## Authentication Model

Alga supports four authentication mechanisms. Every route falls into exactly one category.

### Session Cookie + CSRF

Operator/UI endpoints require an HTTP-only session cookie (`alga_session`). State-changing methods (POST, PUT, PATCH, DELETE) must include the `X-CSRF-Token` header matching the `alga_csrf` cookie value.

```sh
# Login and save cookies
curl -c cookies.txt -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "admin@alga.local", "password": "your-password"}'

# Use session for subsequent requests
curl -b cookies.txt http://localhost:8080/api/v1/alerts

# State-changing request requires CSRF header
curl -b cookies.txt -X POST http://localhost:8080/api/v1/incidents \
  -H "X-CSRF-Token: <value-from-alga_csrf-cookie>" \
  -H "Content-Type: application/json" \
  -d '{"title": "..."}'
```

### Personal Access Tokens (PAT)

PATs (`alga_pat_...`) authenticate the same operator endpoints without cookies or CSRF. Effective permissions are the intersection of the PAT's granted scopes and the user's role permissions.

```sh
curl http://localhost:8080/api/v1/alerts \
  -H "Authorization: Bearer alga_pat_..."
```

### Agent Bearer Tokens

Agent endpoints use agent bearer tokens (`alga_agent_...`) and are rate-limited per agent. Agents have capability-based access (investigate, communicate, command).

```sh
curl http://localhost:8080/api/v1/agent/alerts \
  -H "Authorization: Bearer alga_agent_..."
```

### Idempotency

The following endpoints accept an `Idempotency-Key` header on POST to prevent duplicate operations:

- `POST /api/v1/incidents`
- `POST /api/v1/agent/messages`
- `POST /api/v1/users/me/notification-preferences/test`

---

## Public Routes (No Auth)

These endpoints require no authentication. Some are rate-limited.

### Health & Metrics

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/metrics` | Prometheus-format metrics |
| `ANY` | `/live` | Liveness probe |
| `ANY` | `/ready` | Readiness probe |
| `ANY` | `/health` | Health check |
| `ANY` | `/api/v1/readiness` | Pipeline readiness + scheduler/correlator snapshot |

### Initial Setup

| Method | Path | Description |
|--------|------|-------------|
| `ANY` | `/api/v1/setup/status` | Check if initial setup is complete (rate limited) |
| `ANY` | `/api/v1/setup` | Run initial setup (rate limited) |

### Authentication Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/auth/login` | Login with email and password |
| `POST` | `/api/v1/auth/logout` | Logout and clear session |
| `POST` | `/api/v1/auth/refresh` | Refresh session token |
| `POST` | `/api/v1/auth/forgot-password` | Request password reset email |
| `POST` | `/api/v1/auth/reset-password` | Reset password with token |
| `GET` | `/api/v1/auth/google/enabled` | Check if Google Sign-In is enabled |
| `ANY` | `/api/v1/auth/google` | Start Google OAuth flow |
| `ANY` | `/api/v1/auth/google/callback` | Google OAuth callback |
| `GET` | `/api/v1/auth/slack/enabled` | Check if Slack Sign-In is enabled |
| `ANY` | `/api/v1/auth/slack` | Start Slack Sign-In OAuth flow |
| `ANY` | `/api/v1/auth/slack/callback` | Slack Sign-In OAuth callback |
| `ANY` | `/api/v1/auth/oidc/providers` | List enabled OIDC SSO providers (public view) |
| `ANY` | `/api/v1/auth/oidc/{id}/authorize` | Start OIDC SSO flow for provider |
| `ANY` | `/api/v1/auth/oidc/{id}/callback` | OIDC SSO callback |

### External Callbacks

| Method | Path | Description |
|--------|------|-------------|
| `ANY` | `/api/v1/twilio/callback` | Twilio voice call IVR callback (Twilio signature verified) |
| `ANY` | `/api/v1/telnyx/callback` | Telnyx voice call IVR callback (Ed25519 signature verified) |
| `ANY` | `/api/v1/integrations/slack/oauth/callback` | Slack workspace OAuth callback |
| `ANY` | `/api/v1/users/me/slack/callback` | User Slack account linking callback |
| `ANY` | `/api/v1/users/me/google/callback` | User Google account linking callback |

### Heartbeat Ping

| Method | Path | Description |
|--------|------|-------------|
| `ANY` | `/api/v1/heartbeats/ping/{token}` | Dead-man's-switch ping (token in URL authenticates) |

### Internal

| Method | Path | Description |
|--------|------|-------------|
| `ANY` | `/internal/mm-plugin` | Serves Mattermost plugin tarball |

---

## Authenticated Routes (Session/PAT, No Specific RBAC)

These endpoints require a valid session or PAT but do not enforce a specific RBAC permission beyond basic authentication.

### Current User & Profile

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/auth/me` | Get current user info |
| `POST` | `/api/v1/auth/change-password` | Change password |
| `POST` | `/api/v1/auth/change-email` | Change email address |
| `POST` | `/api/v1/auth/profile` | Update display name / profile |

### Alerts

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/alerts` | List alerts (query: status, channel, provider, severity, search, start_date, end_date, limit, skip) |
| `POST` | `/api/v1/alerts` | Create manual alert |
| `GET` | `/api/v1/alerts/{number}` | Get alert by alert number |
| `PATCH` | `/api/v1/alerts/{number}` | Update alert fields |
| `POST` | `/api/v1/alerts/{number}/acknowledge` | Acknowledge alert |
| `POST` | `/api/v1/alerts/{number}/resolve` | Resolve alert |
| `POST` | `/api/v1/alerts/{number}/reopen` | Reopen resolved alert |
| `POST` | `/api/v1/alerts/{number}/investigate` | Trigger AI investigation |
| `DELETE` | `/api/v1/alerts/{number}` | Delete alert |

### Incidents

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/incidents/metrics` | Aggregate metrics (MTTA, MTTR, MTTM, SLA) |
| `GET` | `/api/v1/incidents` | List incidents (status, severity, service_id, commander_id, search, dates) |
| `POST` | `/api/v1/incidents` | Create manual incident (supports Idempotency-Key) |
| `GET` | `/api/v1/incidents/{id}` | Get incident with timeline, roles, linked items |
| `PATCH` | `/api/v1/incidents/{id}` | Update (title, description, severity, custom_fields) |
| `DELETE` | `/api/v1/incidents/{id}` | Delete incident |

#### Incident Lifecycle Commands

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/incidents/{id}/acknowledge` | Acknowledge (stops escalation) |
| `POST` | `/api/v1/incidents/{id}/mitigate` | Mark mitigated |
| `POST` | `/api/v1/incidents/{id}/resolve` | Mark resolved |
| `POST` | `/api/v1/incidents/{id}/close` | Mark closed |
| `POST` | `/api/v1/incidents/{id}/reopen` | Reopen resolved/mitigated/closed |
| `POST` | `/api/v1/incidents/{id}/cancel` | Cancel (false alarm) |
| `POST` | `/api/v1/incidents/{id}/escalate` | Manual escalation trigger |
| `POST` | `/api/v1/incidents/{id}/request-summary` | Request AI-generated summary |
| `POST` | `/api/v1/incidents/{id}/begin-triage` | Begin triage |
| `POST` | `/api/v1/incidents/{id}/promote` | Promote to active |

#### Incident Coordination

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/incidents/{id}/coordination/messages` | List coordination messages |
| `POST` | `/api/v1/incidents/{id}/coordination/messages` | Add coordination message |
| `GET` | `/api/v1/incidents/{id}/coordination/tasks` | List coordination tasks |
| `POST` | `/api/v1/incidents/{id}/coordination/tasks` | Create a coordination task |
| `PATCH` | `/api/v1/incidents/{id}/coordination/tasks/{taskId}` | Update coordination task |

#### Incident Status Updates

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/incidents/{id}/status-updates` | List status updates |
| `POST` | `/api/v1/incidents/{id}/status-updates` | Create status update |

#### Incident Channels & War Rooms

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/incidents/{id}/slack-channel` | Create/link a Slack incident channel |
| `DELETE` | `/api/v1/incidents/{id}/slack-channel` | Unlink Slack channel |
| `POST` | `/api/v1/incidents/{id}/google-meet` | Create/link a Google Meet war room |
| `DELETE` | `/api/v1/incidents/{id}/google-meet` | Unlink Google Meet war room |

#### Incident Linked Items

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/incidents/{id}/alerts` | List linked alerts |
| `POST` | `/api/v1/incidents/{id}/alerts` | Link alert |
| `DELETE` | `/api/v1/incidents/{id}/alerts/{alertNumber}` | Unlink alert |
| `GET` | `/api/v1/incidents/{id}/timeline` | Get structured timeline |
| `POST` | `/api/v1/incidents/{id}/timeline` | Add manual timeline entry |
| `GET` | `/api/v1/incidents/{id}/investigations` | List investigations under incident |

#### Incident Post-Mortem

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/incidents/{id}/post-mortem` | Get post-mortem |
| `POST` | `/api/v1/incidents/{id}/post-mortem` | Create post-mortem |
| `PATCH` | `/api/v1/incidents/{id}/post-mortem` | Update post-mortem |
| `DELETE` | `/api/v1/incidents/{id}/post-mortem` | Delete post-mortem |

#### ICS Roles & Document

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/incidents/{id}/ics/roles` | List ICS role assignments |
| `POST` | `/api/v1/incidents/{id}/ics/roles` | Assign ICS role |
| `PATCH` | `/api/v1/incidents/{id}/ics/roles/{roleId}` | Update an ICS role assignment |
| `GET` | `/api/v1/incidents/{id}/ics/document` | Get ICS document sections |
| `PATCH` | `/api/v1/incidents/{id}/ics/document/{section}` | Update ICS document section |

### Personal Access Tokens (Self)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/user/tokens` | List your PATs |
| `POST` | `/api/v1/user/tokens` | Create PAT |
| `DELETE` | `/api/v1/user/tokens/{id}` | Revoke your PAT |

### Routes

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/routes` | Get routing rules and default destinations |
| `PUT` | `/api/v1/routes` | Save routing rules |

### Knowledge

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/knowledge` | List/search knowledge notes |
| `POST` | `/api/v1/knowledge` | Create knowledge note |
| `GET` | `/api/v1/knowledge/{id}` | Get knowledge note |
| `PUT` | `/api/v1/knowledge/{id}` | Update knowledge note |
| `DELETE` | `/api/v1/knowledge/{id}` | Delete knowledge note |

### Memories

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/memories` | List/search memories |
| `POST` | `/api/v1/memories` | Create memory |
| `GET` | `/api/v1/memories/{id}` | Get memory |
| `PUT` | `/api/v1/memories/{id}` | Update memory |
| `DELETE` | `/api/v1/memories/{id}` | Delete memory |

### Credential Providers

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/credential-providers` | List credential providers |
| `POST` | `/api/v1/credential-providers` | Create provider |
| `GET` | `/api/v1/credential-providers/{id}` | Get provider |
| `PATCH` | `/api/v1/credential-providers/{id}` | Update provider |
| `DELETE` | `/api/v1/credential-providers/{id}` | Delete provider |

### Shared Secrets

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/shared-secrets` | List shared secrets |
| `POST` | `/api/v1/shared-secrets` | Create shared secret |
| `GET` | `/api/v1/shared-secrets/{id}` | Get shared secret |
| `PATCH` | `/api/v1/shared-secrets/{id}` | Update shared secret |
| `DELETE` | `/api/v1/shared-secrets/{id}` | Delete shared secret |

### Channels & Destinations

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/channels` | List Mattermost channels |
| `GET` | `/api/v1/destinations` | List channels (supports `?provider=slack`) |

### Integrations

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/integrations` | Get integration status |
| `PUT` | `/api/v1/integrations` | Update integrations |

### Linked Provider Accounts (Self)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/users/me/slack/authorize` | Initiate user-to-Slack linking |
| `POST` | `/api/v1/users/me/slack/disconnect` | Disconnect user's Slack account |
| `GET` | `/api/v1/users/me/google/authorize` | Initiate user-to-Google linking |
| `POST` | `/api/v1/users/me/google/disconnect` | Disconnect user's Google account |

### Dashboard

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/dashboard/stats` | Aggregate dashboard counters |
| `GET` | `/api/v1/dashboard/daily-summary` | Daily summary report |

### Notifications

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/notifications` | List notifications for current user |
| `GET` | `/api/v1/notifications/unread-count` | Get unread count |
| `POST` | `/api/v1/notifications/read-all` | Mark all read |
| `POST` | `/api/v1/notifications/{id}/read` | Mark one notification read |

### Notification Preferences

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/users/me/notification-preferences` | Get preferences |
| `PUT` | `/api/v1/users/me/notification-preferences` | Update preferences |
| `POST` | `/api/v1/users/me/notification-preferences/test` | Send test notification (supports Idempotency-Key) |

### Action Items

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/action-items` | All open action items |

### Post-Mortems

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/post-mortems` | List all post-mortems |

### System Configuration

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/system/config` | Get runtime system config |
| `PUT` | `/api/v1/system/config` | Update system config |

### Maintenance Windows

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/maintenance-windows` | List maintenance windows |
| `POST` | `/api/v1/maintenance-windows` | Create maintenance window |
| `GET` | `/api/v1/maintenance-windows/{id}` | Get maintenance window |
| `PUT` | `/api/v1/maintenance-windows/{id}` | Update maintenance window |
| `DELETE` | `/api/v1/maintenance-windows/{id}` | Delete maintenance window |

### Heartbeats (Authenticated)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/heartbeats/{id}` | Get heartbeat |
| `PUT` | `/api/v1/heartbeats/{id}` | Update heartbeat |
| `DELETE` | `/api/v1/heartbeats/{id}` | Delete heartbeat |

### Status Pages (Authenticated Sub-Routes)

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/status-pages/{id}` | Get status page by ID |
| `PATCH` | `/api/v1/status-pages/{id}` | Update status page |
| `DELETE` | `/api/v1/status-pages/{id}` | Delete status page |

### Services

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/services` | List services |
| `POST` | `/api/v1/services` | Create service |
| `GET` | `/api/v1/services/{id}` | Get service (includes status, dependencies) |
| `PATCH` | `/api/v1/services/{id}` | Update service |
| `DELETE` | `/api/v1/services/{id}` | Delete service |

### Teams

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/teams` | List teams |
| `POST` | `/api/v1/teams` | Create team (auto-provisions an on-call schedule) |
| `GET` | `/api/v1/teams/{id}` | Get team (includes members, escalation policy, schedules) |
| `PATCH` | `/api/v1/teams/{id}` | Update team |
| `DELETE` | `/api/v1/teams/{id}` | Delete team |

### Escalation Policies

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/escalation-policies` | List escalation policies |
| `POST` | `/api/v1/escalation-policies` | Create policy |
| `GET` | `/api/v1/escalation-policies/{id}` | Get policy with levels and targets |
| `PATCH` | `/api/v1/escalation-policies/{id}` | Update policy |
| `DELETE` | `/api/v1/escalation-policies/{id}` | Delete policy |

### On-Call

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/on-call/schedules` | List schedules |
| `GET` | `/api/v1/on-call/schedules/{id}` | Get schedule with layers |
| `PATCH` | `/api/v1/on-call/schedules/{id}` | Update schedule layers |
| `DELETE` | `/api/v1/on-call/overrides/{id}` | Delete override |
| `GET` | `/api/v1/on-call/handoffs` | List handoffs |
| `GET` | `/api/v1/on-call/handoffs/{id}` | Get handoff |
| `GET` | `/api/v1/on-call/who-is-on-call` | Global on-call status |
| `GET` | `/api/v1/on-call/me` | My current/pending shifts |
| `GET` | `/api/v1/on-call/my-on-call` | My on-call shifts (alternate) |
| `GET` | `/api/v1/on-call/metrics` | Pager load metrics per shift |

### Playbooks

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/playbooks` | List playbooks (kind, service_id, tag, search) |
| `POST` | `/api/v1/playbooks` | Create playbook |
| `GET` | `/api/v1/playbooks/{id}` | Get playbook with steps |
| `PATCH` | `/api/v1/playbooks/{id}` | Update playbook |
| `DELETE` | `/api/v1/playbooks/{id}` | Delete playbook |

---

## RBAC-Protected Routes

These endpoints require authentication plus the listed RBAC permission.

### Alerts (RBAC)

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/alerts/{number}/related` | `alerts:read` | Get related alerts and linked incident |
| `GET` | `/api/v1/alerts/{number}/thread` | `alerts:read` | Get alert discussion thread |
| `POST` | `/api/v1/alerts/{number}/thread/typing` | `alerts:write` | Send typing indicator to alert thread |
| `POST` | `/api/v1/alerts/{number}/thread/messages` | `alerts:write` | Add message to alert thread |
| `PATCH` | `/api/v1/alert-investigations/{id}/assign` | `alerts:write` | Assign investigation to an agent |

### Webhook Tokens

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/webhook-tokens` | `tokens:manage` | List webhook tokens |
| `POST` | `/api/v1/webhook-tokens` | `tokens:manage` | Create webhook token |
| `DELETE` | `/api/v1/webhook-tokens/{id}` | `tokens:manage` | Revoke webhook token |

### Admin Tokens

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/admin/tokens` | `tokens:manage` | List all PATs |
| `DELETE` | `/api/v1/admin/tokens/{id}` | `tokens:manage` | Revoke any PAT |

### Integrations (RBAC)

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `POST` | `/api/v1/integrations/test` | `integrations:test` | Test Mattermost or Slack connection |
| `POST` | `/api/v1/integrations/slack/oauth/authorize` | `integrations:write` | Initiate Slack OAuth flow |
| `POST` | `/api/v1/integrations/slack/disconnect` | `integrations:write` | Disconnect Slack workspace |

### Users (Admin)

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/users` | `users:manage` | List users |
| `POST` | `/api/v1/users` | `users:manage` | Create user |
| `PUT` | `/api/v1/users/{id}` | `users:manage` | Update user |
| `DELETE` | `/api/v1/users/{id}` | `users:manage` | Delete user |

### Onboarding

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/onboarding/status` | `system:read` | Check if onboarding wizard is completed |
| `POST` | `/api/v1/onboarding/complete` | `system:write` | Mark onboarding as completed |

### Heartbeats (RBAC)

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/heartbeats` | `heartbeats:read` | List heartbeats |
| `POST` | `/api/v1/heartbeats` | `heartbeats:write` | Create heartbeat |

### Status Pages (RBAC)

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/status-pages` | `statuspages:read` | List status pages |
| `POST` | `/api/v1/status-pages` | `statuspages:write` | Create status page |

### OIDC Providers (Admin)

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/oidc/providers` | `oidc:manage` | List configured OIDC providers |
| `POST` | `/api/v1/oidc/providers` | `oidc:manage` | Create provider |

### Incidents (RBAC)

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/incidents/{id}/thread` | `incidents:read` | Get incident investigation thread |
| `POST` | `/api/v1/incidents/{id}/thread/messages` | `incidents:write` | Add thread message |
| `PATCH` | `/api/v1/incident-investigations/{id}/assign` | `incidents:write` | Assign incident investigation to an agent |

### Triage

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/triage/rules` | `triage:read` | List triage rules |
| `POST` | `/api/v1/triage/rules` | `triage:write` | Create triage rule |
| `PUT` | `/api/v1/triage/rules/reorder` | `triage:write` | Reorder triage rules |
| `GET` | `/api/v1/triage/results` | `triage:read` | List triage results |
| `GET` | `/api/v1/triage/stats` | `triage:read` | Get triage accuracy stats |

### Investigations (Admin)

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/investigations/dead-lettered` | `admin:access` | List dead-lettered investigations |

---

## Agent Routes (Bearer Token)

All agent endpoints require agent bearer token auth (`alga_agent_...`) and are rate-limited per agent. Access is further gated by agent capabilities (investigate, communicate, command).

### Operator-Facing Agent Management

These endpoints are used by operators to manage agents. They require session/PAT auth with `tokens:manage`.

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/agent-tokens` | `tokens:manage` | List agent tokens (with online presence) |
| `POST` | `/api/v1/agent-tokens` | `tokens:manage` | Create agent token |
| `PUT` | `/api/v1/agent-tokens/{id}` | `tokens:manage` | Update agent config (enabled, scope, label_selectors) |
| `DELETE` | `/api/v1/agent-tokens/{id}` | `tokens:manage` | Revoke agent token |
| `GET` | `/api/v1/agent-tokens/{id}/chat/messages` | `tokens:manage` | Get agent DM history |
| `POST` | `/api/v1/agent-tokens/{id}/chat/messages` | `tokens:manage` | Send message to agent |
| `GET` | `/api/v1/agent/capabilities` | `tokens:manage` | List available agent capabilities |

### Agent-Facing REST API

These endpoints are called by agents using their bearer token.

#### Alerts

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/agent/alerts` | List alerts |
| `GET` | `/api/v1/agent/alerts/{fingerprint}` | Get alert by fingerprint |
| `POST` | `/api/v1/agent/alerts/{fingerprint}/resolve` | Resolve alert |
| `POST` | `/api/v1/agent/alerts/{fingerprint}/reopen` | Reopen alert |

#### Memories

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/agent/memories` | List/search memories |
| `POST` | `/api/v1/agent/memories` | Create memory |

#### Communication

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/agent/peer-ask` | List peer asks |
| `POST` | `/api/v1/agent/peer-ask` | Ask another agent for help |
| `GET` | `/api/v1/agent/events` | SSE stream (investigation dispatch, chat, peer findings) |
| `POST` | `/api/v1/agent/messages` | Send a text message or invoke an agent tool (supports Idempotency-Key) |
| `POST` | `/api/v1/agent/drafts` | Store an unsent draft |
| `POST` | `/api/v1/agent/typing` | Typing indicator |
| `POST` | `/api/v1/agent/heartbeat` | Renew SSE presence lease |

#### Playbooks & Knowledge

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/agent/playbooks` | List playbooks |
| `GET` | `/api/v1/agent/knowledge` | List/search knowledge notes |

#### Incidents

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/agent/incidents/{id}` | Get incident context (only if assigned) |
| `GET` | `/api/v1/agent/incidents/{id}/timeline` | Get incident timeline |
| `GET` | `/api/v1/agent/incidents/{id}/tasks` | List coordination tasks for incident |

#### Services & On-Call

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/agent/services` | List services |
| `GET` | `/api/v1/agent/on-call/current` | Who is on call |

#### Secrets

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/agent/secrets/{id}` | Fetch a shared secret the agent is authorized to read |

::: tip Agent tools via messages
Agent capabilities like assigning roles, triggering escalation, publishing status updates, and promoting incidents are invoked by posting to `/api/v1/agent/messages` with `kind: "inv_tool"`. See the [Agent SDKs](/integrations/agent-sdks) for the command factory helpers.
:::

---

## RBAC Permissions

Alga defines 53 granular permissions organized by resource domain.

| Domain | Permissions |
|--------|-------------|
| Alerts | `alerts:read`, `alerts:write`, `alerts:delete` |
| Knowledge | `knowledge:read`, `knowledge:write`, `knowledge:delete` |
| Routes | `routes:read`, `routes:write` |
| Integrations | `integrations:read`, `integrations:write`, `integrations:test` |
| Users | `users:manage` |
| Tokens | `tokens:manage` |
| Dashboard | `dashboard:read` |
| Channels | `channels:read` |
| Audit | `audit:read` |
| Notifications | `notifications:read`, `notifications:write` |
| Memories | `memories:read`, `memories:write`, `memories:delete` |
| System | `system:read`, `system:write` |
| Triage | `triage:read`, `triage:write`, `triage:override` |
| Incidents | `incidents:read`, `incidents:write`, `incidents:command`, `incidents:delete` |
| Services | `services:read`, `services:write` |
| On-Call | `oncall:read`, `oncall:write` |
| Escalation | `escalation:read`, `escalation:write` |
| Post-Mortems | `postmortems:read`, `postmortems:write`, `postmortems:delete` |
| Playbooks | `playbooks:read`, `playbooks:write`, `playbooks:delete` |
| Heartbeats | `heartbeats:read`, `heartbeats:write`, `heartbeats:delete` |
| Status Pages | `statuspages:read`, `statuspages:write`, `statuspages:delete` |
| OIDC | `oidc:manage` |
| Credentials | `credentials:manage`, `credentials:read` |
| Admin | `admin:access` |

## Roles

| Role | Scope |
|------|-------|
| **admin** | All 53 permissions |
| **operator** | Most read/write permissions; no delete, users:manage, tokens:manage, system:read/write, admin:access, or oidc:manage |
| **viewer** | Read-only permissions across all domains |
