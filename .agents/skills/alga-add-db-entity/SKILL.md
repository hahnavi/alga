---
name: alga-add-db-entity
description: Use when adding a persistent database entity, Bun model, store, registry entry, or goose migration in Alga.
priority: P1
tags: [backend, go, database, store, bun, goose]
---

# Add a Database Entity

Use this for new PostgreSQL-backed domain data. Keep the entity minimal; add columns, relations, indexes, and store methods only for behavior needed now.

Schema changes are hand-written: add or edit a Bun model in `apps/backend/db/models/`, then author a matching goose SQL migration in `apps/backend/db/migrations/`. There is no code-generation step; the model and the DDL are kept in sync by hand.

Before editing, identify the domain invariant the entity enforces, whether it is user scoped, and whether follow-on API/RBAC/audit work is required. For alert, investigation, incident, scheduler, RabbitMQ, or Valkey behavior, also use `alga-domain-invariants`.

## Check First

- Similar models: `apps/backend/db/models/`.
- Base structs and helpers: `apps/backend/db/models/base.go` (`BaseModel`, `IDModel`, `SoftDeleteModel`, `NewUUID`).
- Existing migrations and wiring: `apps/backend/db/migrations/`, plus `apps/backend/db/client.go` and `apps/backend/db/migrate.go`.
- Store helpers: `apps/backend/store/pg_helpers.go`.
- Registry: `apps/backend/store/registry.go`.
- Existing domain tests near the store or API using the entity.

## Model Rules

- Put models in `apps/backend/db/models/<entity>.go`.
- Use UUIDv7 primary keys (`ID uuid.UUID` with a `bun:"id,pk"` tag, filled via `models.NewUUID()`) unless the domain has a concrete reason not to.
- Embed `BaseModel` for standard tables (gives `id`, `created_at`, `updated_at`); the table name is inferred from the struct (`KnowledgeNote` becomes `knowledge_notes`). Set it explicitly with `bun.BaseModel` plus a `bun:"table:<name>"` tag when the name is not a simple pluralization (see `system_config`).
- Embed `IDModel` for append-only tables that carry their own authoritative timestamp (no `updated_at` trigger); add `SoftDeleteModel` only when the domain soft-deletes.
- Add relations (`bun:"rel:..."`) only when the relation is required now.
- Human-readable sequential numbers (`alert_number`, `incident_number`) come from native Postgres sequences via `nextSeq`, not from a model default.
- For enums, follow nearby model constants; do not invent parallel enum types.

Minimal model (package `models`):

```go
type Thing struct {
    BaseModel // id, created_at, updated_at; table inferred as "things"

    Name string `bun:"name,notnull"`
}
```

## Migrate

Author a new sequential migration in `apps/backend/db/migrations/` following the existing zero-padded numbering (for example `00013_things.sql`). Match the model exactly: same table name, columns, types, defaults, and indexes. Provide both `-- +goose Up` and `-- +goose Down`, and attach the shared `set_updated_at()` trigger to any table that carries `updated_at`.

```sql
-- +goose Up
CREATE TABLE things (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER trg_things_set_updated_at BEFORE UPDATE ON things FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS things CASCADE;
```

Migrations are embedded via `//go:embed migrations/*.sql` and applied on startup and by the CLI. Apply locally with:

```bash
cd apps/backend
go run . db migrate
```

## Store Rules

- Put store code in `apps/backend/store/<entity>.go`.
- Define a record type, filter type when listing, interface, `pg<Entity>Store`, constructor, model-to-record mapper, and all methods declared in the interface.
- Embed `pgStoreBase` and register the store in `store/registry.go`.
- Use `pgctx()` inside store methods unless an existing nearby store intentionally uses caller context.
- Use `handleQueryErr` for not-found translation and wrap other errors with `%w`.
- Use transactions only when consistency requires them; always rollback with `rollbackTx`.
- Use Bun query builders with bound parameters (`Where("id = ?", id)`); never concatenate values into SQL strings.

Store skeleton policy:

```go
type ThingStore interface {
    CreateThing(ctx context.Context, r *ThingRecord) (*ThingRecord, error)
    GetThing(ctx context.Context, id uuid.UUID) (*ThingRecord, error)
    ListThings(ctx context.Context, filter ListThingsFilter) ([]*ThingRecord, int64, error)
}
```

If you declare a method in the interface, implement it before moving on. Do not leave snippets or partial stores that cannot compile.

## Registry

- Add the store field to `Stores` in `apps/backend/store/registry.go`.
- Instantiate with the existing constructor style.
- If the store depends on another store, construct dependencies before the return literal following nearby examples.

## Follow-On Work

- API handler and route: use `alga-add-api-endpoint`.
- RBAC: add permissions and role grants if the entity is exposed.
- Audit: add event constants for mutations.
- Frontend: add typed API methods and pages only when exposed to users.

## Tests and Verification

- Add focused store tests for create/get/list/update/delete behavior you introduce.
- Cover not-found, duplicate/invariant errors, pagination/filtering, and transaction behavior where relevant.
- For exact test-shape guidance, use `alga-testing-patterns`; for the full command ladder, `alga-dev-environment`.

Narrowest check after model or migration edits: `cd apps/backend && go build ./... && go test ./store`.
