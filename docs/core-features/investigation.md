---
title: AI Investigation
description: How Alga's AI helpers look into alerts for you.
---

# AI Investigation

When an alert needs a closer look, Alga hands it to an AI helper that digs in and reports back.

## How it works

1. An alert fires and passes triage.
2. Alga picks the best available AI helper — it prefers specialists (for example, a database helper for database alerts) over a general helper.
3. The helper gets the alert details, your notes, and any matching checklist (playbook).
4. You see its findings and updates in the investigation thread on the alert or incident page.
5. The helper can resolve the alert, or suggest turning it into an incident if it's bigger than it first looked.

If a helper drops offline in the middle of a job, Alga automatically puts the job back in the queue so another available helper can pick it up.

## What you see

- **Status:** waiting, working, done, or needs attention.
- **Thread:** back-and-forth between you and the helper, updating live.
- **Outcome:** what the helper found and what it did.

You can watch, reply in the thread to add context, or reassign the job to a different helper.

## Setting up a helper

1. Go to **Agents** in the sidebar and create a new agent.
2. Give it a name, pick what it is allowed to do, and pick which alerts it should handle (everything, or just certain teams or services).
3. Save and copy the token right away — it is shown once.
4. Start your helper app with that token so it shows as online.

You can run more than one helper. For example, one specialist for databases and one general helper for everything else. Alga will route each job to the best match that is online.

## See also

- [Triage](/core-features/triage) — how Alga decides what deserves an investigation
- [Playbooks](/core-features/playbooks) — checklists your helpers follow automatically
