---
title: Backup & Restore
description: Backup procedures for PostgreSQL, Valkey, application secrets, and RabbitMQ — plus disaster recovery and backup testing.
---

# Backup & Restore

## PostgreSQL

### Manual Backup

```sh
# Using pg_dump
docker compose exec postgres pg_dump -U alga alga > backup_$(date +%Y%m%d).sql

# Compressed backup
docker compose exec postgres pg_dump -U alga --format=custom alga > backup_$(date +%Y%m%d).dump
```

### Automated Backups

Set up a cron job:

```cron
# Daily backup at 2 AM
0 2 * * * docker compose exec -T postgres pg_dump -U alga --format=custom alga > /backups/alga_$(date +\%Y\%m\%d).dump
```

### Restore

```sh
# From SQL dump
cat backup.sql | docker compose exec -T postgres psql -U alga alga

# From custom format
docker compose exec -T postgres pg_restore -U alga -d alga < backup.dump
```

### Point-in-Time Recovery

PostgreSQL WAL archiving enables point-in-time recovery:

1. Enable WAL archiving in `postgresql.conf`:

```
wal_level = replica
archive_mode = on
archive_command = 'cp %p /backups/wal/%f'
```

2. Perform base backup:

```sh
docker compose exec postgres pg_basebackup -D /backups/base -Ft -z -P
```

3. Restore to a specific point in time by configuring `recovery_target_time` in `postgresql.conf`.

## Valkey

### RDB Snapshots

Valkey persistence is enabled by default via RDB snapshots stored in the named volume.

```sh
# Trigger manual save
docker compose exec valkey valkey-cli -a "${VALKEY_PASSWORD}" BGSAVE

# Copy RDB file
docker compose cp valkey:/data/dump.rdb ./valkey_backup_$(date +%Y%m%d).rdb
```

### Restore

```sh
# Stop Valkey
docker compose stop valkey

# Replace RDB file
docker compose cp ./valkey_backup.rdb valkey:/data/dump.rdb

# Start Valkey
docker compose start valkey
```

## Application Secrets

### Encryption Keys

**Critical:** Back up `ENCRYPTION_KEYS` (or `ENCRYPTION_KEY`) securely. Loss of these keys means:
- Integration credentials (Slack, Mattermost, Twilio) become unreadable
- Stored secrets must be re-entered

Store keys in:
- HashiCorp Vault
- AWS Secrets Manager
- Kubernetes Secrets (encrypted at rest)
- Offline encrypted storage (e.g., GPG-encrypted file)

### SECRET_PEPPER

**Critical:** Loss of `SECRET_PEPPER` means:
- All user passwords must be reset
- All active sessions are invalidated
- All bearer tokens must be regenerated

### Backup Strategy

```sh
# Create secrets backup
cat > secrets_backup.env <<EOF
ENCRYPTION_KEYS="your-keys-here"
SECRET_PEPPER="your-pepper-here"
EOF

# Encrypt with GPG
gpg --symmetric --cipher-algo AES256 secrets_backup.env
rm secrets_backup.env
```

## RabbitMQ

### Queue Definitions

```sh
# Export definitions
docker compose exec rabbitmq rabbitmqctl export_definitions /tmp/definitions.json
docker compose cp rabbitmq:/tmp/definitions.json ./rabbitmq_definitions.json
```

### Restore Definitions

```sh
docker compose cp ./rabbitmq_definitions.json rabbitmq:/tmp/definitions.json
docker compose exec rabbitmq rabbitmqctl import_definitions /tmp/definitions.json
```

## Full Disaster Recovery

### Recovery Steps

1. **Restore infrastructure:**
   ```sh
   docker compose up -d postgres valkey rabbitmq
   ```

2. **Restore PostgreSQL:**
   ```sh
   cat backup.sql | docker compose exec -T postgres psql -U alga alga
   ```

3. **Restore secrets:**
   - Decrypt and set `ENCRYPTION_KEYS`, `SECRET_PEPPER`
   - Update `apps/backend/.env`

4. **Start application:**
   ```sh
   docker compose up -d
   ```

5. **Verify:**
   ```sh
   curl http://localhost:8080/health
   curl http://localhost:8080/api/v1/readiness
   ```

### Recovery Time Objectives

| Component | RTO | Method |
|-----------|-----|--------|
| PostgreSQL | ~5 min | pg_restore from backup |
| Valkey | ~1 min | RDB snapshot restore |
| RabbitMQ | ~2 min | Definition import |
| Backend | ~30 sec | Container restart |
| Full system | ~10 min | Sequential restore |

## Testing Backups

Regularly verify backups:

```sh
# Test PostgreSQL backup integrity
pg_restore --list backup.dump

# Test restore to a separate database
createdb alga_test
pg_restore -d alga_test backup.dump
psql alga_test -c "SELECT count(*) FROM users;"
dropdb alga_test
```

Schedule monthly restore tests to ensure backup integrity.
