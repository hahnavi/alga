---
title: Deployment
description: Deploy Alga with Docker Compose, Kubernetes/Helm, or a production overlay with Caddy HTTPS. Includes TLS, scaling, and backup guidance.
---

# Deployment

## Docker Compose (Recommended)

```sh
git clone https://github.com/hahnavi/alga.git
cd alga
./setup.sh
docker compose up -d
```

### Services

| Service | Image | Port | Description |
|---------|-------|------|-------------|
| `postgres` | postgres:17-alpine | 5432 | PostgreSQL database |
| `valkey` | valkey:8.1-alpine | — | In-memory store (internal only) |
| `rabbitmq` | rabbitmq:4.2.4-management-alpine | 5672, 15672 | Message queue |
| `backend` | Built locally | 8080 | Go API server |
| `frontend` | Built locally | 3000 | Vue web UI (nginx) |

### Resource Limits

| Service | Memory | CPU |
|---------|--------|-----|
| Backend | 512 MB | 1.0 |
| Frontend | 256 MB | 0.5 |

### Upgrading

```sh
git pull
docker compose up -d --build
```

Auto-migration handles schema changes on startup.

### Production Override

```sh
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

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

For horizontal scaling:
1. Set `backend.replicas: 2+`
2. Ensure Valkey is configured (sessions, leader election)
3. Ensure RabbitMQ is configured (async pipeline)
4. Multiple replicas coordinate via Valkey pub/sub for SSE fan-out

## HTTPS / TLS

### Caddy (Recommended)

```sh
export DOMAIN=alga.example.com
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

Caddy provides automatic HTTPS with Let's Encrypt. The production overlay:
- Adds a Caddy reverse proxy on ports 80 and 443
- Routes `/api/*` and `/webhooks/*` to backend
- Routes everything else to frontend
- Handles certificate renewal automatically

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

The `/api/v1/events` SSE endpoint requires `proxy_read_timeout 86400s` (24 hours).

### Secure Cookies

When using HTTPS:

```sh
ENVIRONMENT=production
SECURE_COOKIES=true
```

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
