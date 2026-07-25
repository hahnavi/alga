---
title: Deployment
description: Deploy Alga with Docker Compose, Kubernetes/Helm, or a production overlay with Caddy HTTPS. Includes TLS, scaling, and backup guidance.
---

# Deployment

## Docker Compose (Recommended)

Production deployments use the base compose file with the production overlay, which enforces required secrets, adds container hardening, and routes all traffic through Caddy:

```sh
git clone https://github.com/hahnavi/alga.git && cd alga
./setup.sh
# Edit .env — set DOMAIN, and verify the generated secrets
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

The UI is served on `https://${DOMAIN}`. The nginx-based frontend image proxies `/api`, `/api/v1/events` (SSE), and `/webhooks` to the backend internally.

### Environment Variables

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `ALGA_VERSION` | `latest` | No | Image tag for `ghcr.io/hahnavi/alga-{backend,frontend}` |
| `DOMAIN` | `localhost` | **Yes** | Public hostname for Caddy TLS |
| `SECURE_COOKIES` | `false` | No | Set `true` only behind HTTPS (TLS-terminating proxy) |
| `POSTGRES_USER` | `alga` | No | PostgreSQL user |
| `POSTGRES_PASS` | — | **Yes** | PostgreSQL password |
| `POSTGRES_DB` | `alga` | No | PostgreSQL database name |
| `VALKEY_PASSWORD` | — | **Yes** | Valkey requirepass |
| `RABBITMQ_USER` | `alga` | No | RabbitMQ user |
| `RABBITMQ_PASS` | — | **Yes** | RabbitMQ password |
| `ENCRYPTION_KEYS` | — | **Yes** | Comma-separated `kid:base64(32-byte key)` pairs (startup fails without this) |
| `SECRET_PEPPER` | — | **Yes** | HMAC pepper for token hashing (startup fails without this) |
| `LOG_LEVEL` | `info` | No | Backend log level |
| `ENVIRONMENT` | `production` | No | Runtime environment label |
| `SESSION_EXPIRY_HOURS` | `24` | No | Session lifetime in hours |

`setup.sh` generates all required secrets automatically. To generate them manually:

```sh
# ENCRYPTION_KEYS (kid:base64 format)
echo "1:$(openssl rand -base64 32)"

# SECRET_PEPPER
openssl rand -base64 32
```

### Services

| Service | Image | Port | Description |
|---------|-------|------|-------------|
| `postgres` | `pgvector/pgvector:pg18` | — (internal) | PostgreSQL 18 with pgvector extension |
| `valkey` | `valkey/valkey:9.1-alpine` | — (internal) | In-memory store (sessions, leader election, pub/sub) |
| `rabbitmq` | `rabbitmq:4.3.3-management-alpine` | — (internal) | Message queue (async pipeline) |
| `backend` | `ghcr.io/hahnavi/alga-backend:${ALGA_VERSION}` | — (internal) | Go API server + workers |
| `frontend` | `ghcr.io/hahnavi/alga-frontend:${ALGA_VERSION}` | — (internal) | Vue web UI (nginx) |
| `caddy` | `caddy:2-alpine` | 80, 443 | TLS-terminating reverse proxy |

All services run on an internal bridge network. Only Caddy ports are published to the host.

### Upgrading

```sh
git pull
docker compose -f docker-compose.yml -f docker-compose.prod.yml pull
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

Auto-migration handles schema changes on backend startup (`POSTGRES_AUTO_MIGRATE=true`).

## Local Development Infrastructure

The root `docker-compose.yml` pulls pre-built images from GitHub Container Registry and provides the full local stack. Use `setup.sh` to bootstrap secrets:

```sh
git clone https://github.com/hahnavi/alga.git
cd alga
./setup.sh
docker compose up -d
```

To build from source instead (for contributors):

```sh
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build
```

`setup.sh` generates:
- Root `.env` with random `POSTGRES_PASS`, `VALKEY_PASSWORD`, `RABBITMQ_PASS`
- `apps/backend/.env` with random `ENCRYPTION_KEYS` and `SECRET_PEPPER`
- `apps/frontend/.env` (empty placeholder)

Infrastructure ports are bound to `127.0.0.1` only (PostgreSQL 5432, RabbitMQ 5672/15672).

## Kubernetes / Helm

```sh
helm install alga deploy/charts/alga/ \
  --set backend.secrets.encryptionKey=$(openssl rand -base64 32) \
  --set backend.secrets.secretPepper=$(openssl rand -base64 32) \
  --set backend.secrets.postgresDSN="postgres://alga:password@postgresql:5432/alga?sslmode=disable" \
  --set ingress.enabled=true \
  --set ingress.hosts[0].host=alga.example.com
```

### Key Values

```yaml
backend:
  replicas: 1
  resources:
    requests: { memory: 256Mi, cpu: 250m }
    limits: { memory: 512Mi, cpu: "1" }
  config:
    environment: production
    postgresAutoMigrate: true

frontend:
  replicas: 1
  resources:
    requests: { memory: 128Mi, cpu: 100m }
    limits: { memory: 256Mi, cpu: 500m }

ingress:
  enabled: true
  className: nginx
  hosts:
    - host: alga.example.com

postgresql:
  enabled: true
valkey:
  enabled: true
rabbitmq:
  enabled: true
```

Disable sub-charts (`enabled: false`) to use external infrastructure.

### Scaling

Alga backend replicas are stateless and coordinate through shared infrastructure:

1. Set `backend.replicas: 2+`
2. Ensure Valkey is reachable (sessions, leader election, SSE pub/sub)
3. Ensure RabbitMQ is reachable (async pipeline, work distribution)

**Leader election:** Only one replica runs the investigation scheduler at a time. Leadership is coordinated via a Valkey-backed lease (`SCHEDULER_LEADER_TTL`). If the leader dies, another replica acquires the lease after TTL expiry.

**SSE fan-out:** Real-time events (agent presence, UI updates) are published to Valkey pub/sub so that an event generated on one replica reaches SSE clients connected to any replica.

**Work distribution:** RabbitMQ consumer prefetch ensures investigations are distributed across all replicas running workers.

## HTTPS / TLS

### Caddy (Recommended)

A production-ready Caddyfile is provided in `deploy/caddy/Caddyfile`. Use the production Compose overlay to add Caddy as a TLS-terminating reverse proxy:

```sh
export DOMAIN=alga.example.com
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

Caddy provides automatic HTTPS with Let's Encrypt. The production overlay hides direct backend and frontend port exposure — all traffic goes through Caddy on ports 80/443. The Caddyfile routes:
- `/api/*` and `/webhooks/*` to the backend
- `/metrics` and `/health` to the backend (restrict at the network level)
- Everything else to the frontend

Certificate provisioning and renewal are handled automatically. Certificates persist in `caddy_data` and `caddy_config` named volumes.

### nginx

```nginx
server {
    listen 443 ssl http2;
    server_name alga.example.com;

    ssl_certificate /etc/letsencrypt/live/alga.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/alga.example.com/privkey.pem;

    location /api/ {
        proxy_pass http://backend:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /webhooks/ {
        proxy_pass http://backend:8080;
    }

    location /api/v1/events {
        proxy_pass http://backend:8080;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_read_timeout 86400s;
        proxy_buffering off;
    }

    location / {
        proxy_pass http://frontend:80;
    }
}
```

The `/api/v1/events` SSE endpoint requires `proxy_read_timeout 86400s` (24 hours) and `proxy_buffering off`.

When deploying behind a reverse proxy, set `TRUSTED_PROXIES` to the proxy's IP/CIDR so the backend extracts the correct client IP from `X-Forwarded-For`.

### Secure Cookies

When serving over HTTPS:

```sh
ENVIRONMENT=production
SECURE_COOKIES=true
```

HSTS (`Strict-Transport-Security`) is always emitted on HTTPS responses regardless of the `SECURE_COOKIES` flag.

## Backups

### PostgreSQL

```sh
docker compose exec postgres pg_dump -U alga alga > backup.sql
```

### Valkey

Valkey persistence is enabled via RDB snapshots in the named volume.

## See Also

- [Architecture](/operations/architecture) — system architecture overview
- [Performance & Scaling](/operations/performance) — optimization guide
- [Monitoring](/operations/monitoring) — observability setup
