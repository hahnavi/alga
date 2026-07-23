---
title: Migration Guide
description: Database migration behavior, version upgrades, breaking changes, config migration, and migrating from other platforms.
---

# Migration Guide

## Database Migrations

Alga uses Ent ORM for schema management. Schema definitions live in `apps/backend/ent/schema/`.

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

Ent auto-migration:
1. Compares current schema with database state
2. Creates missing tables and columns
3. Creates missing indexes
4. Does NOT drop columns or tables (safe by default)

For destructive changes, you need to handle them manually.

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
6. **Rebuild and restart:**
   ```sh
   docker compose up -d --build
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
   git checkout v1.2.3  # Previous version
   docker compose up -d --build
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
