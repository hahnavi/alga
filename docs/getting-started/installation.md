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

`setup.sh` creates your secret passwords automatically — you don't need to edit anything. Then open `http://localhost:3000` and fill in the setup wizard to create your admin account (email, password, and full name). You only do this once.

### What gets installed

| Part     | What you see it as          | Opens on  |
| -------- | --------------------------- | --------- |
| Database | Stores all your data        | —         |
| Queue    | Delivers work in background | —         |
| Alga app | The web pages you click     | Port 3000 |
| Alga API | Works behind the scenes     | Port 8080 |

Pin a specific release by setting `ALGA_VERSION=v1.2.3` in `.env`.

### Check it's working

```sh
docker compose ps
curl http://localhost:8080/health
```

Open `http://localhost:3000` and finish the setup wizard to create your admin account. The next time you log in, a short welcome guide shows you where to connect your tools.

### Building from Source

Contributors who need to build images locally:

```sh
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d --build
```

## Helm (Kubernetes)

The chart deploys Alga plus the database, cache, and queue it needs (they're included by default, so you don't need to install them separately).

### What you need to install

- A **Kubernetes** cluster with storage available
- **Helm** 3.8+

### Install

The chart needs secret passwords to start — it won't run without them (this keeps your data safe). Save them in a private file rather than typing them on the command line:

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

Keep `alga-values.yaml` private (don't commit it) and reuse it for upgrades — the passwords must stay the same between releases.

### Internet address

By default Alga uses the address `alga.example.com`. Change it to your own address by adding these flags to the install command above:

```sh
helm install alga oci://ghcr.io/hahnavi/charts/alga --version 0.0.6 \
  --namespace alga --create-namespace \
  -f alga-values.yaml \
  --set 'ingress.hosts[0].host=alga.your-domain.com' \
  --set 'ingress.tls[0].hosts[0]=alga.your-domain.com'
```

To use your own database, cache, or queue instead of the included ones, set `postgresql.enabled=false`, `valkey.enabled=false`, or `rabbitmq.enabled=false` and give Alga the connection details. See `deploy/charts/alga/values.yaml` for all options.

### Check it's working

```sh
kubectl get pods -n alga
helm status alga -n alga
```

Open the address in your browser and fill in the setup wizard to create your admin account.

## Installing step by step (without Docker)

If you'd rather run each part yourself, you'll need:

- **Go** 1.27+
- **Node.js** 26+ with **pnpm** 12
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
- `ENCRYPTION_KEYS` — comma-separated `kid:base64(32B)` pairs (e.g. `1:$(openssl rand -base64 32)`); the highest `kid` is the active key
- `SECRET_PEPPER` — generate with `openssl rand -base64 32`

The admin account is not configured via `.env`. On first boot with no users in the database, open `http://localhost:3000` and complete the setup wizard to create the initial admin.

3. **Configure the frontend:**

```sh
cp apps/frontend/.env.example apps/frontend/.env
```

Set `VITE_API_BASE_URL` if the backend is on a different host (leave empty for Vite proxy).

4. **Start the database, cache, and queue** from the list above.

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

## Running in production

### Passwords Alga needs

When running live (`ENVIRONMENT=production`), Alga won't start without its two secret passwords (this protects your data):

- `ENCRYPTION_KEYS` — locks up private data like tokens
- `SECRET_PEPPER` — scrambles passwords so they can't be read

Your admin account is still created in the browser, not here. Open Alga and fill in the setup wizard the first time, then log in — the welcome guide will point you to the right pages.

Generate keys:

```sh
# Format: comma-separated kid:base64(32B) pairs; the highest kid seals new ciphertexts
export ENCRYPTION_KEYS="1:$(openssl rand -base64 32)"

# Key rotation: retain old kids for decryption, raise the kid to seal new data
# export ENCRYPTION_KEYS="1:<existing-key>,2:$(openssl rand -base64 32)"
```

### HTTPS

See [Deployment](/operations/deployment) for Caddy or nginx reverse proxy setup.

### How much computer power you need

| Deployment          | CPU     | Memory | Disk   |
| ------------------- | ------- | ------ | ------ |
| Development         | 2 cores | 4 GB   | 50 GB  |
| Production (Small)  | 4 cores | 8 GB   | 100 GB |
| Production (Medium) | 8 cores | 16 GB  | 200 GB |

## Next Steps

- [First Steps Guide](/getting-started/first-steps) — explore key features
- [Configuration](/configuration/environment-variables) — all environment variables
- [Integrations](/integrations/) — connect Slack, Mattermost, or Twilio
