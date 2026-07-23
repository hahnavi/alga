---
title: Contributing to Alga
description: Contributor guide — development setup, code style, testing, PR process, architecture principles, and community.
---

# Contributing to Alga

Thank you for your interest in contributing to Alga! This guide covers everything you need to know to get started with development, submit changes, and maintain code quality.

## Table of Contents

- [Development Setup](#development-setup)
- [Code Style Guidelines](#code-style-guidelines)
- [Testing Requirements](#testing-requirements)
- [Pull Request Process](#pull-request-process)
- [Architecture Principles](#architecture-principles)
- [API Design Guidelines](#api-design-guidelines)
- [Documentation Contribution](#documentation-contribution)
- [Community Guidelines](#community-guidelines)

## Development Setup

### Prerequisites

**Required:**
- **Go 1.26.5+** - Backend language
- **Node.js 20+** - Frontend tooling (Vite 8, TypeScript ~6.0.3, vue-tsc 3 require Node 20+)
- **pnpm 8+** - Package manager
- **PostgreSQL 15+** - Database for development
- **Valkey/Redis 7+** - For session/caching features (optional but recommended)
- **RabbitMQ 3.12+** - For async pipeline (optional but recommended)

**Recommended:**
- **Docker & Docker Compose** - For local infrastructure
- **Git** - Version control

### Quick Setup

```bash
# Clone repository
git clone https://github.com/hahnavi/alga.git
cd alga

# Install dependencies
pnpm install

# Start local infrastructure (PostgreSQL, Valkey, RabbitMQ)
docker compose up -d

# Set up environment
cp apps/backend/.env.example apps/backend/.env
cp apps/frontend/.env.example apps/frontend/.env

# Edit apps/backend/.env to configure database and integrations
# Edit apps/frontend/.env to set API base URL

# Run database migrations
cd apps/backend
go run . db migrate

# Start development servers
cd ../..
moon run backend:dev   # Backend on :8080
moon run frontend:dev  # Frontend on :5173
```

### Backend Setup

```bash
cd apps/backend

# Install Go dependencies
go mod download

# Build
go build -o alga .

# Run tests
go test ./...
go test -v ./...

# Check formatting
gofmt -w .
gofmt -l .  # List unformatted files

# Run vet
go vet ./...

# Tidy dependencies
go mod tidy
```

**Ent code generation:** If you modify Ent schemas, regenerate the Ent client. The project exposes this via moon:

```bash
moon run backend:gen   # runs `go generate ./ent`
```

When running `go generate ./ent` directly, set `GOMEMLIMIT` and `GOMAXPROCS` to bound resource use during code generation (per the repo's AGENTS.md).

### Frontend Setup

```bash
# Install dependencies from the repo root (pnpm workspaces handle the frontend)
# Run this once from the project root: pnpm install

cd apps/frontend

# Development server
pnpm dev

# Build for production
pnpm build

# Type checking
pnpm typecheck

# Linting
pnpm lint

# Formatting
pnpm format:write
pnpm format  # Check only
```

## Code Style Guidelines

### Go Code Style

**Formatting:**
- Use `gofmt` for all Go code
- Run `gofmt -w .` before committing
- CI checks formatting automatically

**Imports:**
- Group imports in three blocks separated by blank lines:
  1. Standard library
  2. Third-party (`gopkg.in/yaml.v3`, `entgo.io/ent`, `github.com/...`)
  3. Internal packages (`alga/...`)

**Naming:**
- **Packages:** lowercase single words (`config`, `routing`, `logger`)
- **Exported types/functions:** PascalCase (`NewClient`, `RouteConfig`, `Engine`)
- **Unexported:** camelCase (`match`, `formatAlertMessage`, `handleWebhook`)
- **Constants:** PascalCase (`DEBUG`, `INFO`, `WARN`)
- **Variables:** camelCase (`routingEngine`, `mmClient`)

**Error Handling:**
- Always return `error` as the last return value
- Wrap errors with `fmt.Errorf("context: %w", err)`
- Fatal startup errors: `log.Fatalf`
- HTTP errors: `http.Error(w, msg, statusCode)` or `writeError(w, status, msg)`
- Use `writeInternalError(w, err, msg)` to log and return generic message

### Vue/TypeScript Code Style

**Formatting:**
- Use `oxfmt` for JavaScript/TypeScript formatting
- Run `pnpm format:write` before committing
- CI checks formatting automatically

**Linting:**
- Use `oxlint` for linting
- Run `pnpm lint` before committing
- CI runs linter in PR checks

**TypeScript:**
- Strict mode enabled
- Use `<script setup lang="ts">` for Vue components
- Type all props, emits, and function signatures
- Avoid `any` type; use `unknown` if truly unknown

**Component Organization:**
- Page components in `src/pages/*`
- Reusable UI components in `src/components/ui/*`
- Feature components in `src/components/*`
- Composables in `src/composables/*`
- Utility functions in `src/lib/*`
- Stores in `src/stores/*`

**API Calls:**
- Put API calls in `src/lib/api.ts`
- Avoid direct fetch in components
- Use typed interfaces for request/response
- Handle errors gracefully with toast notifications

**Styling:**
- Use Tailwind CSS v4 utility classes
- Avoid inline styles; use Tailwind classes
- For complex components, create utility classes in `app.css`

**Accessibility:**
- Add `aria-label` for icon-only buttons
- Use semantic HTML (`<button>`, `<input>`, `<nav>`)
- Support keyboard navigation
- Add loading indicators for async operations

### General Guidelines

- **Keep packages focused:** One responsibility per package
- **Avoid duplicate code:** Extract shared logic to utilities
- **Document public APIs:** Add godoc comments for exported functions
- **Write tests:** Aim for >80% coverage on critical paths
- **Security first:** Validate all inputs, sanitize outputs
- **Performance matters:** But prioritize readability
- **Version control:** Use conventional commits

## Testing Requirements

### Backend Tests

```bash
cd apps/backend

# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run with race detector
go test -race ./...
```

**Coverage Requirements:**
- Core packages (`api`): >80% coverage
- Integration packages (`mattermost`, `slack`): >60% coverage
- Utility packages (`logger`, `crypto`): >90% coverage

### Frontend Tests

```bash
# From project root
pnpm --filter frontend lint
pnpm --filter frontend typecheck
pnpm --filter frontend build
```

**Coverage Requirements:**
- Critical flows (authentication, alert creation): >70% coverage
- UI components: >50% coverage
- Utilities and composables: >80% coverage

### Pre-Commit Checks

**Automated Checks:**
- Go formatting (`gofmt`)
- Go vet (`go vet`)
- Go tests (`go test`)
- Frontend linting (`oxlint`)
- Frontend type checking (`tsc`)
- Frontend formatting (`oxfmt`)

**Run manually:**
```bash
# From project root
pnpm lint        # Frontend lint
pnpm typecheck   # Frontend typecheck
pnpm build       # Full build

cd apps/backend
go fmt ./...
go vet ./...
go test ./...
go mod tidy
```

## Pull Request Process

### Branch Naming

**Use conventional branch names:**
- `feature/` - New features
- `fix/` - Bug fixes
- `docs/` - Documentation changes
- `refactor/` - Code refactoring
- `test/` - Test additions/changes
- `chore/` - Maintenance tasks

**Examples:**
```
feature/add-agent-memory-system
fix/correct-escalation-timing
docs/update-api-documentation
refactor/simplify-routing-engine
```

### Commit Messages

**Follow Conventional Commits:**
```
<type>[optional scope]: <description>

[optional body]

[optional footer]
```

**Types:**
- `feat` - New feature
- `fix` - Bug fix
- `docs` - Documentation
- `style` - Formatting, style changes
- `refactor` - Code refactoring
- `test` - Test changes
- `chore` - Maintenance, dependencies
- `perf` - Performance improvements
- `ci` - CI/CD changes

**Examples:**
```
feat(api): add agent memory search endpoint

Add vector-based similarity search for agent memories using pgvector.

Fixes #123
```

### Review Process

**Before Submitting:**
1. Ensure all tests pass locally
2. Run linting and formatting checks
3. Update documentation if needed
4. Add tests for new functionality
5. Check for breaking changes
6. Review your own changes first

**Review Guidelines:**
- At least one approval from maintainers
- All CI checks must pass
- Address all review comments
- Squash commits if requested
- Update PR description based on feedback

### Merge Requirements

**Criteria for merging:**
- All CI checks pass
- At least one approval from maintainer
- No unresolved review comments
- Tests added for new features
- Documentation updated if API changed
- Breaking changes documented

## Architecture Principles

### Backend Architecture

**Packages as Modules:**
- Each package has a single responsibility
- Clear interfaces between packages
- Minimize circular dependencies
- Prefer dependency injection

**Layered Architecture:**
```
api/          # HTTP handlers, auth, SSE
store/        # Database persistence (Ent)
worker/       # Background consumers
routing/      # Alert routing logic
correlator/   # Alert correlation
escalation/   # Escalation engine
incident/     # Incident lifecycle
notification/ # Notification dispatcher
```

**Concurrency:**
- Use goroutines for concurrent operations
- Use channels for communication
- Use mutexes for shared state
- Use context for cancellation
- Avoid global mutable state

**Error Handling:**
- Wrap errors with context
- Don't panic in production code
- Log errors at appropriate levels
- Return errors to callers
- Handle errors gracefully

**Database:**
- Use Ent ORM for type-safe queries
- Use transactions for multi-step operations
- Index frequently queried fields
- Use connection pooling
- Handle database errors gracefully

### Frontend Architecture

**Component Hierarchy:**
```
App.vue
├── Sidebar.vue
├── UserMenuBar.vue
├── NotificationBell.vue
└── RouterView
    └── Pages (Dashboard, Alerts, Incidents, etc.)
        └── UI Components (Button, Card, Modal, etc.)
```

**State Management:**
- Use Pinia stores for global state (`auth`, `notifications`)
- Use local state in components for UI state
- Use composables for reusable logic
- Keep stores focused and small

**API Layer:**
- Single API client in `src/lib/api.ts`
- Type-safe request/response interfaces
- Centralized error handling
- Request/response interceptors
- Token management

**Routing:**
- Vue Router for page navigation
- Route guards for auth and permissions
- Lazy loading for code splitting
- Route parameters for dynamic pages

**Real-time Updates:**
- Server-Sent Events (SSE) for push updates
- Reconnect logic for resilience
- Event deduplication
- Cleanup on component unmount

### Cross-Cutting Concerns

**Security:**
- Validate all inputs
- Sanitize outputs
- Use prepared statements for SQL
- Implement CSRF protection
- Use HTTPS in production
- Never expose secrets in logs

**Logging:**
- Use structured logging where possible
- Log at appropriate levels (DEBUG, INFO, WARN, ERROR)
- Include context (request IDs, user IDs)
- Don't log sensitive data

**Performance:**
- Use connection pooling
- Cache frequently accessed data
- Optimize database queries
- Use pagination for large result sets
- Profile before optimizing

## API Design Guidelines

### REST API Design

**Resource Naming:**
- Use nouns, not verbs: `/alerts` not `/getAlerts`
- Use plural for collections: `/users` not `/user`
- Use kebab-case for URLs: `/escalation-policies`
- Nest logical resources: `/incidents/{id}/timeline`

**HTTP Methods:**
- `GET` - Retrieve resources
- `POST` - Create resources
- `PUT` - Replace resources (full update)
- `PATCH` - Partial updates
- `DELETE` - Remove resources

**Status Codes:**
- `200 OK` - Successful GET/PUT/PATCH
- `201 Created` - Successful POST
- `204 No Content` - Successful DELETE
- `400 Bad Request` - Invalid input
- `401 Unauthorized` - Not authenticated
- `403 Forbidden` - Authenticated but not authorized
- `404 Not Found` - Resource doesn't exist
- `409 Conflict` - Resource already exists
- `422 Unprocessable Entity` - Validation failed
- `429 Too Many Requests` - Rate limited
- `500 Internal Server Error` - Server error

**Error codes:**
- `VALIDATION_ERROR` - Input validation failed
- `NOT_FOUND` - Resource not found
- `UNAUTHORIZED` - Authentication required
- `FORBIDDEN` - Insufficient permissions
- `CONFLICT` - Resource conflict
- `RATE_LIMITED` - Too many requests
- `INTERNAL_ERROR` - Server error

### Versioning

**API Versioning:**
- Use URL path versioning: `/api/v1/alerts`
- Maintain backward compatibility where possible
- Document breaking changes in release notes
- Use semver for API versions

## Documentation Contribution

### Code Documentation

**Go Documentation:**
- Add godoc comments for exported functions, types, and packages
- Include examples for complex APIs
- Document parameters and return values
- Note any concurrency requirements

**TypeScript Documentation:**
- Use JSDoc comments for functions and complex types
- Document props and emits in Vue components
- Include examples for utility functions

### User Documentation

**Documentation Structure:**
- `docs/getting-started/` — Quick start guides and installation
- `docs/core-features/` — Feature documentation (alerts, investigations, agents)
- `docs/configuration/` — Configuration and security guides
- `docs/integrations/` — Integration setup guides (Slack, Mattermost, monitoring)
- `docs/incident-management/` — Incident lifecycle, SLA, escalation, post-mortems
- `docs/on-call/` — On-call schedules, handoffs, rotations
- `docs/service-management/` — Service catalog, dependencies, status
- `docs/operations/` — Performance, scaling, troubleshooting
- `docs/api-reference/` — REST API reference
- `docs/resources/` — FAQ, use cases, contributing

**Writing Guides:**
- Start with user goals, not features
- Use clear, simple language
- Include code examples
- Add screenshots for UI workflows
- Cross-reference related topics

**Updating Documentation:**
- Update docs when adding new features
- Update docs when breaking changes occur
- Keep examples up to date
- Review docs for clarity periodically

## Community Guidelines

### Code of Conduct

**Be Respectful:**
- Treat others with respect and professionalism
- Welcome newcomers and help them learn
- Assume good intentions

**Be Constructive:**
- Focus on what is best for community
- Provide constructive feedback
- Accept feedback gracefully

**Be Inclusive:**
- Respect different perspectives and experiences
- Use inclusive language
- Welcome contributions from everyone

### Getting Help

**Resources:**
- [Documentation](../getting-started/)
- [API Reference](../api-reference/)
- [GitHub Issues](https://github.com/hahnavi/alga/issues)
- [Discussions](https://github.com/hahnavi/alga/discussions)

**Asking Questions:**
- Search existing issues and discussions first
- Include relevant context and error messages
- Provide steps to reproduce bugs
- Use appropriate labels

### Reporting Issues

**Bug Reports:**
- Use the bug report template
- Include environment details (OS, Go version, etc.)
- Provide minimal reproduction case
- Include logs and error messages

**Feature Requests:**
- Describe the problem you're solving
- Explain why the feature is needed
- Suggest a solution approach
- Consider contributing the implementation

### Security

**Reporting Vulnerabilities:**
- Do not report publicly
- Email security@example.com
- Include details and reproduction steps
- Wait for confirmation before discussing publicly

**Security Best Practices:**
- Never commit secrets or credentials
- Use environment variables for configuration
- Follow security guidelines in code
- Report security vulnerabilities responsibly

## Recognition

**Contributors:**
- All contributors are recognized in the contributors list
- Significant contributions may be featured in release notes
- Maintain good commit hygiene

**Attribution:**
- Preserve author information when possible
- Credit contributors in relevant documentation
- Mention contributors in changelog for significant changes

## License

By contributing to Alga, you agree that your contributions will be licensed under the MIT License.

---

Thank you for contributing to Alga!
