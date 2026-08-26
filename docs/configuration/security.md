---
title: Security & Authentication
description: Alga's security model — login, SSO (Google, Slack, OIDC), RBAC roles and permissions, session management, password recovery, and API token security.
---

# Authentication

Alga uses session-based authentication with HTTP-only cookies, CSRF protection, and role-based access control.

## Authentication Models

Alga supports three authentication models depending on the caller:

| Model                 | Mechanism                             | Use Case              |
| --------------------- | ------------------------------------- | --------------------- |
| Session cookie        | `alga_session` cookie + CSRF token    | Browser / web UI      |
| Personal Access Token | `Authorization: Bearer alga_pat_...`  | Automation, scripting |
| Agent token           | `Authorization: Bearer <agent-token>` | AI agent API and SSE  |

### Session Authentication

The web UI authenticates via session cookies. State-changing methods (POST, PUT, PATCH, DELETE) require a CSRF token in the `X-CSRF-Token` header that matches the `alga_csrf` cookie (double-submit cookie pattern).

### Personal Access Tokens (PAT)

PATs use `Bearer alga_pat_...` and bypass CSRF validation (intended for machine-to-machine use). A PAT's effective permissions are the intersection of the PAT's granted scopes and the owning user's role permissions — a PAT can never grant access beyond what the user already has.

### Agent Tokens

Agent tokens use `Bearer <agent-token>` with capability checks. Agent routes are protected by `agentBearerMiddleware` with per-agent-token rate limiting and do not rely on CSRF cookies.

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

Set the following environment variables (or configure at runtime via [System Configuration API](/configuration/system-config)):

| Variable                    | Required | Description                                                           |
| --------------------------- | -------- | --------------------------------------------------------------------- |
| `GOOGLE_CLIENT_ID`          | Yes      | Google OAuth client ID                                                |
| `GOOGLE_CLIENT_SECRET`      | Yes      | Google OAuth client secret                                            |
| `GOOGLE_OAUTH_REDIRECT_URL` | No       | Override callback URL (auto-detected from request headers if not set) |

When `GOOGLE_CLIENT_ID` is set, the login page displays a "Sign in with Google" button.

### API Endpoints

| Method | Path                           | Auth | Description                            |
| ------ | ------------------------------ | ---- | -------------------------------------- |
| `GET`  | `/api/v1/auth/google/enabled`  | None | Check if Google OAuth is enabled       |
| `GET`  | `/api/v1/auth/google`          | None | Redirect to Google OAuth authorization |
| `GET`  | `/api/v1/auth/google/callback` | None | Google OAuth callback                  |

### Flow

1. User clicks "Sign in with Google" on the login page
2. Browser redirects to Google's authorization page
3. User authorizes the application
4. Google redirects back to Alga's callback URL
5. Alga creates or finds the user account automatically and establishes a session

## Slack Sign-In

Users can authenticate by linking their Slack identity, configured from **Settings > Integrations** per user (`/api/v1/users/me/slack/*`). A workspace-level Slack app is a prerequisite. The endpoints are `/api/v1/auth/slack/enabled`, `/api/v1/auth/slack`, and `/api/v1/auth/slack/callback`.

## OIDC SSO

Alga supports generic OIDC identity providers (e.g. Okta, Keycloak, Google, Auth0) using the Authorization Code flow with PKCE. Login state (including the PKCE verifier and provider ID) is stored in Valkey as a single-use record with a 10-minute TTL, preventing replay of authorization codes.

Configure from **System > Authentication** or via the [System Configuration API](/configuration/system-config). See [OIDC SSO](/integrations/oidc-sso) for setup.

## Password Recovery

Alga provides a self-service password reset flow via email.

### Prerequisites

- `SMTP_HOST` must be configured (see [Email Configuration](/integrations/email))
- `SECRET_PEPPER` must be set for HMAC token hashing

### API Endpoints

| Method | Path                           | Auth | Description                  |
| ------ | ------------------------------ | ---- | ---------------------------- |
| `POST` | `/api/v1/auth/forgot-password` | None | Request password reset email |
| `POST` | `/api/v1/auth/reset-password`  | None | Reset password with token    |

### Flow

1. User submits their email to `/api/v1/auth/forgot-password`
2. If the email exists, Alga sends a reset link with a time-limited token (valid 1 hour)
3. The response is always the same message regardless of whether the email exists — this prevents email enumeration
4. User clicks the link, which opens `/reset-password?token=...` in the frontend
5. Frontend submits the token and new password to `/api/v1/auth/reset-password`
6. Alga validates the token (not expired, not used), checks the password against the [password policy](#password-policy), and updates the password

Reset tokens are hashed before storage and expire after use.

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

## Account Lockout

Failed login attempts are tracked per account (`failed_login_attempts`). After the threshold is reached, the account is locked (`locked_until`) for a cooldown period. Successful authentication resets the counter.

## Roles & Permissions

Alga has three built-in roles and 52 permissions. Every authenticated request is checked against the route's required permission using OR semantics — if a route accepts multiple permissions, having any one of them grants access.

### Admin

Full access to all features and every permission, including destructive and administrative scopes (deletes, system config, user/token/OIDC management, credential management).

### Operator

Day-to-day operations: read/write for operational domains (alerts, knowledge, memories, incidents, triage, post-mortems, playbooks, heartbeats, status pages), incident command, notification management, and credential management. **No** system config, no deletes (except knowledge/memories), no token/OIDC management, no `admin:access`.

### Viewer

Read-only access across all operational resources.

### Permission Matrix

| Permission                                   | Admin | Operator |  Viewer   |
| -------------------------------------------- | :---: | :------: | :-------: |
| `alerts:read` / `alerts:write`               |  Yes  |   Yes    | Read only |
| `alerts:delete`                              |  Yes  |    —     |     —     |
| `knowledge:read` / `knowledge:write`         |  Yes  |   Yes    | Read only |
| `knowledge:delete`                           |  Yes  |    —     |     —     |
| `memories:read` / `memories:write`           |  Yes  |   Yes    | Read only |
| `memories:delete`                            |  Yes  |    —     |     —     |
| `routes:read`                                |  Yes  |   Yes    |    Yes    |
| `routes:write`                               |  Yes  |    —     |     —     |
| `integrations:read`                          |  Yes  |   Yes    |    Yes    |
| `integrations:write`                         |  Yes  |    —     |     —     |
| `integrations:test`                          |  Yes  |   Yes    |     —     |
| `users:manage`                               |  Yes  |   Yes    |     —     |
| `tokens:manage`                              |  Yes  |    —     |     —     |
| `dashboard:read`                             |  Yes  |   Yes    |    Yes    |
| `channels:read`                              |  Yes  |   Yes    |    Yes    |
| `audit:read`                                 |  Yes  |   Yes    |     —     |
| `notifications:read` / `notifications:write` |  Yes  |   Yes    | Read only |
| `system:read` / `system:write`               |  Yes  |    —     |     —     |
| `triage:read` / `triage:write`               |  Yes  |   Yes    | Read only |
| `triage:override`                            |  Yes  |   Yes    |     —     |
| `incidents:read` / `incidents:write`         |  Yes  |   Yes    | Read only |
| `incidents:command`                          |  Yes  |   Yes    |     —     |
| `incidents:delete`                           |  Yes  |    —     |     —     |
| `services:read`                              |  Yes  |   Yes    |    Yes    |
| `services:write`                             |  Yes  |    —     |     —     |
| `oncall:read`                                |  Yes  |   Yes    |    Yes    |
| `oncall:write`                               |  Yes  |    —     |     —     |
| `escalation:read`                            |  Yes  |   Yes    |    Yes    |
| `escalation:write`                           |  Yes  |    —     |     —     |
| `postmortems:read` / `postmortems:write`     |  Yes  |   Yes    | Read only |
| `postmortems:delete`                         |  Yes  |    —     |     —     |
| `playbooks:read` / `playbooks:write`         |  Yes  |   Yes    | Read only |
| `playbooks:delete`                           |  Yes  |    —     |     —     |
| `heartbeats:read` / `heartbeats:write`       |  Yes  |   Yes    | Read only |
| `heartbeats:delete`                          |  Yes  |    —     |     —     |
| `statuspages:read` / `statuspages:write`     |  Yes  |   Yes    | Read only |
| `statuspages:delete`                         |  Yes  |    —     |     —     |
| `oidc:manage`                                |  Yes  |    —     |     —     |
| `credentials:read`                           |  Yes  |   Yes    |     —     |
| `credentials:manage`                         |  Yes  |   Yes    |     —     |
| `admin:access`                               |  Yes  |    —     |     —     |

## Password Policy

Alga enforces the following password requirements:

- Minimum 8 characters
- At least one uppercase letter
- At least one lowercase letter
- At least one digit
- At least one special character

Passwords are hashed with Argon2id (OWASP baseline: 64 MiB memory, 3 iterations, parallelism 2, 16-byte salt, 32-byte key) with a server-side HMAC pepper pre-hash (`SECRET_PEPPER`).

## Session Management

- Sessions expire after `SESSION_EXPIRY_HOURS` (default: 24 hours, max: 720)
- Refresh tokens enable seamless session renewal
- Refresh tokens rotate on every use; the previous token hash is recorded (`prev_refresh_token_hashes`) within a session family (`family_id`)
- Reuse of a previously-rotated refresh token is detected as replay and revokes the entire session family
- When `SECURE_COOKIES=true`, cookies are only sent over HTTPS
- Cookie-side session IDs and refresh tokens are HMAC-SHA-256 hashed before persistence

## API Tokens

### Webhook Tokens

Bearer tokens for alert ingestion endpoints. Tokens are shown exactly once on creation and stored as HMAC-SHA-256 hashes with a non-secret `lookup_prefix` for efficient lookup. Send them via the `Authorization` header — **the `?token=` query parameter is denied by default** (`WEBHOOK_ALLOW_QUERY_TOKEN=true` is a temporary escape hatch slated for removal; credentials in URLs leak into proxy/access logs, `Referer` headers and browser history):

```sh
curl -X POST http://localhost:8080/webhooks/alerts \
  -H "Authorization: Bearer alga_..." \
  -H "Content-Type: application/json" \
  -d '{"alerts": [...]}'
```

### Agent Tokens

Bearer tokens for AI agent API and SSE. The SSE endpoint also accepts `Authorization` headers (fetch-based SSE); a legacy `?token=` query fallback is denied by default (`AGENT_SSE_ALLOW_QUERY_TOKEN=true` escape hatch only):

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

## Cryptographic Primitives

| Primitive           | Algorithm     | Configuration                                                                             |
| ------------------- | ------------- | ----------------------------------------------------------------------------------------- |
| Password hashing    | Argon2id      | 64 MiB memory, time 3, parallelism 2, 16-byte salt, 32-byte key                           |
| Envelope encryption | AES-256-GCM   | `ENCRYPTION_KEYS` (comma-separated `kid:base64-key` pairs, supports key rotation via kid) |
| Token hashing       | HMAC-SHA-256  | `SECRET_PEPPER` as the HMAC key                                                           |
| Secret comparison   | Constant-time | `crypto/subtle` for all secret, token, and signature checks                               |

## Security Features

- **CSRF Protection** — Double-submit cookie pattern (`alga_csrf` cookie + `X-CSRF-Token` header) for state-changing methods
- **Secure Cookies** — HttpOnly, Secure (HTTPS), SameSite=Strict
- **Fail-Closed Startup** — Refuses to start without `ENCRYPTION_KEYS` (or `ENCRYPTION_KEY`) AND `SECRET_PEPPER` in **every** environment, not only production
- **HSTS** — `Strict-Transport-Security` emitted on all HTTPS responses regardless of the `SecureCookies` flag
- **Encryption at Rest** — AES-256-GCM envelope encryption for integration secrets and auth secrets (Google/OIDC client secrets in system config)
- **Constant-Time Comparison** — All secret checks use `crypto/subtle`
- **Rate Limiting** — Per-IP rate limiting for public endpoints; per-agent-token rate limiting for agent endpoints; login attempts limited to 5 per 15 minutes
- **Account Lockout** — Failed login tracking with time-based lockout
- **Audit Logging** — Fire-and-forget audit trail for every create, update, delete, command, and state transition; must not block request success
- **Idempotency-Key** — Optional replay protection for mutating requests (requires Valkey); first request executes, subsequent requests with the same key return the cached response
- **Bearer Token Storage** — Tokens stored as HMAC-SHA-256 hashes with non-secret `lookup_prefix` for indexed lookup; never stored in plaintext
- **Webhook Tokens** — Shown exactly once on creation; cannot be retrieved afterward
- **OIDC Security** — Authorization Code + PKCE flow; Valkey-backed single-use login state with 10-minute TTL prevents authorization code replay
- **Password Reset** — Token-based, hashed before storage, time-limited expiry
- **Session Rotation** — Refresh tokens rotate on use; family-based replay detection revokes the entire session family on reuse
- **Security Headers** — Middleware emits standard security headers on all responses
- **Trusted Proxies** — `TRUSTED_PROXIES` configures which upstream proxy IPs are trusted for `X-Forwarded-For` client IP extraction
- **SMTP TLS** — `SMTP_SKIP_TLS_VERIFY=true` disables TLS certificate verification for outgoing email; logs a loud warning at startup due to MITM risk. Use only for testing.
- **SSO** — Google Sign-In, Slack Sign-In, and multi-provider OIDC SSO (see [OIDC SSO](/integrations/oidc-sso))
