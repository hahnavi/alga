---
title: Onboarding Wizard
description: First-run onboarding wizard — guided setup for admin password, integrations, routing rules, and initial configuration.
---

# Onboarding Wizard

Alga includes a first-run onboarding wizard that guides new installations through initial setup. The wizard activates automatically once the initial admin account has been created via the setup wizard.

## Accessing the Wizard

On first boot with no users in the database, the frontend redirects to `/setup` where the operator creates the initial admin account (email, password, and full name). After the setup wizard is completed, the first login with those credentials triggers the onboarding page. Complete the wizard to access the main application.

## What the Wizard Covers

### 1. Change Default Password

The first step requires changing the admin password away from the value set during the setup wizard. Alga enforces a password policy:

- Minimum 8 characters
- At least one uppercase letter
- At least one lowercase letter
- At least one digit
- At least one special character

The password used during setup is rejected on subsequent password changes — choose a new, strong password before proceeding.

### 2. Configure Basic Settings

Set up the core alerting and notification configuration:

- **Alert source** — where alerts come from (Grafana, Prometheus, etc.)
- **Notification channels** — Mattermost server URL, Slack bot token, or email (SMTP)

### 3. Create First Webhook Token

Generate a webhook token for sending alerts to Alga:

```sh
curl -X POST http://localhost:8080/api/v1/webhook-tokens \
  -H "Authorization: Bearer $SESSION" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Grafana Production"
  }'
```

Use the returned token in your alerting source configuration:

```sh
curl -X POST http://localhost:8080/webhooks/alerts \
  -H "Authorization: Bearer alga_..." \
  -H "Content-Type: application/json" \
  -d '{
    "labels": {
      "alertname": "HighErrorRate",
      "severity": "critical"
    },
    "annotations": {
      "summary": "Error rate exceeded 10% threshold"
    }
  }'
```

### 4. Verify Setup

The final step verifies the configuration by:

- Confirming the webhook token was created
- Testing connectivity to configured notification channels (if any)
- Creating the first test alert

## Skipping the Wizard

The wizard can be completed in sequence or dismissed. After completion, the onboarding page is no longer accessible and the admin is redirected to the dashboard.

## API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/onboarding/status` | Session | Check if onboarding is completed |
| `POST` | `/api/v1/onboarding/complete` | Session | Mark onboarding as completed |

### Checking Onboarding Status

```sh
curl http://localhost:8080/api/v1/onboarding/status \
  -H "Authorization: Bearer $SESSION"
```

Response:

```json
{
  "completed": false
}
```

## Environment Setup

Before running the onboarding wizard, ensure the following infrastructure is running:

| Service | Required | Purpose |
|---------|----------|---------|
| PostgreSQL | Yes | Data persistence |
| Valkey/Redis | No | Sessions, caching, rate limiting, leader election |
| RabbitMQ | No | Async alert processing and investigation pipeline |

Start all services with Docker Compose:

```sh
docker compose up -d
```

## See Also

- [Installation & Setup](/getting-started/installation) — deploying Alga
- [First Steps Guide](/getting-started/first-steps) — walkthrough after onboarding
- [Security & Authentication](/configuration/security) — authentication, RBAC, and session management
