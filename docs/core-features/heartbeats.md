---
title: Heartbeats
description: Get alerted when a scheduled job stops checking in.
---

# Heartbeats

A heartbeat means "ping me every X minutes or raise an alert." Use it for cron jobs, scheduled tasks, or anything that should check in regularly — so you hear about it when it goes quiet.

## How it works

1. You create a heartbeat and tell Alga how often to expect a ping — for example, every 5 minutes, with a little extra grace time.
2. Alga gives you a private ping address. Add it to your job so it pings Alga on every run.
3. As long as pings keep arriving, everything shows as healthy.
4. If Alga doesn't hear from the job in time, it raises an alert that flows through your normal routing and notifications.
5. The next ping clears the alert automatically.

## Creating a heartbeat

1. Go to **Heartbeats → New Heartbeat**.
2. Give it a name like "Nightly backup."
3. Set how often it should ping and how much grace time to allow.
4. Pick a severity for the alert it raises.
5. Save and copy the ping token right away — it is shown once, so keep it secret like a password.

If you lose the token, come back and regenerate it to get a new one.

## Tips

- Pick intervals that match your job's real schedule, plus a minute or two of grace for slow runs.
- Give each job its own heartbeat so you know exactly which one went quiet.
- You can pause a heartbeat without deleting it when a job is intentionally stopped.
