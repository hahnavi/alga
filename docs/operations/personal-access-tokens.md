---
title: Personal Access Tokens
description: Scoped personal access tokens for CLI and API access — creation, format, usage, and security best practices.
---

# Personal Access Tokens

Personal Access Tokens (PATs) provide an alternative to session-based authentication for API automation, scripting, and integrations.

## Overview

PATs are long-lived bearer tokens that authenticate as a specific user. The effective permissions are the intersection of the PAT's assigned permissions and the user's role permissions (least privilege).

| Feature       | Session Auth            | PAT Auth                       |
| ------------- | ----------------------- | ------------------------------ |
| Lifetime      | 24 hours (configurable) | Until expiry or revocation     |
| CSRF Required | Yes                     | No                             |
| Use Case      | Browser sessions        | API automation, scripts, CI/CD |
| Storage       | HTTP-only cookie        | Bearer token header            |

## Creating a PAT

### Via UI

1. Go to **Personal Access Tokens** in the sidebar
2. Click **Create Token**
3. Specify a name, permissions, and optional expiry date
4. Save the displayed token — it will not be shown again

### Via API

```sh
curl -X POST http://localhost:8080/api/v1/user/tokens \
  -H "Authorization: Bearer $SESSION" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "CI/CD Pipeline",
    "permissions": ["alerts:read", "alerts:write"],
    "expires_at": "2026-12-31T23:59:59Z"
  }'
```

Response includes the plaintext token (shown once):

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "CI/CD Pipeline",
  "token": "alga_pat_a1b2c3d4e5f6...",
  "permissions": ["alerts:read", "alerts:write"],
  "expires_at": "2026-12-31T23:59:59Z"
}
```

## Token Format

PATs use the prefix `alga_pat_` followed by a cryptographically random secret. Like all Alga tokens, only the HMAC-SHA-256 hash is stored in the database. The plaintext is shown exactly once at creation time.

The non-secret `lookup_prefix` (the first 12 hex characters of the SHA-256 digest of the token) is used for indexed database lookups, followed by constant-time comparison of the full HMAC hash.

## Using PATs

Include the token in the `Authorization` header:

```sh
curl -H "Authorization: Bearer alga_pat_..." \
  http://localhost:8080/api/v1/alerts
```

PATs bypass CSRF requirements since they are intended for machine-to-machine communication. All other authorization checks (RBAC permissions, user-scoped data) still apply.

### CI/CD Example

```sh
export ALGA_PAT="alga_pat_..."

curl -H "Authorization: Bearer $ALGA_PAT" \
  http://localhost:8080/api/v1/alerts?status=firing&limit=5
```

## API Endpoints

### User Tokens

Manage your own personal access tokens:

| Method   | Path                       | Auth    | Description                              |
| -------- | -------------------------- | ------- | ---------------------------------------- |
| `GET`    | `/api/v1/user/tokens`      | Session | List your PATs (token values are masked) |
| `POST`   | `/api/v1/user/tokens`      | Session | Create a new PAT                         |
| `DELETE` | `/api/v1/user/tokens/{id}` | Session | Revoke one of your PATs                  |

### Admin Token Management

Administrators can view and revoke any user's PATs:

| Method   | Path                        | Auth    | Description                    |
| -------- | --------------------------- | ------- | ------------------------------ |
| `GET`    | `/api/v1/admin/tokens`      | Session | List all PATs across all users |
| `DELETE` | `/api/v1/admin/tokens/{id}` | Session | Revoke any PAT                 |

## Security

PATs follow the same security model as all Alga tokens:

- **HMAC-SHA-256 hashing** with `SECRET_PEPPER` — plaintext tokens are never stored
- **Non-secret `lookup_prefix`** for indexed lookups without exposing the full token
- **Constant-time comparison** (`crypto/subtle.ConstantTimeCompare`) on every validation
- **Permission intersection** — effective permissions are PAT permissions AND user role permissions (least privilege)
- **Expiry dates** — set optional expiry dates to limit token lifetime
- **Revocation** — tokens can be revoked at any time by the owner or an admin
- **Audit logging** — all PAT creation and revocation events are logged

### Production Requirements

In production environments, `SECRET_PEPPER` and `ENCRYPTION_KEYS` must be configured. Tokens cannot be validated without the pepper, ensuring that a database leak alone is insufficient to use stored tokens.

## See Also

- [Security & Authentication](/configuration/security) — session auth, RBAC, and security model
- [Environment Variables](/configuration/environment-variables) — `SECRET_PEPPER` and encryption key configuration
