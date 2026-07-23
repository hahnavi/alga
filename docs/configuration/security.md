---
title: Security & Authentication
description: Alga's security model — login, SSO (Google, Slack, OIDC), RBAC roles and permissions, session management, password recovery, and API token security.
---

# Authentication

Alga uses session-based authentication with HTTP-only cookies, CSRF protection, and role-based access control.

## Login

Authenticate via the API or the web UI:

```sh
curl -c cookies.txt -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email": "admin@alga.local", "password": "your-password"}'
```

The response sets two cookies:
- `alga_session` — HTTP-only session cookie
- `alga_csrf` — CSRF token for state-changing requests

## Google Sign-In

Alga supports Google Sign-In via standard OAuth 2.0. When configured, users can authenticate with their Google account instead of email/password.

### Configuration

Set the following environment variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `GOOGLE_CLIENT_ID` | Yes | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | Yes | Google OAuth client secret |
| `GOOGLE_OAUTH_REDIRECT_URL` | No | Override callback URL (auto-detected from request headers if not set) |

When `GOOGLE_CLIENT_ID` is set, the login page displays a "Sign in with Google" button.

### API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/auth/google/enabled` | None | Check if Google OAuth is enabled |
| `GET` | `/api/v1/auth/google` | None | Redirect to Google OAuth authorization |
| `GET` | `/api/v1/auth/google/callback` | None | Google OAuth callback |

### Flow

1. User clicks "Sign in with Google" on the login page
2. Browser redirects to Google's authorization page
3. User authorizes the application
4. Google redirects back to Alga's callback URL
5. Alga creates or finds the user account automatically and establishes a session

## Slack Sign-In

Users can authenticate by linking their Slack identity, configured from **Settings → Integrations** per user (`/api/v1/users/me/slack/*`). A workspace-level Slack app is a prerequisite. The endpoints are `/api/v1/auth/slack/enabled`, `/api/v1/auth/slack`, and `/api/v1/auth/slack/callback`.

## OIDC SSO

Alga supports multiple generic OIDC identity providers (e.g. Okta, Keycloak, Google, Auth0) configured from **System → Authentication**. See [OIDC SSO](/integrations/oidc-sso) for setup.

## Password Recovery

Alga provides a self-service password reset flow via email.

### Prerequisites

- `SMTP_HOST` must be configured (see [Email Configuration](/integrations/email))
- `SECRET_PEPPER` must be set for HMAC token hashing

### API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/api/v1/auth/forgot-password` | None | Request password reset email |
| `POST` | `/api/v1/auth/reset-password` | None | Reset password with token |

### Flow

1. User submits their email to `/api/v1/auth/forgot-password`
2. If the email exists, Alga sends a reset link with a time-limited token (valid 1 hour)
3. The response is always the same message regardless of whether the email exists — this prevents email enumeration
4. User clicks the link, which opens `/reset-password?token=...` in the frontend
5. Frontend submits the token and new password to `/api/v1/auth/reset-password`
6. Alga validates the token (not expired, not used), checks the password against the [password policy](#password-policy), and updates the password

### Request Password Reset

```sh
curl -X POST http://localhost:8080/api/v1/auth/forgot-password \
  -H "Content-Type: application/json" \
  -d '{"email": "user@example.com"}'
```

### Reset Password

```sh
curl -X POST http://localhost:8080/api/v1/auth/reset-password \
  -H "Content-Type: application/json" \
  -d '{"token": "reset-token-from-email", "new_password": "NewP@ssw0rd!"}'
```

## Roles & Permissions

Alga has three built-in roles. Every authenticated request is checked against the route's required permission.

### Admin

Full access to all features and every permission, including destructive and administrative scopes (deletes, system config, user/token/OIDC management, credential management).

### Operator

Day-to-day operations: read/write for operational domains (alerts, knowledge, memories, incidents, triage, post-mortems, playbooks, heartbeats, status pages), incident command, notification management, and credential management. **No** system config, no deletes (except knowledge/memories), no token/OIDC management, no `admin:access`.

### Viewer

Read-only access across all operational resources.

### Permission Matrix

| Permission | Admin | Operator | Viewer |
|------------|:-----:|:--------:|:------:|
| `alerts:read` / `alerts:write` | ✅ | ✅ | 👁️ / — |
| `alerts:delete` | ✅ | — | — |
| `knowledge:read` / `knowledge:write` | ✅ | ✅ | 👁️ / — |
| `knowledge:delete` | ✅ | — | — |
| `memories:read` / `memories:write` | ✅ | ✅ | 👁️ / — |
| `memories:delete` | ✅ | — | — |
| `routes:read` | ✅ | ✅ | 👁️ |
| `routes:write` | ✅ | — | — |
| `integrations:read` | ✅ | ✅ | 👁️ |
| `integrations:write` | ✅ | — | — |
| `integrations:test` | ✅ | ✅ | — |
| `users:manage` | ✅ | ✅ | — |
| `tokens:manage` | ✅ | — | — |
| `dashboard:read` | ✅ | ✅ | 👁️ |
| `channels:read` | ✅ | ✅ | 👁️ |
| `audit:read` | ✅ | ✅ | — |
| `notifications:read` | ✅ | ✅ | 👁️ |
| `notifications:write` | ✅ | ✅ | 👁️ |
| `system:read` / `system:write` | ✅ | — | — |
| `triage:read` / `triage:write` | ✅ | ✅ | 👁️ / — |
| `triage:override` | ✅ | ✅ | — |
| `incidents:read` / `incidents:write` | ✅ | ✅ | 👁️ / — |
| `incidents:command` | ✅ | ✅ | — |
| `incidents:delete` | ✅ | — | — |
| `services:read` | ✅ | ✅ | 👁️ |
| `services:write` | ✅ | — | — |
| `oncall:read` | ✅ | ✅ | 👁️ |
| `oncall:write` | ✅ | — | — |
| `escalation:read` | ✅ | ✅ | 👁️ |
| `escalation:write` | ✅ | — | — |
| `postmortems:read` / `postmortems:write` | ✅ | ✅ | 👁️ / — |
| `postmortems:delete` | ✅ | — | — |
| `playbooks:read` / `playbooks:write` | ✅ | ✅ | 👁️ / — |
| `playbooks:delete` | ✅ | — | — |
| `heartbeats:read` / `heartbeats:write` | ✅ | ✅ | 👁️ / — |
| `heartbeats:delete` | ✅ | — | — |
| `statuspages:read` / `statuspages:write` | ✅ | ✅ | 👁️ / — |
| `statuspages:delete` | ✅ | — | — |
| `oidc:manage` | ✅ | — | — |
| `credentials:read` | ✅ | ✅ | — |
| `credentials:manage` | ✅ | ✅ | — |
| `admin:access` | ✅ | — | — |

## SSO Providers

In addition to Google Sign-In, Alga supports generic OIDC SSO and Slack Sign-In. See [OIDC SSO](/integrations/oidc-sso) for configuring multiple identity providers.

## Password Policy

Alga enforces the following password requirements:
- Minimum 8 characters
- At least one uppercase letter
- At least one lowercase letter
- At least one digit
- At least one special character

Passwords are hashed with Argon2id (OWASP 2026 baseline: 64 MiB memory, 3 iterations, parallelism 2) with a server-side HMAC pepper pre-hash.

## Session Management

- Sessions expire after `SESSION_EXPIRY_HOURS` (default: 24 hours)
- Refresh tokens enable seamless session renewal
- Refresh token reuse detection revokes the entire session family
- When `SECURE_COOKIES=true`, cookies are only sent over HTTPS

## API Tokens

### Webhook Tokens

Bearer tokens for alert ingestion endpoints:

```sh
curl -X POST http://localhost:8080/webhooks/alerts \
  -H "Authorization: Bearer alga_..." \
  -H "Content-Type: application/json" \
  -d '{"alerts": [...]}'
```

### Agent Tokens

Bearer tokens for AI agent API and SSE:

```sh
# SSE connection
curl -N http://localhost:8080/api/v1/agent/events \
  -H "Authorization: Bearer alga_agent_..."

# REST API
curl http://localhost:8080/api/v1/agent/alerts \
  -H "Authorization: Bearer alga_agent_..."
```

### Personal Access Tokens

Personal Access Tokens (PATs) allow users to authenticate API requests for automation and scripting without using session cookies:

- **Token format:** `alga_pat_...`
- **CSRF bypass:** PATs are intended for machine-to-machine communication and bypass CSRF validation
- **Permission model:** A PAT's permissions intersect with the owning user's role — a PAT cannot grant access beyond what the user already has
- **Management:** Create, list, and revoke PATs from the user profile settings

See [Personal Access Tokens](/operations/personal-access-tokens) for details.

## Security Features

- **CSRF Protection** — Double-submit cookie pattern (`alga_csrf` + `X-CSRF-Token` header)
- **Secure Cookies** — HttpOnly, Secure (HTTPS), SameSite=Strict
- **Password Hashing** — Argon2id with server-side HMAC pepper pre-hash
- **Encryption at Rest** — AES-256-GCM for integration secrets
- **Constant-Time Comparison** — All secret checks use `crypto/subtle`
- **Rate Limiting** — Login attempts limited to 5 per 15 minutes, per-IP request rate limiting
- **Account Lockout** — 30-minute lockout after 5 failed attempts
- **Audit Logging** — All auth, alert, investigation, knowledge, and peer-ask events recorded in PostgreSQL
- **Session Secrets** — Cookie-side session IDs and refresh tokens are HMAC-SHA-256 hashed before persistence
- **Refresh Token Rotation** — Atomic rotation with reuse detection via family tracking
- **Bearer Token Storage** — Tokens stored as HMAC-SHA-256 + non-secret `lookup_prefix`
- **Production Fail-Closed** — Refuses to start without `ENCRYPTION_KEYS` (or `ENCRYPTION_KEY`) AND `SECRET_PEPPER` in **every** environment, not only production. HSTS is emitted on HTTPS regardless of the `SecureCookies` flag
- **SSO** — Google Sign-In, Slack Sign-In, and multi-provider OIDC SSO (see [OIDC SSO](/integrations/oidc-sso))
