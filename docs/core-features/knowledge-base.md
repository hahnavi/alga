---
title: Knowledge Base
description: Shared operator-curated notes with label selectors, episodic past-investigation knowledge, and concurrent investigation awareness — all queried by agents during investigations.
---

# Knowledge Base

Alga's knowledge base provides shared notes that help both operators and AI agents investigate faster.

## How It Works

The knowledge system aggregates three sources:

| Source | Description | Storage |
|--------|-------------|---------|
| **Episodic** | Similar past investigations | PostgreSQL |
| **Notes** | Operator-curated notes with tags and TTL | PostgreSQL |
| **Concurrent** | Currently-running investigations on the same correlation key | Valkey |

Agents access all three sources via `/api/v1/agent/knowledge` during investigations.

## Managing Notes

### Create a Note

Go to **Knowledge** → **Create Note** or use the API:

```sh
curl -b cookies.txt -X POST http://localhost:8080/api/v1/knowledge \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Database Connection Pool Tuning",
    "body_markdown": "When seeing connection timeout errors on the API, check the connection pool settings...",
    "tags": ["database", "connection-pool", "api"],
    "selectors": [{"field": "namespace", "operator": "exact", "value": "production"}, {"field": "service", "operator": "exact", "value": "api-server"}]
  }'
```

### Note Fields

| Field | Description |
|-------|-------------|
| `kind` | Note category (e.g., operator note vs episodic knowledge) |
| `title` | Human-readable title |
| `body_markdown` | Markdown content with details, steps, and context |
| `tags` | Labels for categorization and search |
| `selectors` | Array of condition objects (e.g., `[{"field": "namespace", "operator": "exact", "value": "production"}]`) for matching alert labels |
| `author_id` / `author_type` / `author_name` | Who authored the note (`author_type` is `user` or `agent`) |
| `source_investigation_id` | Investigation the note was derived from (for agent-authored notes) |
| `confidence` | Optional confidence score (0–1) for agent-authored notes |
| `expires_at` | Optional expiry timestamp (RFC3339 format, auto-deletes after this time) |

### Search Notes

```sh
curl -b cookies.txt "http://localhost:8080/api/v1/knowledge?q=database&tag=production"
```

## Selectors

Selectors let you attach notes to specific alert contexts. When an agent investigates an alert with matching labels, the knowledge system automatically retrieves relevant notes.

For example, a note with selectors `[{"field": "namespace", "operator": "exact", "value": "production"}, {"field": "service", "operator": "exact", "value": "api-server"}]` will be suggested when investigating alerts from the `api-server` service in the `production` namespace.

## Episodic Knowledge

Past investigations are automatically indexed by:
- Correlation key (deployment name + alertname)
- Alert labels and annotations
- Root cause and resolution text

Agents query similar past investigations to find patterns and reuse solutions.

## API Endpoints

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/knowledge` | — | List/search notes |
| `POST` | `/api/v1/knowledge` | `knowledge:write` | Create note |
| `GET` | `/api/v1/knowledge/{id}` | — | Get note |
| `PUT` | `/api/v1/knowledge/{id}` | `knowledge:write` | Update note |
| `DELETE` | `/api/v1/knowledge/{id}` | `knowledge:delete` | Delete note |

### Agent Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/v1/agent/knowledge` | Bearer | List/search notes (read-only) |
| `POST` | `/api/v1/agent/knowledge` | Bearer | Create note (requires `source_investigation_id` and `confidence` fields) |
