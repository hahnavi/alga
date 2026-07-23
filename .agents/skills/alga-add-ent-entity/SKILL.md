---
name: alga-add-ent-entity
description: Use when adding a persistent Ent-backed database entity, store, registry entry, migration path, or generated Ent client support in Alga.
priority: P1
tags: [backend, go, ent, database, store]
---

# Add an Ent Entity

Use this for new PostgreSQL-backed domain data. Keep the entity minimal; add fields, edges, indexes, and store methods only for behavior needed now.

Before editing, identify the domain invariant the entity enforces, whether it is user scoped, and whether follow-on API/RBAC/audit work is required. For alert, investigation, incident, scheduler, RabbitMQ, or Valkey behavior, also use `alga-domain-invariants`.

## Check First

- Similar schema: `apps/backend/ent/schema/`.
- Shared schema helpers: `timeNow`, enum type files, and existing annotations.
- Store helpers: `apps/backend/store/pg_helpers.go`.
- Registry: `apps/backend/store/registry.go`.
- Existing domain tests near the store or API using the entity.

## Schema Rules

- Put schemas in `apps/backend/ent/schema/<entity>.go`.
- Use explicit table annotations when nearby code does.
- Use UUID primary keys unless the domain has a concrete reason not to.
- Use `created_at` and `updated_at` with `timeNow` for persistent records.
- Add edges only when the relation is required now.
- Add unique indexes and partial indexes only when they enforce real invariants.
- For enums, follow nearby schema-local constants or shared type files; do not invent parallel enum models.

Minimal shape:

```go
func (Thing) Fields() []ent.Field {
    return []ent.Field{
        field.UUID("id", uuid.UUID{}).Default(uuid.New).StorageKey("id"),
        field.String("name").NotEmpty(),
        field.Time("created_at").Default(timeNow),
        field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
    }
}
```

## Generate and Migrate

After schema edits:

```bash
cd apps/backend
go generate ./ent
go test ./ent/... ./store
```

For local migration checks:

```bash
cd apps/backend
go run . db migrate
```

## Store Rules

- Put store code in `apps/backend/store/<entity>.go`.
- Define a record type, filter type when listing, interface, `pg<Entity>Store`, constructor, Ent mapper, and all methods declared in the interface.
- Embed `pgStoreBase` and register the store in `store/registry.go`.
- Use `pgctx()` inside store methods unless an existing nearby store intentionally uses caller context.
- Use `handleQueryErr` for not-found translation and wrap other errors with `%w`.
- Use transactions only when consistency requires them; always rollback with `rollbackTx`.
- Use Ent predicates/builders; do not concatenate SQL strings.

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

Narrowest check after schema edits: `cd apps/backend && go generate ./ent && go test ./store ./ent/...`.
