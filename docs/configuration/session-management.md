---
title: Session Management
description: Self-service session listing and revocation — view active sessions, revoke individual sessions, and revoke all other sessions.
---

# Session Management

Alga gives every authenticated user self-service control over their active browser sessions. Sessions are stored in Valkey (preferred) or Postgres (fallback) and all three endpoints are cookie-session only — **Personal Access Tokens receive `400`** (see [Security & Authentication](/configuration/security)).

All three routes require an authenticated session cookie and the standard CSRF check (`X-CSRF-Token` header matching the `alga_csrf` cookie). Mutations are audited as `session_revoked` / `sessions_revoked_all` (fire-and-forget, never blocking the response).

## Endpoints

| Method   | Path                         | Purpose                                                        |
| -------- | ---------------------------- | -------------------------------------------------------------- |
| `GET`    | `/api/v1/auth/sessions`      | List the caller's active sessions                              |
| `DELETE` | `/api/v1/auth/sessions/{id}` | Revoke one of the caller's sessions by its HMAC digest ID      |
| `DELETE` | `/api/v1/auth/sessions`      | Revoke all of the caller's sessions **except** the current one |

`{id}` is the session's **HMAC digest** (hex), not the raw cookie value — the plaintext session ID is never exposed by the API.

## List sessions

```sh
curl -b cookies.txt -c cookies.txt \
  http://localhost:8080/api/v1/auth/sessions \
  -H "X-CSRF-Token: $CSRF"
```

```json
{
  "items": [
    {
      "id": "a3f1…hex-digest…",
      "created_at": "2026-08-29T09:12:00Z",
      "last_used_at": "2026-08-30T06:44:00Z",
      "expires_at": "2026-08-31T09:12:00Z",
      "ip": "203.0.113.42",
      "user_agent": "Mozilla/5.0 …",
      "current": true
    }
  ]
}
```

Fields:

- `id` — HMAC-SHA-256 digest of the session token (safe to display, not reversible).
- `created_at` / `last_used_at` / `expires_at` — RFC 3339 timestamps.
- `ip` / `user_agent` — as seen on the last request that touched the session.
- `current` — exactly one entry in the list has `current: true` (the session that made the request). Sorted most-recently-used first.

Expired sessions are filtered out on both the Valkey and Postgres backends. Behaviour is identical regardless of which store is active — the store is selected at boot (`Valkey` when reachable, otherwise `Postgres`); there is no runtime switching.

## Revoke a single session

```sh
curl -X DELETE -b cookies.txt -c cookies.txt \
  http://localhost:8080/api/v1/auth/sessions/a3f1…hex-digest… \
  -H "X-CSRF-Token: $CSRF"
```

| Outcome               | Status | Meaning                                                                                                          |
| --------------------- | ------ | ---------------------------------------------------------------------------------------------------------------- |
| Revoked               | `204`  | The target session was deleted.                                                                                  |
| Own current session   | `400`  | `"cannot revoke current session; sign out instead"` — use `POST /api/v1/auth/logout` to end the current session. |
| Foreign or unknown ID | `404`  | No session oracle — missing and foreign IDs both return 404 to avoid enumeration.                                |
| PAT caller            | `400`  | Session management is not available for Personal Access Tokens; use token revocation instead.                    |

## Revoke all other sessions

```sh
curl -X DELETE -b cookies.txt -c cookies.txt \
  http://localhost:8080/api/v1/auth/sessions \
  -H "X-CSRF-Token: $CSRF"
```

```json
{ "revoked": 2 }
```

Deletes every session belonging to the caller **except** the current one, returns the count. Useful after a password change or when a device is lost.

## Frontend

The Settings → **Security** tab (`/settings/security`) renders this list with device/IP, last-used time, and a "Current" badge. Each row has a **Revoke** action (disabled for the current session); a **Revoke all others** button appears when more than one session exists. All actions show success/error toasts and are gated behind CSRF.

## See also

- [Security & Authentication](/configuration/security) — session lifecycle, refresh-token rotation, replay detection, HMAC storage, fail-closed crypto, HSTS, and RBAC.
- [Personal Access Tokens](/operations/personal-access-tokens) — PAT lifecycle (PATs cannot use session management endpoints).
- [System Configuration API](/configuration/system-config) — `SESSION_EXPIRY_HOURS` / `SESSION_MAX_LIFETIME` tuning.
