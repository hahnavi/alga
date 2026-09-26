---
title: Playbooks
description: Write your response checklist once — Alga shows it to the AI helper automatically.
---

# Playbooks

A playbook is a checklist you write once for a recurring problem — like "database connection pool full" — so the AI helper follows your proven steps every time.

## How it works

1. You write the checklist: title, when it applies, and the steps in order.
2. When a matching alert is investigated, Alga automatically shows your checklist to the AI helper along with the alert.
3. The helper uses it as guidance while it investigates.

## Creating a playbook

1. Go to **Playbooks → New Playbook**.
2. Give it a clear title like "Database connection pool exhaustion."
3. Say when it applies — for example, "when the alert name is DBConnectionPoolExhausted" or "when namespace looks like prod-*."
4. Add steps in order, each with a short title and instructions. You can add how long each step usually takes.

A playbook can match when all your conditions match, or when any one of them matches — you choose when you create it.

## Example

**Title:** Database connection pool exhaustion

1. Check active connections
2. Find queries running longer than 60 seconds
3. Stop queries that are blocking others

Next time that alert fires, the helper sees these steps automatically.

## See also

- [AI Investigation](/core-features/investigation) — how helpers use your playbooks
- [Incident Lifecycle](/incident-management/lifecycle) — what happens when an alert becomes an incident
