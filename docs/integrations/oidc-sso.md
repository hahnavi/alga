---
title: OIDC SSO
description: Single sign-on via Okta, Keycloak, Google, Auth0, and other OIDC identity providers — single and multi-provider configuration.
---

# OIDC SSO

Alga supports **multiple OIDC identity providers** (e.g. Okta, Keycloak, Google, Auth0) for single sign-on. Users authenticate against their IdP; Alga verifies the ID token and links the OIDC identity to a local user account.

## Quick Config (Single Provider)

For a single provider, set these environment variables (also editable via **System → Authentication**):

| Variable | Default | Description |
|----------|---------|-------------|
| `OIDC_ENABLED` | `false` | Enable OIDC SSO |
| `OIDC_ISSUER_URL` | | OIDC issuer URL |
| `OIDC_CLIENT_ID` | | OIDC client ID |
| `OIDC_CLIENT_SECRET` | | OIDC client secret |
| `OIDC_SCOPES` | `openid email profile` | OIDC scopes |

## Multi-Provider Management

Multiple providers are managed from **System → Authentication** (requires the `oidc:manage` permission).

### Provider Fields

| Field | Description |
|-------|-------------|
| `name` | Display name shown on the login page |
| `issuer` | OIDC issuer URL (must serve `.well-known/openid-configuration`) |
| `client_id` | OAuth/OIDC client ID |
| `client_secret` | Client secret (stored encrypted at rest) |
| `scopes` | Requested scopes (default `openid email profile`) |
| `enabled` | Toggle the provider on/off |

### API Endpoints

**Public (login flow):**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/auth/oidc/providers` | List enabled providers (name + id only) |
| `GET` | `/api/v1/auth/oidc/{id}/authorize` | Start OIDC flow (PKCE + state) |
| `GET` | `/api/v1/auth/oidc/{id}/callback` | OIDC callback |

**Admin (requires `oidc:manage`):**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/api/v1/oidc/providers` | List all providers |
| `POST` | `/api/v1/oidc/providers` | Create provider |
| `GET`/`PUT`/`DELETE` | `/api/v1/oidc/providers/{id}` | Manage a provider |

## Login Flow

1. The login page lists enabled OIDC providers (fetched from `/api/v1/auth/oidc/providers`).
2. User clicks a provider → Alga generates PKCE verifier + challenge, nonce, and state, then redirects to the IdP authorization endpoint.
3. The IdP authenticates the user and redirects back to `/api/v1/auth/oidc/{id}/callback`.
4. Alga exchanges the authorization code for tokens (using PKCE + client secret).
5. **Alga verifies the ID token**: signature via JWKS, plus `iss`, `aud`, `exp`, and `nonce` checks.
6. **The `email_verified` claim must be `true`** — unverified emails are rejected to prevent account takeover.

## User Provisioning

::: warning No auto-account-creation
Alga **links** a verified OIDC identity to an **existing** user — it does not auto-create new accounts. Users must be provisioned (by an admin, or pre-existing) before their first OIDC login.
:::

On callback, Alga resolves the user by:

1. **Subject match** — looks up an existing `OIDCIdentity` by `(provider_id, subject)`.
2. **Email match** — if no identity exists but the verified email matches an existing user, Alga creates the identity link. This is safe because `email_verified` was enforced.
3. If no matching account is found → redirects to `/login?error=oidc_no_account`.

Once linked, the identity persists in `oidc_identities`; subsequent logins resolve directly by `(provider_id, subject)`. Account lock (`locked_until`) is honored.
