---
title: Automation
description: Run audited actions when alerts and incidents change.
---

# Automation

Automation rules use **When → If → Then**: choose an alert or incident event, add optional conditions, and run up to five actions in order. Rules start disabled so you can review them before they act. Playbooks remain guidance for investigators; their suggested commands are never executed by Automation.

## Triggers and conditions

The first release supports alert created, acknowledged, resolved, and reopened, plus incident created and status changed. `alert.created` means a new alert record with a new `alert_number`; another firing payload for an already-open fingerprint does not create another alert. Silenced alerts do not execute rules.

All conditions must match. Fields include `status`, `previous_status`, `severity`, `priority`, `service_id`, `silenced`, and alert labels such as `label.alertname` or `label.namespace`. Operators can compare equality, inequality, substring containment, or existence. A rule without conditions matches every event of its trigger type.

## Actions

- **Add incident note** appends a system-attributed timeline note for an incident event.
- **Send HTTPS request** POSTs a JSON envelope to an approved external service, such as n8n. An optional bearer token is encrypted at rest and is never returned by the API. Editing a webhook URL requires re-entering its token; leaving the URL unchanged and the token field blank keeps the existing token. You can explicitly remove it in the form.

Set `AUTOMATION_WEBHOOK_ALLOWED_HOSTS` to a comma-separated list of exact DNS names before enabling outbound rules. If an approved service resolves to an RFC1918 or ULA address, add its exact DNS name to `AUTOMATION_WEBHOOK_PRIVATE_HOSTS` too. Loopback, link-local, metadata, and other unsafe addresses remain blocked. HTTP URLs, redirects, embedded URL credentials, and IP literals are not accepted.

Each request contains `event_id`, `run_id`, `rule_id`, `action_index`, and an `event` object with `schema_version: 1` and the captured entity number, status, severity, and relevant labels or service ID. The `Idempotency-Key` header is stable for a given run and action. A receiver should persist that key before performing non-repeatable work: a timeout or worker crash can cause Alga to resend a request that the receiver already processed.

## Runs and failures

The **Runs** tab records action attempts, HTTP status, and failure codes. Network errors, HTTP 429, and HTTP 5xx retry with bounded backoff; other HTTP 4xx responses fail the run. An operator can retry a failed run from its failed action. Completed actions are not replayed. Rules are versioned for execution: editing a rule changes future runs while existing runs continue with their captured definition.

Rule management requires `automation:write`; viewing rules and runs requires `automation:read`. Deleting a rule archives and disables it so historical runs remain inspectable. Webhook URLs can contain opaque tokens, so Automation access is limited to operators and admins.
