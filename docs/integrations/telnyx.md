---
title: Telnyx Integration
description: Alternative voice-call escalation provider via Telnyx Call Control API, with provider selection and callback configuration.
---

# Telnyx Integration

Telnyx is an alternative **voice-call escalation provider** to Twilio. When an incident escalation fires, Alga calls the on-call user's phone (`user.phone`, E.164) with an IVR menu to acknowledge or silence the alert. The two providers are mutually exclusive — only the selected one is active at a time.

## Provider Selection

Voice provider selection is controlled by `VOICE_PROVIDER` (default `twilio`). Setting it to `telnyx` activates Telnyx and fully dorms Twilio. Switching providers requires a restart.

When `VOICE_PROVIDER` is unset, the provider can be chosen from the **Integrations UI** (persisted in the DB); setting the env var locks the UI selector.

## Configuration

Credentials are managed in the DB via the Integrations UI (stored encrypted), the same model as Slack/Mattermost. When `TELNYX_API_KEY` is set via env, the UI fields are locked and the env values take precedence.

| Variable                 | Default     | Required    | Description                                                                                                                                                       |
| ------------------------ | ----------- | ----------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `VOICE_PROVIDER`         | `twilio`    | No          | `twilio` or `telnyx`                                                                                                                                              |
| `TELNYX_API_KEY`         |             | No          | Telnyx API key                                                                                                                                                    |
| `TELNYX_CONNECTION_ID`   |             | No          | Telnyx Call Control Application ID                                                                                                                                |
| `TELNYX_FROM_NUMBER`     |             | No          | Telnyx outbound phone number                                                                                                                                      |
| `TELNYX_PUBLIC_KEY`      |             | Conditional | Ed25519 public key (base64) from the Telnyx portal. Required when `VOICE_PROVIDER=telnyx` — used to verify inbound webhook signatures                             |
| `TELNYX_DISABLED`        | `false`     | No          | Disable Telnyx entirely                                                                                                                                           |
| `TELNYX_TTS_VOICE`       | `KokoroTTS` | No          | TTS voice for spoken prompts. Provider-prefixed (e.g. `kokoro/af_something`, `Polly.Brian`, `Azure.en-CA-ClaraNeural`, `ElevenLabs.eleven_flash_v2_5.<voice_id>`) |
| `TELNYX_TTS_LANGUAGE`    | `en-US`     | No          | TTS language                                                                                                                                                      |
| `TELNYX_TTS_API_KEY_REF` |             | No          | Identifier of a Telnyx integration secret holding the ElevenLabs API key (BYOK). Required only when `TELNYX_TTS_VOICE` starts with `ElevenLabs.`                  |

## Callback Verification

Telnyx sends call-control webhooks to `POST /api/v1/telnyx/callback`. Verification uses **Ed25519** signatures (unlike Twilio's HMAC-SHA1):

1. Alga reads the `Ed25519` (base64 signature) and `Timestamp` headers.
2. Rejects webhooks with a timestamp more than **5 minutes** from now.
3. Verifies `ed25519.Verify(publicKey, timestamp+body, signature)` against the configured `TELNYX_PUBLIC_KEY`.

If verification fails, the callback is rejected with HTTP 403.

## Call Control API Methods

Alga uses the Telnyx REST v2 `/calls` API with these methods:

| Method             | Description                                                       |
| ------------------ | ----------------------------------------------------------------- |
| `Answer`           | Answer an inbound call                                            |
| `GatherUsingSpeak` | Speak a TTS prompt and collect DTMF input (used for the IVR menu) |
| `Speak`            | Speak a TTS message without collecting input (confirmations)      |
| `Hangup`           | End the call                                                      |

## IVR Flow

When the call is answered, Alga speaks the incident announcement and presents a menu:

> "Alga escalation. Incident {n}, escalation level {n}." _(+ incident title)_ "Press 1 to acknowledge. Press 2 to silence for one hour."

| Key          | Action                                                                                                           |
| ------------ | ---------------------------------------------------------------------------------------------------------------- |
| `1`          | **Acknowledge** — stops escalation, records a timeline entry, speaks confirmation, hangs up                      |
| `2`          | **Silence 1 hour** — suppresses escalation for 60 minutes, records timeline entry, speaks confirmation, hangs up |
| other / none | Re-prompts up to **2 attempts**, then hangs up                                                                   |

The IVR state is carried statelessly via a base64 `client_state` token (`incident:level:attempt:user`) — no server-side per-call tracking.
