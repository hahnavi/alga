---
title: Twilio Integration
description: Voice-call escalation via Twilio — IVR menu to acknowledge or silence alerts, callback verification, and security.
---

# Twilio Integration

Alga integrates with Twilio for voice call escalation paging. When an escalation policy fires, Alga resolves the current on-call human from the on-call schedule and pages that person's phone number with an interactive voice call (IVR).

## Prerequisites

- A Twilio account with a phone number that supports voice calls.
- Users who should receive pages must have a valid E.164 phone number set in their Alga profile (`user.phone`). On-call schedule members already require a phone.
- An on-call schedule with at least one phone-required member, so an escalation can resolve a recipient.
- An escalation policy with `voice` in its level `notify_channels`, and/or users selecting the `voice` channel in their notification preferences for `incident_escalation` (and other) notification types.
- A publicly reachable Alga base URL so Twilio can reach the IVR callback endpoint.

## Configuration

Twilio is configured from the **Integrations** page in the Alga UI (the Twilio card). Credentials are stored encrypted in the `integrations` table — the same model used by Slack and Mattermost.

Environment variables remain available as a bootstrap fallback and override. When `TWILIO_ACCOUNT_SID` is set in the environment, the UI fields are locked and the env values take precedence. This lets you seed the integration before the DB is populated, or enforce a fixed configuration in managed deployments.

### Environment Variables

| Variable             | Default | Description                                                                                             |
| -------------------- | ------- | ------------------------------------------------------------------------------------------------------- |
| `TWILIO_ACCOUNT_SID` |         | Twilio account SID. When set, the UI fields are locked and env values override the DB.                  |
| `TWILIO_AUTH_TOKEN`  |         | Twilio auth token. Also used to validate callback signatures. Stored encrypted when set via the UI.     |
| `TWILIO_FROM_NUMBER` |         | The Twilio number placing calls (E.164).                                                                |
| `TWILIO_DISABLED`    | `false` | Set to `true` to disable Twilio entirely. Also togglable from the UI via the `twilio_disabled` DB flag. |

## How It Works

1. An escalation policy fires for an incident.
2. Alga resolves the current on-call human from the on-call schedule.
3. Alga places an outbound voice call via the Twilio REST API (`Calls.json`) to that user's phone number (`user.phone`).
4. The call uses TwiML with a `Gather` verb to collect DTMF input (acknowledge or silence).

### Voice (IVR)

A voice call announces the incident number and the escalation level, then presents an interactive menu:

- **Press 1** — Acknowledge the incident. This sets the escalation state to acknowledged in Valkey, removes the incident from the escalation sweep sorted set, and adds an incident timeline entry. Acknowledging stops all further escalation across every level.
- **Press 2** — Silence escalation paging for one hour. The incident remains open, but no further pages are sent during the silence window.

## Callback

Twilio posts IVR key presses (DTMF responses) to:

```
POST /api/v1/twilio/callback?incident=<n>
```

DTMF responses map to incident actions:

- `1` — Acknowledge the incident (stops escalation)
- `2` — Silence the incident (suppresses paging for one hour)

- The endpoint validates the `X-Twilio-Signature` HMAC-SHA1 header against the configured auth token using a constant-time comparison. Requests with a missing or invalid signature are rejected with `403`.
- The `incident` query parameter identifies which incident the call relates to, so the acknowledge or silence action is applied to the correct incident.
- The callback URL must be publicly reachable by Twilio. Configure your Alga public base URL so Twilio can reach this endpoint.
- Call lifecycle events (ringing, answered, completed) are delivered to a `StatusCallback` URL on the same host.

## Security

- The auth token is stored encrypted in the `integrations` table, or supplied via `TWILIO_AUTH_TOKEN` for bootstrap/override.
- All callbacks are verified with `X-Twilio-Signature` before any state change is applied.
- The API never returns the auth token or token hashes.

## Disabling

Disable Twilio from the **Integrations** page (the `twilio_disabled` DB flag) or by setting `TWILIO_DISABLED=true`. When disabled, no voice pages are sent even if an escalation policy requests the `voice` channel.
