# Contributing to Alga

Thanks for your interest in contributing! This guide covers development setup, code style, testing, and the PR process.

## Development Setup

1. **Clone the repository**

   ```bash
   git clone https://github.com/hahnavi/alga.git
   cd alga
   ```

2. **Install dependencies**

   ```bash
   pnpm install --no-frozen-lockfile
   ```

3. **Start infrastructure**

   ```bash
   cp .env.example .env
   cp apps/backend/.env.example apps/backend/.env
   docker compose up -d postgres valkey rabbitmq
   ```

4. **Run the backend**

   ```bash
   moon run backend:dev
   ```

5. **Run the frontend**

   ```bash
   moon run frontend:dev
   ```

   The frontend runs at `http://localhost:5173` and proxies API requests to the backend on port 8080.

## Code Style

### Go (Backend)

- Run `gofmt -w .` before committing
- Run `go vet ./...` to catch common issues
- Run `go mod tidy` after adding or removing dependencies
- Use standard library `testing` for tests

### Frontend (Vue 3 + TypeScript)

- Run `oxfmt` for formatting and `oxlint` for linting
- Use Vue 3 Composition API with `<script setup lang="ts">`
- Use Pinia stores for shared state
- Use shared primitives from `src/components/ui/*`

### Imports

Group imports in three blocks separated by blank lines:

1. Standard library
2. Third-party packages
3. Internal packages (`alga/...`)

See [AGENTS.md](AGENTS.md) for detailed conventions.

## Testing

### Backend

```bash
cd apps/backend
go test ./...
go test -v ./...
```

### Frontend

```bash
pnpm --filter frontend typecheck
pnpm --filter frontend lint
```

### Full monorepo checks

```bash
pnpm lint
pnpm typecheck
pnpm build
```

## PR Process

1. **Fork** the repository
2. **Create a branch** from `main`
3. **Make your changes** with clear, focused commits
4. **Open a Pull Request** against `main`

### Requirements

- CI must pass (lint, typecheck, build)
- Use [conventional commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, `chore:`, etc.)
- Keep PRs focused — one concern per PR
- Update documentation if your change affects behavior

## Reporting Issues

Use the [GitHub issue templates](https://github.com/hahnavi/alga/issues/new/choose) to report bugs or request features.

## Agent Skills

Guidance for AI agents lives in `AGENTS.md` (always-loaded) and `.agents/skills/` (loaded on demand). When changing skills, run the bundled linter to validate frontmatter, line limits, referenced paths, and required `validation/` artifacts:

```bash
python3 .agents/skills/scripts/audit.py
```
