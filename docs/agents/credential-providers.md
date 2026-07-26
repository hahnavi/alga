---
title: Credential Providers & Shared Secrets
description: Securely store and broker secrets that AI agents need during investigations — AES-256-GCM encrypted at rest, scoped per-agent access, full audit trail.
---

# Credential Providers & Shared Secrets

Alga can store and broker secrets that AI agents need during investigations — database credentials, API keys, etc. Secrets are fetched at runtime by authorized agents over a scoped, audited endpoint. Secrets are never exposed in logs or API list responses.

## Credential Providers

A **credential provider** is a backend that stores or resolves secret values. The `internal` provider stores values encrypted in the Alga database (AES-256-GCM). External providers (HashiCorp Vault, AWS Secrets Manager, GCP Secret Manager, Azure Key Vault) are selectable in the UI for future integration.

| Type | Description |
|------|-------------|
| `internal` | Values encrypted at rest in the Alga DB (fully implemented; seeded as a system default) |
| `hashicorp_vault` | External — resolves via Vault path |
| `aws_secrets_manager` | External — resolves via AWS ARN |
| `gcp_secret_manager` | External — resolves via GCP resource name |
| `azure_key_vault` | External — resolves via Azure Key Vault |

::: warning External providers
The four external provider types are persisted and selectable today, but return "not implemented" (HTTP 501) until a real backend is wired in. Use `internal` for stored secrets now.
:::

The `internal` provider is seeded automatically as a **system** provider and cannot be deleted or reconfigured.

## Shared Secrets

A **shared secret** is a credential that one or more agents are authorized to fetch. Each shared secret belongs to a provider and has a server-generated `secret_id` that agents use to retrieve it.

| Field | Description |
|------|-------------|
| `provider_id` | Owning credential provider |
| `name` | Human-readable name |
| `secret_id` | Server-generated identifier agents use to fetch (lowercase UUID) |
| `description` | Optional context |
| `remote_ref` | Backend path for external providers (vault path / AWS ARN); empty for `internal` |
| `value` | Plaintext value for `internal` (write-only; never returned in list/get) |
| `allowed_agent_ids` | Agent token IDs permitted to fetch. **Empty list denies all agents** |

### Access Control

- Only agent tokens listed in `allowed_agent_ids` can fetch the secret.
- Unauthorized or non-existent secret fetches return **404** (deliberately generic — existence is not leaked).
- Every fetch is **audited** (`AuditSharedSecretAccessed`).

## Agent Fetch

Agents fetch secrets over the agent bearer API:

```sh
curl https://alga.example.com/api/v1/agent/secrets/{secret_id} \
  -H "Authorization: Bearer alga_agent_..."
```

Response (value shown only here, at fetch time):

```json
{
  "secret_id": "...",
  "name": "prod-db-readonly",
  "value": "<plaintext>",
  "fetched_at": "2026-07-11T12:00:00Z"
}
```

## API

All admin endpoints require an authenticated session; the Permission column lists the additional RBAC permission required.

### Credential Providers (admin)

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/credential-providers` | `credentials:read` | List providers |
| `POST` | `/api/v1/credential-providers` | `credentials:manage` | Create provider |
| `GET`/`PATCH`/`DELETE` | `/api/v1/credential-providers/{id}` | `credentials:read`/`manage` | Manage a provider |

### Shared Secrets (admin)

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/shared-secrets` | `credentials:read` | List shared secrets |
| `POST` | `/api/v1/shared-secrets` | `credentials:manage` | Create shared secret |
| `GET`/`PATCH`/`DELETE` | `/api/v1/shared-secrets/{id}` | `credentials:read`/`manage` | Manage a shared secret |

### Agent

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/agent/secrets/{secret_id}` | Agent Bearer | Fetch a shared secret (must be in `allowed_agent_ids`) |
