# Migrations

Hand-written goose SQL migrations (no code generation). The Bun models in
`db/models/` must be edited alongside every schema change.

## Provenance notes

Filenames of already-applied migrations are part of version tracking and are
never renamed — historical oddities stay and are explained here instead.

- **`00003_org.sql`** (2026-08): the name implies multi-tenancy,
  but no `organizations` table or `org_id` column exists anywhere; the file
  holds the teams / on-call / credential-providers / shared-secrets DDL. All
  of that DDL is actively consumed — only the filename's "org" framing is
  vestigial.
