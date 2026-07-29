---
name: alga-security-checklist
description: Use when reviewing Alga changes that touch authentication, authorization, API routes, secrets, sessions, tokens, user-scoped data, integrations, or mutations.
priority: P2
tags: [security, review, auth, rbac, checklist]
---

# Alga Security Checklist

Use this before implementing and before finishing changes that touch routes, auth, RBAC, sessions, secrets, tokens, integrations, user-scoped data, or mutations.

AGENTS.md "Secure By Default" is the always-loaded baseline; this checklist applies and expands it for review — build on the baseline, don't restate it.

At the start, write the route/data classification and expected controls. Before finishing, re-check the same list against the diff and tests.

## Route Classification

Classify every route explicitly:

- Public/callback: intentionally unauthenticated, defensible, and rate limited when auth-adjacent.
- Authenticated self-scoped: uses current user from auth context; never trusts body `user_id`.
- RBAC-protected operator/frontend: uses `authMiddleware` and permissions.
- Agent bearer: uses `agentBearerMiddleware(agentRateLimitMiddleware(...))`.

## Auth, RBAC, CSRF

- Frontend/operator `/api/v1` routes are wrapped in `authMiddleware` unless explicitly public/callback.
- State-changing frontend/operator routes rely on CSRF through `authMiddleware`.
- Pass permissions to `authMiddleware(handler, rbac.XxxRead)` when one permission covers the route.
- For multi-method dispatchers, call `s.checkPermission` inside each method path.
- Add new permissions in `apps/backend/rbac/permissions.go`.
- Grant roles in `apps/backend/rbac/roles.go`: admin full, viewer read-only, operator appropriate write.
- Add unauthorized and forbidden tests for protected routes when feasible.

## Input and Output

- Decode request bodies only with `decodeJSON`.
- Validate required fields, path params, enum values, ownership, and state transitions.
- Use Bun query builders and bound parameters; no SQL string concatenation.
- Return generic internal errors with `writeInternalError`.
- Do not expose secrets, token hashes, peppers, encryption keys, plaintext credentials, or internal-only lookup values.
- Keep response models explicit; do not reuse persistence records that contain sensitive fields.

## Secrets and Tokens

- Passwords use the crypto package Argon2id flow with `SECRET_PEPPER`.
- Session IDs, refresh tokens, webhook tokens, agent tokens, and PATs are stored as HMAC hashes plus lookup metadata, not plaintext.
- Secret/token comparisons use constant-time comparison in validation paths.
- Integration credentials are encrypted with the configured keyring.
- Newly created bearer tokens are shown once and cannot be recovered later.
- Production fails closed without `ENCRYPTION_KEYS` or `ENCRYPTION_KEY`, plus `SECRET_PEPPER`.

## Mutations and Data Safety

- Create/update/delete/command/state-transition handlers audit with `s.audit`.
- Audit event constants live in `apps/backend/store/audit.go`.
- SSE broadcasts do not replace persistence or audit.
- Deletions are hard-delete only when the domain already uses hard-delete safely.
- User-scoped operations derive the user from auth context and verify ownership or membership.

## Rate Limits, Cookies, Proxies

- Login, password reset, OAuth callbacks, webhook/token-adjacent, and agent routes use existing rate-limit middleware.
- Session cookies are HttpOnly, SameSite strict, and Secure in production.
- Trusted proxy configuration is considered when deploying behind load balancers.
- Do not implement inline rate limiting in handlers.

## Quick Triage Commands

```bash
cd apps/backend
rg -n "HandleFunc" api/http.go
rg -n "==.*(secret|token|password)|password.*==" --glob '*.go'
rg -n "handle(Create|Update|Patch|Delete|Command)" api
```

These commands find review targets; they are not proof of safety.

## Finish Criteria

- Route classification is documented in code/tests or obvious from middleware.
- Auth, RBAC, CSRF, and rate limits match the classification.
- Secrets are hashed/encrypted and compared safely.
- User-scoped access uses current auth context.
- Mutations audit.
- Tests cover unauthorized, forbidden, invalid input, and success paths where feasible.
- `alga-testing-patterns` was used when deciding test coverage.
