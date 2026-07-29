---
title: Migration Guide
description: Database migration behavior, version upgrades, breaking changes, config migration, and migrating from other platforms.
---

# Migration Guide

## Database Migrations

Alga manages its schema with hand-written [goose](https://github.com/pressly/goose) SQL migrations in `apps/backend/db/migrations/`, paired with the Bun models in `apps/backend/db/models/`. Migrations are embedded into the binary at build time.

### Auto-Migration

Enable auto-migration on startup:

```sh
POSTGRES_AUTO_MIGRATE=true
```

This is enabled by default in Docker Compose via `POSTGRES_AUTO_MIGRATE=true` in the environment block. The config default is `false`. Schema changes are applied on every startup when enabled.

### Manual Migration

```sh
# From apps/backend
go run . db migrate

# Or using the built binary
./alga db migrate
```

### Migration Process

Migrations are versioned SQL files applied by [goose](https://github.com/pressly/goose):

1. goose tracks applied versions in its `goose_db_version` table
2. On `up`, pending migrations run in version order, each within a transaction
3. Only the `-- +goose Up` blocks run on startup; the paired `-- +goose Down` blocks exist for manual rollback
4. Schema changes are explicit — there is no auto-diffing; author a new migration file and the matching Bun model change together

For destructive changes, author them explicitly in a migration.

## Version Upgrades

### Upgrade Checklist

1. **Review release notes** for breaking changes
2. **Backup the database** before upgrading
3. **Pull the latest code:**

   ```sh
   git pull
   ```

4. **Install dependencies:**

   ```sh
   pnpm install --no-frozen-lockfile
   cd apps/backend && go mod tidy
   ```

5. **Review configuration changes** — new env vars may be required
6. **Pull new images and restart:**

   ```sh
   docker compose pull
   docker compose up -d
   ```

7. **Verify:**

   ```sh
   curl http://localhost:8080/health
   curl http://localhost:8080/api/v1/readiness
   ```

### Zero-Downtime Deployments

For zero-downtime upgrades with multiple replicas:

1. Deploy new version to one replica
2. Wait for health check to pass
3. Drain and update remaining replicas one at a time
4. Use rolling updates in Kubernetes:

   ```yaml
   strategy:
     type: RollingUpdate
     rollingUpdate:
       maxUnavailable: 1
       maxSurge: 1
   ```

### Rollback

If issues occur after upgrade:

1. **Database rollback** — restore from pre-upgrade backup:

   ```sh
   cat backup_pre_upgrade.sql | docker compose exec -T postgres psql -U alga alga
   ```

2. **Code rollback:**

   ```sh
   # Pin ALGA_VERSION=v1.2.3 in .env, then:
   docker compose pull
   docker compose up -d
   ```

3. **Configuration rollback** — restore previous `.env` values

## Breaking Changes

### When Breaking Changes Occur

Alga follows semantic versioning:

- **Patch** (1.2.x): Bug fixes, no breaking changes
- **Minor** (1.x.0): New features, backward-compatible
- **Major** (x.0.0): Breaking changes, migration required

### Handling Breaking Changes

1. Release notes document all breaking changes
2. Migration scripts are provided for data changes
3. Deprecated features are warned for at least one minor version before removal
4. Configuration changes are documented with before/after examples

## Configuration Migration

### Environment Variable Changes

When env variables change:

1. New variables are added with sensible defaults
2. Deprecated variables are logged with migration instructions
3. Use the System Configuration API for runtime changes where possible

### Encryption Key Rotation

To rotate encryption keys:

1. Generate a new key:

   ```sh
   NEW_KEY=$(openssl rand -base64 32)
   ```

2. Add to `ENCRYPTION_KEYS` with a higher `kid`:

   ```sh
   ENCRYPTION_KEYS="1:old_key_base64,2:$NEW_KEY"
   ```

3. The highest `kid` becomes the active encryption key. Older keys decrypt existing ciphertexts.

4. After all secrets are re-encrypted (on next save), remove the old key:

   ```sh
   ENCRYPTION_KEYS="2:$NEW_KEY"
   ```

## From Other Platforms

### From OpsGenie

1. Export users, teams, and escalation policies via OpsGenie API
2. Import users: `POST /api/v1/users` for each user
3. Create teams: `POST /api/v1/teams`
4. Recreate escalation policies: `POST /api/v1/escalation-policies`
5. Update monitoring tools to send webhooks to Alga
6. Disable OpsGenie integrations

### From PagerDuty

1. Export services and schedules via PagerDuty API
2. Create services in Alga service catalog
3. Import on-call schedules
4. Recreate escalation policies
5. Update alert routing to point to Alga webhooks
6. Gradually migrate services one at a time

## See Also

- [Deployment](/operations/deployment) — deployment options
- [Backup & Restore](/operations/backup) — backup procedures
- [Architecture](/operations/architecture) — system design
