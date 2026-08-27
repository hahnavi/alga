---
title: Agent Memory & Vector Search
description: How Alga agents persist and semantically recall episodic memories using pgvector-backed vector search, with three memory types and LLM extraction.
---

# Agent Memory & Vector Search

Agent Memory is Alga's long-term learning system. When an agent investigates an alert and finds a root cause, that knowledge is extracted into structured memory entries, embedded as vectors, and recalled during future investigations — so the platform gets better at resolving incidents over time.

Memory is agent-agnostic: it works identically whether you use the native Alga Agent, Hermes, OpenClaw, or a custom agent.

## How It Works

```
Investigation completes
        │
        ▼
LLM extracts discrete facts, patterns, and procedures
        │
        ▼
Each memory is embedded as a vector (pgvector)
        │
        ▼
Future investigation → agent searches memories by semantic similarity
        │
        ▼
Relevant memories injected into the dispatch prompt
```

### Three Memory Types

| Type            | Description                                                     | Example                                                                      |
| --------------- | --------------------------------------------------------------- | ---------------------------------------------------------------------------- |
| **`fact`**      | A concrete piece of knowledge about a system or behavior        | "The payment-service connection pool leaks under load above 80%"             |
| **`pattern`**   | A recurring condition or relationship observed across incidents | "Database CPU spikes correlate with deploy windows on Fridays"               |
| **`procedure`** | A step-by-step remediation or diagnostic sequence               | "To clear the Redis cache deadlock: restart the sentinel, then flush node 3" |

The extraction LLM classifies each memory into one of these types and assigns a confidence score.

## Enabling Agent Memory

Memory is **off by default**. To enable it, configure these environment variables:

### Required for Auto-Extraction

| Variable             | Default       | Description                                                                                               |
| -------------------- | ------------- | --------------------------------------------------------------------------------------------------------- |
| `MEMORY_ENABLED`     | `false`       | Master switch — no memory service is wired without this                                                   |
| `MEMORY_LLM_URL`     |               | OpenAI-compatible chat-completions endpoint URL for extraction. **If empty, auto-extraction is disabled** |
| `MEMORY_LLM_API_KEY` |               | Bearer token for the extraction LLM                                                                       |
| `MEMORY_LLM_MODEL`   | `gpt-4o-mini` | Chat model used for extraction                                                                            |

### Required for Vector Search

| Variable                   | Default                  | Description                                                                                                                                    |
| -------------------------- | ------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `MEMORY_EMBEDDING_URL`     |                          | OpenAI-compatible embeddings endpoint. **If empty, a no-op embedder produces zero vectors** — search falls back to PostgreSQL full-text search |
| `MEMORY_EMBEDDING_API_KEY` |                          | Bearer token for the embedding service                                                                                                         |
| `MEMORY_EMBEDDING_MODEL`   | `text-embedding-3-small` | Embedding model — MUST be a 1536-dimension model; startup fails closed on anything else (the `vector(1536)` column is pinned; DT-E2)           |

::: tip Works with any OpenAI-compatible API
Both the LLM and embedding endpoints accept any OpenAI-compatible API — OpenAI, Ollama, vLLM, Azure OpenAI (with a compatible proxy), etc.
:::

### Optional Tuning

| Variable                       | Default | Description                                                                                                                                       |
| ------------------------------ | ------- | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| `MEMORY_AUTO_EXTRACT`          | `false` | Auto-extract on investigation completion (forced `true` when `MEMORY_LLM_URL` is set)                                                             |
| `MEMORY_MAX_PER_INVESTIGATION` | `10`    | Maximum memories extracted per investigation                                                                                                      |
| `MEMORY_SIMILARITY_THRESHOLD`  | `0`     | Minimum cosine similarity for search results (accepted and validated, but not currently applied as a filter — search returns top-K by similarity) |

## What Gets Extracted

When an investigation completes (with a root cause, resolution, or timeline updates), the extraction LLM receives:

- Observation date, status, agent name and type, correlation key
- Up to 5 alerts (name, namespace, service, deployment, summary)
- The investigation outcome (root cause, resolution, evidence, recommended actions)
- The last 20 timeline entries

The LLM is instructed to produce discrete, self-contained memory statements (15–80 words each, no pronouns, one fact per memory, no fabrication). Each extracted memory is deduplicated by SHA-256 hash of its content — so the same fact won't be stored twice.

## Semantic Search

When an agent searches memories, Alga:

1. Embeds the query text using the configured embedding model
2. Runs a pgvector cosine similarity search (`1 - (vec <=> query)`) ordered by similarity
3. Returns the top-K results (default 5)
4. Increments the `access_count` on each returned memory

If no embedding model is configured, or embedding fails, the search **falls back to PostgreSQL full-text search** (`ts_rank` with `plainto_tsquery`).

Expired memories (past their `expires_at`) are automatically excluded from search results and cleaned up by a background sweeper running every 5 minutes.

## Managing Memories

### Via the UI

The **Memory** page (brain icon in the sidebar) lets you browse, create, edit, and delete agent memories:

- **Search and filter** by text content and memory type (`fact`, `pattern`, `procedure`)
- **Create** memories manually with fields for type, content, correlation key, labels, confidence, and expiry
- **Edit** memory content (automatically re-embeds)
- **Delete** memories individually
- Each memory card shows the type badge, source agent, confidence, access count, labels, and timestamps

### Via the Operator API

| Method   | Endpoint                | Permission        | Description                                                                        |
| -------- | ----------------------- | ----------------- | ---------------------------------------------------------------------------------- |
| `GET`    | `/api/v1/memories`      | —                 | List/search memories (filters: `q`, `memory_type`, `agent_id`, `investigation_id`) |
| `POST`   | `/api/v1/memories`      | `memories:write`  | Create a memory manually                                                           |
| `GET`    | `/api/v1/memories/{id}` | —                 | Get a specific memory                                                              |
| `PUT`    | `/api/v1/memories/{id}` | `memories:write`  | Update memory content (re-embeds)                                                  |
| `DELETE` | `/api/v1/memories/{id}` | `memories:delete` | Delete a memory                                                                    |

### Via the Agent API

Agents interact with memories through their own bearer-scoped endpoints:

| Method   | Endpoint                      | Description                                                                                                                                                                                                                                |           |
| -------- | ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------- |
| `GET`    | `/api/v1/agent/memories`      | List/search memories (requires `investigate\                                                                                                                                                                                               | command`) |
| `POST`   | `/api/v1/agent/memories`      | Create a memory (requires `investigate`; auto-stamps the calling agent's ID)                                                                                                                                                               |           |
| `GET`    | `/api/v1/agent/memories/{id}` | Get a specific memory (requires `investigate\                                                                                                                                                                                              | command`) |
| `DELETE` | `/api/v1/agent/memories/{id}` | Delete — requires `investigate` AND ownership; memories with no owning agent (extraction output) can only be removed by an operator via the RBAC-gated `/api/v1/memories/{id}`, and every successful delete is audited as `memory_deleted` |           |

## Memory Fields

| Field              | Description                                                    |
| ------------------ | -------------------------------------------------------------- |
| `content`          | The memory text (required)                                     |
| `memory_type`      | `fact`, `pattern`, or `procedure` (default: `fact`)            |
| `agent_id`         | The agent that created or extracted this memory                |
| `investigation_id` | The investigation that produced this memory                    |
| `correlation_key`  | The correlation key of the source alert group                  |
| `labels`           | Key-value labels copied from the source alert                  |
| `entities`         | Named entities identified during extraction                    |
| `confidence`       | LLM-assigned confidence score (0–1, default 0.7)               |
| `access_count`     | How many times this memory has been returned in search results |
| `expires_at`       | Optional expiry — memories are auto-deleted after this time    |

## Best Practices

- **Start with auto-extraction** — let the LLM extract memories from real investigations rather than manually seeding. The extraction prompt is tuned to produce specific, self-contained facts.
- **Review extracted memories** periodically — delete inaccurate or outdated entries from the Memory page.
- **Use labels** — memories inherit labels from their source alert. This helps with filtering and contextual recall.
- **Set expiry on time-sensitive facts** — if a memory describes a temporary condition (e.g., a known issue pending a fix), set an `expires_at` so it's automatically cleaned up.
- **Monitor access_count** — frequently-accessed memories are high-value; rarely-accessed ones may be candidates for cleanup.

## See Also

- [AI Investigation](/core-features/investigation) — how memories are injected into dispatch prompts
- [Knowledge Base](/agents/knowledge-base) — operator-authored notes (complementary to agent-extracted memories)
- [Peer Ask](/agents/peer-ask) — agent-to-agent knowledge sharing
- [Environment Variables](/configuration/environment-variables) — full `MEMORY_*` configuration
