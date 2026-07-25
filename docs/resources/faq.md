---
title: FAQ
description: Frequently asked questions about setup, configuration, integrations, features, performance, security, licensing, agents, and troubleshooting.
---

# Frequently Asked Questions (FAQ)

Common questions about Alga setup, integrations, features, troubleshooting, and more.

## Table of Contents

- [Setup & Installation](#setup--installation)
- [Configuration](#configuration)
- [Integrations](#integrations)
- [Features & Usage](#features--usage)
- [Performance & Scaling](#performance--scaling)
- [Security & Compliance](#security--compliance)
- [Licensing & Deployment](#licensing--deployment)
- [Slack-Specific Questions](#slack-specific-questions)
- [AI Agent Configuration](#ai-agent-configuration)
- [Database & Infrastructure](#database--infrastructure)
- [Troubleshooting](#troubleshooting)

## Setup & Installation

### How do I install Alga?

**Quick Install:**
```bash
git clone https://github.com/hahnavi/alga.git
cd alga
pnpm install
docker compose up -d
```

See [Quick Start Guide](/getting-started/) for details.

### What are minimum system requirements?

**Minimum:**
- CPU: 2 cores
- RAM: 4 GB
- Disk: 20 GB
- Go 1.26.5+
- Node.js 18+

**Recommended for production:**
- CPU: 4+ cores
- RAM: 8+ GB
- Disk: 50+ GB SSD
- Dedicated PostgreSQL, Valkey, RabbitMQ

### How do I upgrade Alga?

**Using Git:**
```bash
git fetch origin
git checkout v1.2.3
git pull origin v1.2.3
pnpm install
go run . db migrate
```

**Using Docker:**
```bash
docker compose pull
docker compose up -d
```

Always backup your database before upgrading!

## Configuration

### Where do I configure Alga?

Configuration via environment variables:

1. **Backend config:** `apps/backend/.env`
2. **Frontend config:** `apps/frontend/.env`
3. **Docker config:** `.env` (project root)

See [Configuration Guide](/configuration/environment-variables) for all options.

### How do I set up encryption keys?

Generate encryption keys for production:

```bash
# Generate 32-byte key (base64 encoded)
openssl rand -base64 32

# For key rotation
ENCRYPTION_KEYS="1:$(openssl rand -base64 32),2:$(openssl rand -base64 32)"
```

See [Security Guide](/configuration/security) for details.

## Integrations

### Which monitoring tools does Alga support?

Alga accepts webhooks from **any** tool that sends HTTP POST requests:

- **Grafana:** Use webhook
- **Prometheus:** Use Alertmanager webhook
- **Datadog:** Use webhook integration
- **CloudWatch:** Use SNS → Lambda → Webhook
- **New Relic:** Use webhook integration
- **Zabbix:** Use webhook media type

See [Integration Guide](../integrations/) for examples.

### How do I integrate with Slack?

1. Create Slack app at https://api.slack.com/apps
2. Enable Bot permissions
3. Add OAuth scopes (`chat:write`, `channels:history`)
4. Install app to workspace
5. Set environment variables:
```bash
SLACK_BOT_TOKEN="xoxb-your-token"
SLACK_DEFAULT_CHANNEL="#alerts"
```

See [Slack Integration Guide](/integrations/slack) for details.

### How do I integrate with Mattermost?

1. Install Mattermost plugin from `integrations/alga-mattermost-plugin/`
2. Configure plugin with Alga webhook URL
3. Set environment variables:
```bash
MATTERMOST_SERVER_URL="https://mattermost.example.com"
MATTERMOST_WEBHOOK_SECRET="shared-secret"
MATTERMOST_DEFAULT_CHANNEL="alerts"
```

### Can I use both Slack and Mattermost simultaneously?

Yes! Multi-platform routing:

```yaml
routes:
  - name: Critical Alerts
    destinations:
      - channel: "#incidents"
        provider: slack
      - channel: "ops-team"
        provider: mattermost
```

## Features & Usage

### How does alert correlation work?

Alga groups alerts sharing same `deployment`, `statefulset`, `daemonset`, or `job` (combined with `namespace` and `alertname`) within a time window:

```bash
CORRELATION_WINDOW=5m
```

### Can I manually create alerts?

Yes! Use API or web UI:

**API:**
```bash
curl -X POST http://localhost:8080/api/v1/alerts \
  -H "Content-Type: application/json" \
  -d '{"fingerprint": "manual-1", "labels": {"severity": "critical"}}'
```

**Web UI:** Navigate to Alerts page → "Create Alert" button.

### How do I acknowledge an alert?

**API:** `POST /api/v1/alerts/{alert_number}/acknowledge`
**Web UI:** Click "Acknowledge" button
**Slack:** Use `/alga ack {fingerprint}` command

### How do I set up on-call schedules?

**API:**
```bash
curl -X POST http://localhost:8080/api/v1/on-call/schedules \
  -d '{"name": "Primary", "rotation_period_days": 7}'
```

**Web UI:** Navigate to On-Call page → "Create Schedule"

### How do escalation policies work?

Multi-tier escalation with delays:

```json
{
  "name": "Critical Escalation",
  "levels": [
    {"delay_minutes": 5, "targets": [{"type": "on_call"}]},
    {"delay_minutes": 10, "targets": [{"type": "team"}]},
    {"delay_minutes": 15, "targets": [{"type": "user"}]}
  ]
}
```

Escalation stops when incident is acknowledged.

### What are playbooks?

Playbooks are **structured response procedures** that are automatically matched to investigations based on alert label selectors. When an investigation starts, Alga checks if any playbook's label selectors match the incoming alert's labels. If a match is found, the playbook's step-by-step procedure is attached to the investigation, giving the responding team (or AI agent) a clear remediation path. Playbooks reduce mean-time-to-resolution by ensuring consistent, repeatable responses to known alert patterns.

### What is ICS?

**ICS (Incident Command System)** is a structured incident response framework adapted from emergency management. When an incident is created, Alga can automatically assign ICS roles — Incident Commander, Operations Lead, Scribe, and Liaison — based on the on-call schedule and escalation policy configured on the affected service. ICS brings clear role accountability to incident response: the Incident Commander drives overall coordination, the Operations Lead manages technical remediation, the Scribe documents timeline entries and decisions, and the Liaison handles stakeholder communication.

### What are dead-lettered investigations?

When an investigation fails repeatedly, it passes through Alga's RabbitMQ retry topology (three retry queues with exponential backoff). If it exhausts all retry attempts, it is **dead-lettered** — moved to a terminal state where it will no longer be retried automatically. Dead-lettered investigations are visible via `GET /api/v1/investigations?status=dead_lettered` and can be manually retried with `POST /api/v1/investigations/{id}/retry`. Reviewing dead-lettered investigations helps identify systemic issues like misconfigured agents or unreachable external services.

### How do handoffs work?

**Handoffs** provide structured on-call shift transitions. When an on-call engineer's shift ends, Alga can generate a handoff summary that includes: active incidents and their current status, ongoing investigations and their progress, any pending escalations, and freeform notes from the outgoing engineer. Handoffs are triggered based on the on-call schedule configuration and `ON_CALL_REMIND_ENABLED` / `ON_CALL_REMIND_MINUTES` settings. The incoming engineer receives the handoff via their configured notification channels (in-app, email, Slack, or Mattermost).

### What are personal access tokens?

**Personal access tokens** are long-lived bearer tokens for machine-to-machine API authentication. Unlike session-based user authentication (which requires cookies and CSRF tokens), personal access tokens are sent via the `Authorization: Bearer` header and are ideal for CI/CD pipelines, automation scripts, and external tool integrations. Tokens are created per-user, inherit the user's RBAC permissions, and can be revoked independently without affecting the user's session. Like all secrets in Alga, token values are stored as HMAC hashes — the plaintext is shown exactly once at creation time.

## Performance & Scaling

### How many alerts can Alga handle?

- **Single instance:** ~1,000 alerts/second
- **With horizontal scaling:** Unlimited

Performance depends on database indexing, RabbitMQ sizing, and network bandwidth.

### How do I scale Alga horizontally?

1. Deploy multiple replicas behind load balancer
2. Use shared PostgreSQL instance
3. Use shared Valkey/Redis instance
4. Use shared RabbitMQ instance
5. Configure `SCHEDULER_LEADER_TTL` for leader election

### How do I monitor Alga performance?

Alga exposes metrics at `/debug/vars`:

- `alga_correlator_*` - Correlator metrics
- `alga_scheduler_*` - Scheduler metrics
- `alga_webhook_*` - Webhook metrics

Use Prometheus exporter to scrape these metrics.

## Security & Compliance

### Is Alga secure for production use?

Yes! Production-grade security:

- **Session-based authentication** with HTTP-only cookies
- **CSRF protection** via double-submit pattern
- **RBAC** with three built-in roles
- **Argon2id password hashing** with pepper
- **AES-256-GCM encryption** for secrets
- **Rate limiting** for login and API endpoints
- **Audit logging** for all actions

See [Security Guide](/configuration/security) for details.

### How do I protect secrets?

**Never commit secrets!**

1. Use `.env` files (gitignored)
2. Use environment variables
3. Use secret management (Kubernetes Secrets, AWS Secrets Manager)
4. Rotate keys regularly
5. Audit access to secrets

### Is Alga compliant with SOC2/HIPAA?

Alga provides foundation for compliance:

- **Audit logging:** All actions logged with timestamps and user IDs
- **Data encryption:** At rest (AES-256-GCM) and in transit (TLS)
- **Access controls:** RBAC with least-privilege model
- **Data retention:** Configurable retention policies
- **Self-hosting:** Full control over data location

## Licensing & Deployment

### What license does Alga use?

**MIT License**:

- Free to use, modify, and distribute
- Commercial use allowed
- Must include license and copyright notice

### Can I use Alga commercially?

Yes! The MIT License permits commercial use without licensing fees.

### Where can I deploy Alga?

**Cloud providers:**
- AWS (EC2, RDS, ElastiCache)
- Google Cloud (Compute Engine, Cloud SQL)
- Azure (VMs, Azure Database)

**Self-hosted:**
- Bare metal servers
- Virtual machines
- Kubernetes clusters

**Managed services:**
- Render
- Railway
- DigitalOcean App Platform

## Slack-Specific Questions

### Why aren't my alerts showing up in Slack?

**Troubleshooting:**

1. Check `SLACK_BOT_TOKEN` is correct
2. Verify channel exists and bot is invited
3. Bot needs `chat:write` scope
4. Verify routing rules match your alerts
5. Check logs for Slack delivery errors

### How do I use Slack slash commands?

Alga supports these commands:

- `/alga ack {fingerprint}` - Acknowledge alert
- `/alga resolve {fingerprint}` - Resolve alert
- `/alga investigate {fingerprint}` - Trigger investigation
- `/alga status` - Show status

### Why is Slack text truncated?

Slack has message size limits (~40,000 characters). Alga automatically truncates large payloads.

## AI Agent Configuration

### How do I connect an agent to Alga?

1. Create agent token via API:
```bash
curl -X POST http://localhost:8080/api/v1/agent-tokens \
  -d '{"name": "my-agent"}'
```

2. Connect agent via SSE:
```bash
curl -H "Authorization: Bearer {token}" \
  http://localhost:8080/api/v1/agent/events
```

### How do I configure agent capacity?

```bash
curl -X PUT http://localhost:8080/api/v1/agent-tokens/{id} \
  -d '{"max_concurrent_investigations": 5}'
```

### How do agents send messages?

Use unified agent API:

```bash
curl -X POST http://localhost:8080/api/v1/agent/messages \
  -H "Authorization: Bearer {token}" \
  -d '{"chat_id": "investigation-123", "kind": "text", "text": "Update"}'
```

## Database & Infrastructure

### What database does Alga use?

**PostgreSQL 18** is required.

- Used for all persistent data (alerts, incidents, users, etc.)
- Schema managed via Ent ORM migrations
- Supports pgvector for agent memory system

### Can I use managed database services?

Yes! Alga works with:

- AWS RDS (PostgreSQL)
- Google Cloud SQL (PostgreSQL)
- Azure Database for PostgreSQL
- Neon, Supabase, Railway Postgres

### How do I back up the database?

**Using pg_dump:**
```bash
pg_dump postgresql://user:pass@localhost:5432/alga > backup.sql
```

**Using Docker Compose:**
```bash
docker compose exec postgres pg_dump -U alga alga > backup.sql
```

Automate with cron or managed backup services.

### Do I need Valkey/Redis?

Valkey/Redis is **optional but recommended** for:

- Session storage with refresh token rotation
- Caching and rate limiting
- Agent presence and leader election
- SSE cross-replica fan-out

Without Valkey, Alga falls back to PostgreSQL for some features.

### Do I need RabbitMQ?

RabbitMQ is **optional but recommended** for:

- Async alert processing with retry topology
- Background message consumers
- High-throughput message queuing

Without RabbitMQ, some async features are disabled.

## Troubleshooting

### Alga won't start - what do I do?

**Check logs:**
```bash
# Docker
docker compose logs backend

# Direct
go run .  # Check for startup errors
```

**Common issues:**

1. **Database connection failed:** Check `POSTGRES_DSN`
2. **Port already in use:** Change `PORT` or stop conflicting service
3. **Missing encryption keys:** Set `ENCRYPTION_KEY` for production

### Alerts aren't arriving - why?

**Check:**

1. Webhook URL is correct
2. Bearer token is valid
3. Routing rules match your alert labels
4. Integration services are reachable (Slack, Mattermost)
5. Check backend logs for delivery errors

### Login is failing - what's wrong?

**Check:**

1. User exists and is enabled
2. Password is correct (case-sensitive)
3. Account not locked (5 failed attempts = 30-minute lockout)
4. Session cookie is being set
5. CSRF token is present

**Reset lockout:**
```sql
UPDATE users SET failed_login_attempts = 0 WHERE email = 'user@example.com';
```

### Database migration failed - what now?

**Common causes:**

1. **Migration conflict:** Manually resolve in `apps/backend/ent/schema/`
2. **Connection error:** Check database is accessible
3. **Schema mismatch:** Dump and restore fresh database

**Force re-run:**
```bash
# WARNING: This drops all data!
DROP DATABASE alga;
CREATE DATABASE alga;
go run . db migrate;
```

### Agent can't connect - why?

**Check:**

1. Agent token is valid and enabled
2. SSE endpoint is reachable
3. CORS is configured correctly
4. `AGENT_SSE_ALLOWED_ORIGINS` includes your origin

### Performance is slow - how to tune?

**Database:**

1. Index frequently queried fields
2. Analyze query plans with `EXPLAIN ANALYZE`
3. Increase connection pool size
4. Use read replicas for queries

**Backend:**

1. Increase worker pool sizes
2. Tune `MAX_CONCURRENT_PASSWORD_HASHES`
3. Enable Valkey for caching
4. Profile with `pprof`

**Infrastructure:**

1. Scale PostgreSQL resources
2. Use SSD storage
3. Optimize network configuration
4. Add CDN for static assets

## Getting Help

Still have questions?

- [Documentation](../getting-started/)
- [API Reference](../api-reference/)
- [GitHub Issues](https://github.com/hahnavi/alga/issues)
- [Discussions](https://github.com/hahnavi/alga/discussions)

## Additional Resources

- [Quick Start Guide](/getting-started/)
- [Configuration Guide](/configuration/environment-variables)
- [Integration Guide](/integrations/)
- [Security Guide](/configuration/security)
- [Performance Guide](/operations/performance)
- [Troubleshooting Guide](/resources/troubleshooting)
