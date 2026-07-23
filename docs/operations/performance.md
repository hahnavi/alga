---
title: Performance & Scaling
description: Resource recommendations, horizontal scaling with Valkey and RabbitMQ, database tuning, and backend/frontend performance optimization.
---

# Performance & Scaling

## Resource Recommendations

| Deployment | CPU | Memory | Disk | Replicas |
|------------|-----|--------|------|----------|
| Development | 2 cores | 4 GB | 50 GB | 1 |
| Production (Small) | 4 cores | 8 GB | 100 GB | 2 |
| Production (Medium) | 8 cores | 16 GB | 200 GB | 3-4 |
| Production (Large) | 16+ cores | 32 GB+ | 500 GB+ | 5+ |

## Horizontal Scaling

Alga scales horizontally when Valkey and RabbitMQ are configured:

1. Deploy multiple backend replicas behind a load balancer
2. Share PostgreSQL, Valkey, and RabbitMQ across replicas
3. Configure `SCHEDULER_LEADER_TTL` (default: 15s) for leader election
4. SSE fan-out works across replicas via Valkey pub/sub

### Coordination Mechanisms

| Mechanism | Purpose | Storage |
|-----------|---------|---------|
| Leader election | Single scheduler tick runner | Valkey lease |
| Agent presence | Track online agents across replicas | Valkey hash |
| SSE fan-out | Cross-replica event distribution | Valkey pub/sub |
| Atomic claims | Prevent double-dispatch | PostgreSQL row lock |
| Disconnect grace | Handle agent reconnects | Valkey SET NX |

## Database Optimization

### Connection Pooling

Use PgBouncer for connection management:

```ini
[databases]
alga = host=postgres port=5432 dbname=alga

[pgbouncer]
pool_mode = transaction
max_client_conn = 200
default_pool_size = 25
```

### Indexing

Key indexes (auto-created by Ent):
- `alerts.fingerprint` — partial unique index (`WHERE status != 'resolved'`)
- `alerts.status`, `alerts.severity` — query filters
- `investigations.status`, `investigations.agent_id` — scheduler lookups
- `incidents.service_id`, `incidents.status` — service incident queries

### Read Replicas

For read-heavy workloads:
1. Set up PostgreSQL streaming replication
2. Route read queries to replicas via PgBouncer
3. Write queries go to primary

## Valkey Optimization

### Memory Management

- **Sessions**: Short TTL (`SESSION_EXPIRY_HOURS * 3600`)
- **Agent presence**: 90s TTL, renewed on heartbeat
- **SLA sorted sets**: Entries removed on incident close
- **Correlation windows**: TTL matches `CORRELATION_WINDOW`
- **On-call cache**: 5-minute TTL

### Persistence

For production:
- Enable RDB snapshots for point-in-time recovery
- Consider AOF for durability
- Use Valkey Sentinel for HA

## RabbitMQ Tuning

### Queue Configuration

| Queue | Purpose | Prefetch |
|-------|---------|----------|
| `alga.alert.process` | Alert processing | 10 |
| `alga.investigate.process` | Investigation dispatch | 3 |
| `alga.triage.process` | Triage decisions | 10 |
| `alga.incident.process` | Incident management | 10 |
| `alga.escalation.process` | Escalation processing | 10 |
| `alga.sla.sweep` | SLA breach detection | 10 |
| `alga.email.send` | Email delivery | 10 |
| `alga.notification-dispatch.process` | Notification dispatching | 10 |
| `alga.notification.send` | Notification sending | 10 |
| `alga.audit.log` | Audit logging | 20 |

### Retry Topology

Per-domain retry chains with TTL values:

| Domain | retry.1 | retry.2 | retry.3 | DLQ |
|--------|---------|---------|---------|-----|
| Alert | 30s | 2min | 5min | Dead-letter |
| Investigate | 60s | 5min | 15min | Dead-letter |
| Triage | 30s | 2min | 5min | Dead-letter |
| Incident | 60s | 5min | 15min | Dead-letter |
| Escalation | 30s | 2min | 5min | Dead-letter |
| Notification-dispatch | 30s | 2min | 5min | Dead-letter |

Tune retry counts and TTLs based on your error patterns.

### Consumer Scaling

Increase consumers per queue by deploying more backend replicas. Each replica runs its own consumer pool.

## Backend Tuning

### Argon2id

Password hashing is CPU-intensive. Tune for your hardware:

```sh
ARGON2_MEMORY_KIB=65536   # 64 MiB per hash
ARGON2_TIME=3              # Iterations
ARGON2_PARALLELISM=2       # Parallel threads
MAX_CONCURRENT_PASSWORD_HASHES=4  # Default: 2 * NumCPU
```

Higher values = more secure but slower logins. `MAX_CONCURRENT_PASSWORD_HASHES` caps memory usage: `value * ARGON2_MEMORY_KIB`.

### Scheduler

```sh
SCHEDULER_LEADER_TTL=15s   # Leader lease
AGENT_PRESENCE_TTL=90s     # Agent presence
AGENT_DISCONNECT_GRACE=45s # Grace period
STALE_ALERT_THRESHOLD=15m  # Stale alert age
STALE_ALERT_SWEEP_INTERVAL=5m  # Sweep frequency
```

## Frontend Optimization

The frontend is served by nginx with:
- Gzip compression
- Static asset caching (immutable, 1 year)
- SPA routing via `try_files`

### Vite Build

```sh
pnpm --filter frontend build
```

Output is minified and tree-shaken. No additional optimization needed.

## See Also

- [Architecture](/operations/architecture) — system design
- [Monitoring](/operations/monitoring) — observability setup
- [Deployment](/operations/deployment) — deployment options
