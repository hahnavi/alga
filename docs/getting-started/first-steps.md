---
title: First Steps Guide
description: A 12-step walkthrough from creating the initial admin account through sending test alerts, connecting Grafana, and securing your instance.
---

# First Steps Guide

Alga is running. Here's what to do next, step by step.

## 1. Create your admin account

Open `http://localhost:3000` in your browser. Since this is the first time, you'll see a setup screen. Enter your email, full name, and a password — that's your admin account. You'll only see this screen once.

Tip: if you turned on Google, Slack, or company login (SSO), you'll also see those buttons on the login page later.

## 2. Create a webhook token

You need a token (a secret password for machines) before your tools can send alerts to Alga.

1. In the left sidebar, click **Incoming Webhooks**
2. Create a new token and give it a name like "Grafana"
3. Copy the token — you'll use it in the next step

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

You should see the alert appear on the **Alerts** page. (The token comes from the **Incoming Webhooks** page and goes in the `Authorization: Bearer` line.)

## 4. Connect Grafana

1. In Alga, copy your token from the **Incoming Webhooks** page
2. In Grafana, go to **Alerting → Contact points → Add contact point**
3. Set type to **Webhook**
4. Set URL to `http://your-alga-host:8080/webhooks/alerts`
5. Add an HTTP header `Authorization` = `Bearer alga_YOUR_TOKEN`
   (in Grafana you'll find this under _Optional Webhook settings → HTTP Headers_)
6. Save and send a test

## 5. Decide where alerts go

Go to **Settings → Routes** (click Settings in the sidebar, then Routes) to decide which alerts go where:

1. Click **Add Rule**
2. Pick which alerts it applies to (for example, `namespace = production`)
3. Pick where they go (Slack, Mattermost, and more)
4. Save

Alerts that don't match any rule go to the default channel.

## 6. Write down what you know

**Knowledge** notes are your team's shared cheat sheets — what to do when an alert fires.

1. In the sidebar, click **Knowledge**
2. Write a note with tags matching your alerts (for example, `alertname: HighMemory`)
3. The AI helpers and your teammates will see these notes when that alert fires

Writing down fixes once means faster fixes every time. For step-by-step checklists, see [Playbooks](/core-features/playbooks).

## 7. Look around

| What you want to see | Where to click      | What you can do                |
| -------------------- | ------------------- | ------------------------------ |
| **Alerts**           | Alerts page         | See, confirm, and close alerts |
| **Investigations**   | Investigations page | Read AI findings               |
| **Knowledge**        | Knowledge page      | Shared team notes              |
| **Routes**           | Settings → Routes   | Decide where alerts go         |

## 8. Set up incident response

1. Add your **Services** (the parts of your system you care about)
2. Note which services depend on each other
3. Create **Teams** and add your people
4. Set up **On-Call Schedules** so someone is always reachable
5. Create **Escalation Policies** — who gets called if the first person doesn't answer

## 9. Set up on-call and escalation

On-call schedules make sure the right person is always reachable. Escalation says what happens if they don't answer.

1. Go to **On-Call → Schedules** and create a rotation
2. Add extra layers if your team spans time zones
3. Create an **Escalation Policy** with levels and wait times
4. Link the policy to a service

See [Teams & On-Call](/on-call/) for details.

## 10. Turn on AI help (optional)

Alga comes with a built-in Alga Agent, and also works with Hermes or OpenClaw — you don't need to install anything to try the built-in one. To connect a helper:

1. Go to **Agents** in the sidebar and create an agent token
2. Connect your chosen agent (built-in Alga Agent, Hermes, or OpenClaw) using that token

See [AI Investigation](/core-features/investigation) for details.

## 11. Choose how you get notified

Everyone picks their own notification style:

1. Click your **profile picture** (top corner) → go to **Settings → Notifications**
2. Pick how you hear about each kind of alert (pop-up in Alga, email, Slack message, or phone call)
3. Pick a backup method for anything you didn't list
4. Optionally link your personal Slack account so Alga can message you directly

See [Notification Preferences](/on-call/notification-preferences) for all the options.

## 12. Lock things down

If other people will use this over the internet:

- Run it with `ENVIRONMENT=production`
- Make sure your secret passwords are set (`ENCRYPTION_KEYS` and `SECRET_PEPPER` — `setup.sh` already created these for you)
- Use `https://` with secure cookies on
- See [Security & Authentication](/configuration/security) for details
