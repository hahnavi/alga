# Security Policy

## Supported Versions

| Version | Supported |
| --- | --- |
| `main` branch | Yes |
| Tagged releases | Yes |
| Development branches | No |

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Use [GitHub's private vulnerability reporting](https://github.com/hahnavi/alga/security/advisories/new) to submit your report.

- We will acknowledge your report within **48 hours**.
- We aim to provide a fix or mitigation within **90 days** of disclosure.
- Please include enough detail to reproduce the issue (version, configuration, steps).

We ask that you:

- Do not exploit the vulnerability beyond what is necessary to demonstrate it.
- Do not access or modify other users' data.
- Allow us a reasonable time to respond before any public disclosure.

## Security Features

Alga implements the following security measures:

- **Argon2id password hashing** — OWASP 2026 baseline parameters with server-side HMAC pepper pre-hash
- **AES-256-GCM encryption at rest** — Versioned keyring for integration secrets with in-place rotation
- **CSRF protection** — Double-submit cookie pattern on all state-changing endpoints
- **HTTP-only session cookies** — Secure and SameSite=Strict flags in production
- **Constant-time comparison** — `crypto/subtle` on every secret check path
- **HMAC token storage** — Bearer and session tokens stored as peppered HMAC, never plaintext
- **Rate limiting** — Login attempt limiting and per-IP request throttling
- **Audit logging** — All auth, alert, investigation, and administrative events persisted in PostgreSQL

## Scope

This policy covers the **Alga core application** (`apps/backend` and `apps/frontend`). Third-party integrations (Hermes adapter, OpenClaw plugin, Mattermost plugin, Slack app) should report to their respective maintainers unless the vulnerability originates in Alga's handling of integration data.
