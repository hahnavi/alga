---
title: Troubleshooting
description: Common issues and solutions — health checks, logs, metrics, database problems, and getting help.
---

# Troubleshooting

## Common Issues

### Can't Connect to Database

**Symptom:** Backend fails to start with PostgreSQL connection errors.

**Fix:**

- Verify `POSTGRES_DSN` is correct: `postgres://user:pass@host:5432/alga?sslmode=disable`
- Check PostgreSQL is running: `docker compose ps postgres`
- Check connectivity: `docker compose exec backend wget -qO- postgres:5432`

### Session Expires Immediately

**Symptom:** Logged-in users are immediately logged out.

**Fix:**

- Ensure `SECRET_PEPPER` is consistent across restarts (don't regenerate)
- Check `SECURE_COOKIES` matches your setup (only enable with HTTPS)

### Webhooks Return 401

**Symptom:** Alert webhooks fail with `401 Unauthorized`.

**Fix:**

- Verify the bearer token is correct and not revoked
- Check token format: `Authorization: Bearer alga_...`
- Send the token as an `Authorization: Bearer alga_...` header. The legacy `?token=alga_...` query parameter is denied by default; only enable `WEBHOOK_ALLOW_QUERY_TOKEN=true` as a temporary escape hatch

### Mattermost Not Receiving Alerts

**Symptom:** Alerts appear in Alga but not in Mattermost.

**Fix:**

- Verify `MATTERMOST_SERVER_URL` is set correctly
- Check `MATTERMOST_WEBHOOK_SECRET` matches the plugin config
- Ensure `MATTERMOST_DISABLED` is not set to `true`
- Test with `POST /api/v1/integrations/test`

### Slack Not Receiving Alerts

**Symptom:** Alerts appear in Alga but not in Slack.

**Fix:**

- Verify `SLACK_BOT_TOKEN` starts with `xoxb-`
- Check the bot is added to the target channel
- Ensure `SLACK_DISABLED` is not set to `true`
- Test with `POST /api/v1/integrations/test`

### SSE Disconnects Frequently

**Symptom:** Agent or frontend SSE connections drop repeatedly.

**Fix:**

- Check reverse proxy timeout settings (nginx: `proxy_read_timeout 86400s`)
- Ensure no load balancer is terminating idle connections
- Verify `AGENT_PRESENCE_TTL` is longer than heartbeat interval

### Agent Not Connecting

**Symptom:** Agent shows as offline despite being configured.

**Fix:**

- Verify the agent token is enabled (`POST /api/v1/agent-tokens/{id}/enable`)
- Check the SSE endpoint: `GET /api/v1/agent/events` with bearer token
- Verify network connectivity to the Alga backend
- Check `AGENT_SSE_ALLOWED_ORIGINS` if using browser-based agents

### Investigation Not Starting

**Symptom:** Alerts are firing but no investigations are created.

**Fix:**

- Ensure `RABBITMQ_URI` is configured (required for investigations)
- Check `CORRELATION_WINDOW` is > 0 (0 disables correlation)
- Verify at least one agent is online (check Agents page)
- Check `MAX_CONCURRENT_INVESTIGATIONS` isn't reached

### Dead-Lettered Investigations

**Symptom:** Investigations show `dead_lettered` status and are not retried automatically.

**Fix:**

- List dead-lettered investigations: `GET /api/v1/investigations?status=dead_lettered`
- Check the investigation's error message for the root cause (e.g., agent timeout, external service unreachable)
- Retry manually: `POST /api/v1/investigations/{id}/retry`
- If retries keep failing, check agent health and connectivity
- Increase retry TTL in the RabbitMQ retry topology if transient failures need more recovery time
- Review `INVESTIGATION_TIMEOUT` — if agents are consistently timing out, increase the value

#### Broker Dead-Letter Queue (`alga.dead_letter`)

Messages that exhaust all four retries across every domain land here. There is intentionally no built-in replay tooling (steady-state depth SHOULD be near zero); inspect and drain with the RabbitMQ management plugin or CLI:

```sh
# Inspect depth and sample messages
rabbitmqctl list_queues name messages | grep alga.dead_letter
rabbitmqadmin get queue=alga.dead_letter count=10 requeue=true

# Requeue (publish back to origin exchange/routing key from x-death headers),
# or purge once the cause is fixed
rabbitmqadmin purge queue name=alga.dead_letter
```

Fix the underlying consumer first (check worker logs for the repeated decode/process error); purging without fixing only silences the signal.

### Playbook Not Matching

**Symptom:** A playbook exists but is not being attached to investigations.

**Fix:**

- Verify the playbook's label selectors match the alert's actual labels (exact match is required for each selector key)
- Check that the playbook is enabled (not in draft or disabled state)
- Confirm the alert labels include all keys referenced in the selector — partial matches are not applied
- Use the API to list playbooks and review their selectors: `GET /api/v1/playbooks`
- Test label matching by comparing alert labels (`GET /api/v1/alerts/{fp}`) against the playbook selector

### ICS Role Not Auto-Assigned

**Symptom:** Incidents are created but ICS roles (Incident Commander, Scribe, etc.) are not automatically assigned.

**Fix:**

- Verify the affected service has an escalation policy attached
- Check that an on-call schedule is linked to the escalation policy's targets
- Ensure the on-call schedule has at least one active rotation with members
- Confirm the service's ICS auto-assignment setting is enabled
- Check the escalation policy targets include a `team` target so Alga can resolve the current responder via the team's on-call schedule
- Review incident timeline for any role assignment errors

### Handoff Not Triggering

**Symptom:** On-call shift transitions happen without a handoff record.

**Fix:**

- Confirm the team's on-call schedule has at least one active rotation layer with members
- Verify the shift actually changes the resolved on-call user (handoffs are generated when the resolved responder changes)
- Check that the outgoing and incoming engineers have notification preferences configured (in-app, email, Slack, or voice)
- Ensure Valkey is connected (used for on-call caching)
- Check backend logs for handoff generation errors around shift transition times

## Health Checks

### Liveness

```sh
curl http://localhost:8080/health
```

Returns `200 OK` if the HTTP server is running.

### Readiness

```sh
curl http://localhost:8080/api/v1/readiness
```

Returns pipeline readiness status, scheduler state, and correlator snapshot. Useful for load balancer health checks.

## Logs

### Backend

```sh
# Docker Compose
docker compose logs -f backend

# Direct
LOG_LEVEL=debug ./alga
```

Log levels: `debug`, `info`, `warn`, `error`

### Frontend

```sh
docker compose logs -f frontend
```

Frontend is served by nginx. Check access logs for routing issues.

## Metrics

### Prometheus

```sh
curl http://localhost:8080/metrics
```

Key metrics:

- `alga_correlator_alerts_total` — alerts received
- `alga_scheduler_pending` — pending investigations
- `alga_scheduler_online_agents` — connected agents
- `alga_scheduler_bind_failed_total` — failed investigation dispatches

### Grafana Dashboard

Import `deploy/grafana/alga-dashboard.json` into Grafana for a pre-built monitoring dashboard.

## Database

### Auto-Migration

Alga migrates the schema on startup when `POSTGRES_AUTO_MIGRATE=true`:

```sh
# Manual migration
./alga db migrate
```

### Reset Password Without Email

```sh
./alga user reset-password user@example.com
```

This generates a temporary password printed to stdout. Does not require SMTP.

## Getting Help

- [GitHub Issues](https://github.com/hahnavi/alga/issues) — bug reports and feature requests
- [GitHub Discussions](https://github.com/hahnavi/alga/discussions) — questions and community support
