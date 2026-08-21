---
title: Installation & Setup
description: Install Alga via Docker Compose, Helm on Kubernetes, manual setup, or build from source. Production deployment prerequisites and resource recommendations.
---

# Installation & Setup

## Docker Compose (Recommended)

The fastest way to run Alga:

```sh
git clone https://github.com/hahnavi/alga.git
cd alga
./setup.sh
docker compose up -d
```

`setup.sh` generates `ENCRYPTION_KEYS` and `SECRET_PEPPER` in `apps/backend/.env`. The admin account is **not** seeded automatically: on first boot with no users in the database, open `http://localhost:3000` and complete the setup wizard to create the initial admin account (email, password, and full name).

### Services

| Service    | Image                            | Port        | Purpose                             |
| ---------- | -------------------------------- | ----------- | ----------------------------------- |
| `postgres` | pgvector/pgvector:pg18           | 5432        | PostgreSQL database (with pgvector) |
| `valkey`   | valkey/valkey:9.1-alpine         | 6379        | Sessions, caching, leader election  |
| `rabbitmq` | rabbitmq:4.3.3-management-alpine | 5672, 15672 | Async message queue                 |
| `backend`  | ghcr.io/hahnavi/alga-backend     | 8080        | Go API server                       |
| `frontend` | ghcr.io/hahnavi/alga-frontend    | 3000        | Vue web UI (nginx)                  |

Pin a specific release by setting `ALGA_VERSION=v1.2.3` in `.env`.

### Verify

```sh
docker compose ps
curl http://localhost:8080/health
```

Open `http://localhost:3000` and complete the setup wizard to create the initial admin account. The onboarding wizard then walks you through connecting integrations and configuring routing rules.

### Building from Source

Contributors who need to build images locally:

```sh
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build
```

## Helm (Kubernetes)

The Alga Helm chart is published to GHCR as an OCI artifact at `oci://ghcr.io/hahnavi/charts/alga`. It deploys the backend, frontend, and (by default) bundled PostgreSQL (pgvector), Valkey, and RabbitMQ.

### Prerequisites

- **Kubernetes** cluster with a default StorageClass (for bundled data services)
- **Helm** 3.8+ (OCI registry support)

### Install

The chart fails closed: `backend.secrets.encryptionKeys`, `backend.secrets.secretPepper`, and a pinned `auth.password` for each enabled bundled service are required.

Put secrets in a values file with owner-only permissions instead of `--set` flags, so they don't land in shell history or process listings:

```sh
(umask 077; cat > alga-values.yaml <<EOF
backend:
  secrets:
    encryptionKeys: "1:$(openssl rand -base64 32)"
    secretPepper: "$(openssl rand -base64 32)"
postgresql:
  auth:
    password: "$(openssl rand -hex 16)"
valkey:
  auth:
    password: "$(openssl rand -hex 16)"
rabbitmq:
  auth:
    password: "$(openssl rand -hex 16)"
EOF
)
```

```sh
helm install alga oci://ghcr.io/hahnavi/charts/alga --version 0.0.6 \
  --namespace alga --create-namespace \
  -f alga-values.yaml
```

Keep `alga-values.yaml` out of version control and reuse it for upgrades — the bundled-service passwords must stay stable across releases and retained volumes.

### Ingress

Ingress is enabled by default with host `alga.example.com`. Set your own host by appending these flags to the `helm install` command above (`--set` merges into the default hosts entry, keeping its paths):

```sh
helm install alga oci://ghcr.io/hahnavi/charts/alga --version 0.0.6 \
  --namespace alga --create-namespace \
  -f alga-values.yaml \
  --set 'ingress.hosts[0].host=alga.your-domain.com' \
  --set 'ingress.tls[0].hosts[0]=alga.your-domain.com'
```

To use externally managed data services, set `postgresql.enabled=false`, `valkey.enabled=false`, or `rabbitmq.enabled=false` and provide `backend.secrets.postgresDSN`, `backend.secrets.valkeyAddr`/`valkeyPassword`, or `backend.secrets.rabbitmqURI`. See `deploy/charts/alga/values.yaml` for all options.

### Verify

```sh
kubectl get pods -n alga
helm status alga -n alga
```

Open the ingress host in a browser and complete the setup wizard to create the initial admin account.

## Manual Installation

### Prerequisites

- **Go** 1.27+
- **Node.js** 22+ with **pnpm** 10
- **PostgreSQL** 18
- **Valkey** 9+
- **RabbitMQ** 4.3+

### Steps

1. **Clone and install dependencies:**

```sh
git clone https://github.com/hahnavi/alga.git
cd alga
pnpm install --no-frozen-lockfile
```

2. **Configure the backend:**

```sh
cp apps/backend/.env.example apps/backend/.env
```

Edit `apps/backend/.env` and set:

- `POSTGRES_DSN` — your PostgreSQL connection string
- `ENCRYPTION_KEY` — generate with `openssl rand -base64 32`
- `SECRET_PEPPER` — generate with `openssl rand -base64 32`

The admin account is not configured via `.env`. On first boot with no users in the database, open `http://localhost:3000` and complete the setup wizard to create the initial admin.

3. **Configure the frontend:**

```sh
cp apps/frontend/.env.example apps/frontend/.env
```

Set `VITE_API_BASE_URL` if the backend is on a different host (leave empty for Vite proxy).

4. **Start infrastructure services** (PostgreSQL, Valkey, RabbitMQ).

5. **Run the backend:**

```sh
moon run backend:dev
```

6. **Run the frontend:**

```sh
moon run frontend:dev
```

### Build from Source

```sh
# Backend binary
cd apps/backend
go build -o alga .
./alga

# Frontend production build
cd apps/frontend
pnpm build
```

## Production Setup

### Required Variables

In production (`ENVIRONMENT=production`), Alga requires:

- `ENCRYPTION_KEYS` — versioned keyring for AES-256-GCM secret encryption
- `SECRET_PEPPER` — HMAC pepper for password and token hashing

The admin account is not configured via environment. On first boot with no users in the database, open the UI and complete the setup wizard to create the initial admin. The onboarding wizard then guides you through connecting integrations and configuring routing rules.

Generate keys:

```sh
# Single key (first deployment)
export ENCRYPTION_KEY=$(openssl rand -base64 32)

# Versioned keyring (recommended for rotation)
export ENCRYPTION_KEYS="1:$(openssl rand -base64 32)"
```

### HTTPS

See [Deployment](/operations/deployment) for Caddy or nginx reverse proxy setup.

### Resource Recommendations

| Deployment          | CPU     | Memory | Disk   |
| ------------------- | ------- | ------ | ------ |
| Development         | 2 cores | 4 GB   | 50 GB  |
| Production (Small)  | 4 cores | 8 GB   | 100 GB |
| Production (Medium) | 8 cores | 16 GB  | 200 GB |

## Next Steps

- [First Steps Guide](/getting-started/first-steps) — explore key features
- [Configuration](/configuration/environment-variables) — all environment variables
- [Integrations](/integrations/) — connect Slack, Mattermost, or Twilio
