---
title: First Steps Guide
description: A 12-step walkthrough from creating the initial admin account through sending test alerts, connecting Grafana, and securing your instance.
---

# First Steps Guide

You have Alga running. Here's what to do next.

## 1. Complete First-Run Setup

Open `http://localhost:3000` in your browser. When no users exist in the database, Alga automatically redirects to the setup wizard. Enter an email, password, and full name to create the initial admin account. This is the only time the setup wizard is available — once an admin exists, it is disabled.

## 2. Create a Webhook Token

Go to **Settings → Webhook Tokens** and create a token. You'll need this to send alerts.

## 3. Send a Test Alert

```sh
curl -X POST http://localhost:8080/webhooks/alerts \
  -H "Authorization: Bearer alga_YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "alerts": [{
      "status": "firing",
      "labels": {
        "alertname": "TestAlert",
        "severity": "warning",
        "namespace": "default"
      },
      "annotations": {
        "summary": "This is a test alert"
      },
      "generatorURL": "http://localhost:3000/test"
    }]
  }'
```

You should see the alert appear on the **Alerts** page.

## 4. Connect Grafana

1. In Alga, copy your webhook token
2. In Grafana, go to **Alerting → Contact points → Add contact point**
3. Set type to **Webhook**
4. Set URL to `http://your-alga-host:8080/webhooks/alerts?token=alga_YOUR_TOKEN`
5. Save and test

## 5. Set Up Routing Rules

Go to **Routes** and create rules to route alerts to specific channels:

1. Click **Add Rule**
2. Set conditions (e.g., `namespace = production`)
3. Set destination channels (Slack or Mattermost)
4. Save

Alerts that don't match any rule go to the default channel.

## 6. Create Knowledge Notes

Knowledge notes capture operational runbooks and context so your team (and AI agents) know exactly what to do when an alert fires.

1. Go to **Knowledge** in the sidebar
2. Create notes with tags and selectors matching your alert labels (e.g., `alertname: HighMemory`)
3. AI agents and operators can reference these during investigations

Knowledge notes help standardize incident response and reduce mean time to resolution. For structured, step-by-step procedures, see [Playbooks](/core-features/playbooks).

## 7. Explore Core Features

| Feature            | Where               | What                                  |
| ------------------ | ------------------- | ------------------------------------- |
| **Alerts**         | Alerts page         | View, acknowledge, resolve alerts     |
| **Investigations** | Investigations page | AI-powered root cause analysis        |
| **Knowledge**      | Knowledge page      | Shared notes for operators and agents |
| **Routing**        | Routes page         | Alert routing rules                   |

## 8. Set Up Incident Management

1. Create **Services** in the Service Catalog
2. Define **Dependencies** between services
3. Create **Teams** and add members
4. Set up **On-Call Schedules** with rotations
5. Create **Escalation Policies** with tiers and delays

## 9. Set Up On-Call and Escalation

On-call schedules ensure the right person is always reachable. Escalation policies define what happens when they don't respond.

1. Go to **On-Call → Schedules** and create a rotation
2. Add layers for follow-the-sun coverage if you have multiple time zones
3. Create an **Escalation Policy** with tiered levels and delays
4. Assign the escalation policy to a service (target a team or user within each level)

See [Teams & On-Call](/on-call/) for detailed schedule and escalation configuration.

## 10. Connect an Agent (Optional)

To enable automated investigations:

1. Go to **Agents** in the sidebar menu and create an agent token
2. Install the Hermes adapter or OpenClaw plugin
3. Configure the agent to connect to `http://your-alga-host:8080/api/v1/agent/events` with the agent token

See [AI Investigation](/core-features/investigation) for details.

## 11. Configure Notifications

Each user can set their own notification preferences for how they receive alerts:

1. Click your **profile avatar** → **Notification Preferences**
2. Add rules mapping notification types to channels (in-app, email, Slack DM, voice)
3. Set a default channel for notification types without an explicit rule
4. Optionally enable the voice opt-out to suppress phone calls
5. Link your personal Slack account for DM delivery under **Connected Accounts**

See [Notification Preferences](/on-call/notification-preferences) for details on all available channels.

## 12. Secure Your Instance

For production deployments:

- Set `ENVIRONMENT=production`
- Configure `ENCRYPTION_KEYS` and `SECRET_PEPPER`
- Enable `SECURE_COOKIES=true` with HTTPS
- Complete the first-run setup wizard to create the initial admin account before enabling production
- See [Security & Authentication](/configuration/security) for details
