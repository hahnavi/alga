---
name: alga-dev-environment
description: Use when setting up Alga locally, running dev servers, building, testing, formatting, migrating, or troubleshooting workspace commands.
priority: P1
tags: [devops, setup, build, test, operations]
---

# Alga Dev Environment

Run commands from the repository root unless noted. Prefer the smallest relevant command, then broaden when shared behavior changed.

## First-Time Setup

Create env files only if missing; do not overwrite local secrets or developer config.

```bash
cp -n .env.example .env
cp -n apps/backend/.env.example apps/backend/.env
cp -n apps/frontend/.env.example apps/frontend/.env
pnpm install --no-frozen-lockfile
docker compose up -d
```

If `cp -n` is unavailable, check whether the destination exists before copying.

## Dev Servers

```bash
moon run backend:dev
moon run frontend:dev
```

Manual alternatives:

```bash
cd apps/backend
go run .
```

```bash
cd apps/frontend
pnpm dev
```

Backend defaults to `:8080`; frontend Vite defaults to `:5173` and proxies `/api` and `/webhooks`.

## Backend Commands

Run from `apps/backend`:

```bash
go build -o alga .
go test ./...
go vet ./...
gofmt -w .
go mod tidy
go generate ./ent
```

Use targeted tests while iterating, for example `go test ./api ./store` or `go test ./worker ./rabbitmq`.

## Frontend Commands

Run from repo root:

```bash
pnpm --filter frontend typecheck
pnpm --filter frontend lint
pnpm --filter frontend format
pnpm --filter frontend format:write
pnpm --filter frontend build
```

## Verification Ladder

- API only: `cd apps/backend && go test ./api ./store`.
- Ent schema: `cd apps/backend && go generate ./ent && go test ./ent/... ./store`.
- Worker or RabbitMQ: `cd apps/backend && go test ./worker ./rabbitmq`.
- Frontend only: `pnpm --filter frontend typecheck && pnpm --filter frontend lint`.
- Shared backend: `cd apps/backend && go test ./... && go vet ./...`.
- Shared frontend or pre-commit confidence: `pnpm typecheck && pnpm lint && pnpm build`.

## Monorepo Commands

```bash
pnpm lint
pnpm build
pnpm typecheck
pnpm format
pnpm format:write
```

## CLI and Migrations

Run from `apps/backend` after building when using `./alga`:

```bash
go build -o alga .
./alga db migrate
./alga webhook-token generate <name>
./alga webhook-token list
./alga webhook-token revoke <id>
./alga alerts query '{"status": "firing"}'
```

`go run . db migrate` is the direct local migration alternative.

## Services

Docker Compose provides PostgreSQL, Valkey, RabbitMQ, backend, and frontend. Source-of-truth settings are root `.env.example`, `apps/backend/.env.example`, and `apps/frontend/.env.example`.

## Troubleshooting

- Check manifests for exact versions: `apps/backend/go.mod`, root `package.json`, and `apps/frontend/package.json`.
- If generated Ent code is stale, run `go generate ./ent` from `apps/backend` and then backend tests.
- If frontend types fail after API changes, update `apps/frontend/src/lib/api.ts` before pages/components.
- If services fail to connect, verify Docker Compose is running and env files were created without overwriting local values.
