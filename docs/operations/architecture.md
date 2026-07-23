---
title: System Architecture
description: Alga's full-stack monorepo architecture — components, data flow, RabbitMQ topology, Valkey coordination, scalability, high availability, and security.
---

# System Architecture

## Overview

Alga is a full-stack monorepo architecture designed for high availability, scalability, and production-grade security. The system ingests alerts from Grafana and other monitoring systems, routes notifications to Mattermost or Slack, persists alert state in PostgreSQL (via Ent ORM), and orchestrates an SRE agent investigation pipeline backed by RabbitMQ and Valkey.

## Components

### Backend (Go)
- **Location:** `apps/backend/`
- **Version:** Go 1.26.5
- **Module:** `alga`

The backend provides:
- REST API for all operations
- SSE (Server-Sent Events) for real-time updates
- Webhook ingestion for alerts
- Agent SSE endpoint for bidirectional communication
- Background workers for async processing
- Authentication, authorization, and security

### Frontend (Vue)
- **Location:** `apps/frontend/`
- **Framework:** Vue 3 + Vite + Tailwind CSS v4

The frontend provides:
- Operations console
- Role-based access control
- Real-time SSE updates
- In-app notifications
- Shared knowledge base
- Incident management system

### Infrastructure

#### PostgreSQL
- **Purpose:** Primary persistence layer
- **Access:** Via Ent ORM and pgx driver
- **Features:**
  - Alert state and history
  - User sessions and auth data
  - Integration credentials (encrypted)
  - Investigation records
  - Incident data
  - Audit logs
  - Knowledge base
  - Service catalog
  - On-call schedules
  - Escalation policies
  - Post-mortems

#### Valkey/Redis
- **Purpose:** Distributed state, caching, and coordination
- **Features:**
  - Session storage (preferred)
  - Alert deduplication
  - Rate limiting
  - Agent presence tracking
  - Leader election
  - Pub/sub for SSE fan-out
  - SLA tracking (dedup keys)
  - On-call caching
  - Correlation windows
  - Agent grace locks

#### RabbitMQ
- **Purpose:** Async message queue with retry topology
- **Features:**
  - Alert processing
  - Notification dispatch
  - Audit logging
  - SRE agent investigations
  - Three retry levels with TTL backoff
  - Dead-letter queue for failed messages

## Data Flow

### Alert Lifecycle

```
┌─────────────────┐
│  Alert Source   │
│  (Grafana, etc.) │
└────────┬────────┘
         │ POST /webhooks/alerts
         ▼
┌─────────────────┐
│  Webhook Handler │
│  (Bearer Auth)   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Routing Engine  │
│  (Rule Matching) │
└────────┬────────┘
         │
         ├───► Matched Rule ──► Multi-target Dispatch
         │                         ├──► Mattermost
         │                         └──► Slack
         │
         └───► Default Destinations
                                   ├──► Mattermost
                                   └──► Slack
         │
         ▼
┌─────────────────┐
│  RabbitMQ Queue  │
│  (alert processing)│
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Alert Worker   │
│  (Store + Notify)│
└────────┬────────┘
         │
         ├───► PostgreSQL (persist alert)
         │
         └───► Notifications (Mattermost/Slack/Email/In-app)
         │
         ▼
┌─────────────────┐
│  Correlator     │
│  (Group related alerts)│
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Investigation  │
│  (Auto-create)  │
└────────┬────────┘
         │
         └───► RabbitMQ (investigate queue)
```

### Investigation Lifecycle

```
┌─────────────────┐
│  Correlator     │
│  (Correlated alerts)│
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  RabbitMQ        │
│  (investigate)   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Scheduler       │
│  (Filter/Score/Bind)│
└────────┬────────┘
         │
         ├───► Agent Presence Check (Valkey)
         │
         ├───► Capacity Check (max concurrent)
         │
         └───► Bind to Agent (atomic claim)
         │
         ▼
┌─────────────────┐
│  Agent SSE       │
│  (Dispatch)      │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  SRE Agent       │
│  (Hermes/OpenClaw)│
└────────┬────────┘
         │
         ├───► Query knowledge base
         │
         ├───► Playbook Enrichment (matched procedures)
         │
         ├───► Fetch related alerts
         │
         ├───► Send messages/comments
         │
         └───► Complete/Cancel/Update
         │
         ▼
┌─────────────────┐
│  Investigation  │
│  (State Update) │
└────────┬────────┘
         │
         └───► PostgreSQL + SSE (notify frontend)
```

### Incident Lifecycle

```
┌─────────────────┐
│  Triage Worker  │
│  (Classify alerts)│
└────────┬────────┘
         │
         ▼ (incident-worthy)
┌─────────────────┐
│  IncidentWorker │
│  (Create incident)│
└────────┬────────┘
         │
         ├───► Link alerts/investigations
         │
         ├───► Resolve on-call responder
         │
         ├───► Assign escalation policy
         │
         ├───► Compute SLA targets
         │
         ├───► ICS Role Assignment (IC, Operations Lead, Comms Lead)
         │
         └───► Slack Channel Provisioning (war room)
         │
         ▼
┌─────────────────┐
│  State Machine  │
│  detected → triaging → active → mitigated → resolved → closed
└─────────────────┘
         │
         ├───► Acknowledge (stop escalation, start SLA)
         │
         ├───► Mitigate (update status, propagate to services)
         │
         ├───► Resolve (compute MTTR, update SLA)
         │
         ├───► Close (compute MTTM, final metrics)
         │
         └───► Escalate (policy-driven multi-tier)
         │
         ▼
┌─────────────────┐
│  Escalation Engine│
│  (Policy-driven) │
└────────┬────────┘
         │
          ├───► PostgreSQL (escalation state)
         │
         ├───► Multi-tier levels with delays
         │
         ├───► Targets: user/team/on-call
         │
         └───► Acknowledge stops escalation
```

## Incident Coordination

### Coordination Stream

Incidents expose a real-time **coordination stream** — a chronological message feed where responders, agents, and automated systems post updates. Messages are pushed via SSE to all participants and persisted as part of the incident record.

### Status Updates

Incidents support **structured status updates** with explicit phases (e.g., *investigating*, *identified*, *monitoring*). Each update is timestamped and attributed to its author, creating an authoritative timeline of the response narrative.

### Incident Documents

Responders can create **incident documents** — structured notes attached to an incident for capturing real-time observations, decision rationale, and action items. Documents support Markdown formatting and are versioned for traceability.

### ICS Role Assignments

Alga supports **ICS (Incident Command System) role assignments** on each incident:

- **Incident Commander (IC):** Overall authority for the incident response
- **Operations Lead:** Manages tactical response actions
- **Communications Lead:** Handles internal and external communications
- Custom roles can be added as needed

Roles are assigned via the incident roles API and are visible on the incident detail page. Role changes are recorded in the incident timeline.

### IC Handoff

The Incident Commander role supports **handoff** — transferring IC authority from one responder to another. Handoffs are explicitly recorded in the incident timeline with timestamps and acknowledgments to ensure clear ownership at all times.

## Playbooks

Playbooks provide **structured response procedures** that guide SRE agents and human responders through standardized investigation and mitigation steps.

### Playbook Types

| Type | Purpose |
|------|---------|
| **Procedure** | Step-by-step instructions for investigation and diagnosis |
| **Mitigation** | Predefined remediation actions for known failure modes |

### Automatic Matching

Playbooks are matched to investigations automatically during scheduling via **label selectors**. When an investigation is dispatched, the scheduler evaluates playbook selectors against the investigation's alert labels and attaches matching playbooks. This ensures agents receive relevant procedural context without manual intervention.

## RabbitMQ Topology

RabbitMQ uses the following exchanges to route messages through the async pipeline:

| Exchange | Purpose |
|----------|---------|
| `alga.alerts` | Alert webhook processing |
| `alga.notifications` | Alert notification delivery |
| `alga.audit` | Audit log persistence |
| `alga.investigate` | Investigation dispatch |
| `alga.triage` | Alert triage |
| `alga.incidents` | Incident creation |
| `alga.escalation` | Escalation execution |
| `alga.sla` | SLA breach detection |
| `alga.email` | Email delivery |
| `alga.notification-dispatch` | Per-user notification dispatch |
| `alga.ics-provision` | ICS war room provisioning |
| `alga.dlx` | Dead letter collection |

Each exchange routes to dedicated queues with retry topology (three retry levels with TTL backoff before dead-lettering).

## Technology Stack

| Component | Technology | Version | Purpose |
|-----------|-----------|---------|---------|
| **Backend** | Go | 1.26.5 | REST API, webhooks, workers |
| **Frontend** | Vue | 3 | Operations console |
| **Frontend** | Vite | Latest | Build tooling |
| **Frontend** | Tailwind CSS | v4 | Styling |
| **Database** | PostgreSQL | Latest | Primary persistence |
| **Database ORM** | Ent | Latest | Schema & query builder |
| **Database Driver** | pgx | v5 | PostgreSQL driver |
| **Database Extensions** | pgvector | Latest | Vector search for agent memory |
| **Cache** | Valkey/Redis | Latest | Distributed state, caching |
| **Message Queue** | RabbitMQ | Latest | Async processing with retries |
| **Web Server** | net/http | Go stdlib | HTTP server |
| **Session Storage** | Valkey/PostgreSQL | Preferred: Valkey | Session management |
| **Encryption** | AES-256-GCM | Go crypto | Integration secrets |
| **Password Hashing** | Argon2id | golang.org/x/crypto | Password security |
| **Auth** | Session + CSRF | Custom | Session-based auth |
| **RBAC** | Custom | - | Role-based access control |
| **Real-time** | SSE + Valkey pub/sub | Custom | Server-sent events |
| **Monitoring** | expvar | Go stdlib | Metrics endpoint |
| **Frontend Rich Text** | TipTap | @tiptap/vue-3 | Markdown editor |
| **Frontend Charts** | Chart.js | vue-chartjs | Data visualization |

## Scalability

### Horizontal Scaling

Alga is designed to scale horizontally when Valkey and RabbitMQ are configured:

- **Stateless backend:** Multiple backend replicas can run simultaneously
- **Shared infrastructure:** All replicas share PostgreSQL, Valkey, and RabbitMQ
- **Leader election:** Scheduler leader election ensures only one replica runs scheduling ticks
- **Agent presence:** Valkey-based presence tracking works across replicas
- **SSE fan-out:** Valkey pub/sub enables cross-replica SSE event distribution

### Vertical Scaling

- **CPU:** Alert processing, investigation scoring, encryption/decryption
- **Memory:** Session caches, routing engine, correlation windows
- **I/O:** PostgreSQL queries, Valkey operations, RabbitMQ message processing
- **Network:** SSE connections, webhook ingestion, API requests

### Resource Recommendations

| Deployment | CPU | Memory | Disk | Notes |
|------------|-----|--------|------|-------|
| Development | 2 cores | 4GB | 50GB | Single replica |
| Production (Small) | 4 cores | 8GB | 100GB | 2 replicas |
| Production (Medium) | 8 cores | 16GB | 200GB | 3-4 replicas |
| Production (Large) | 16+ cores | 32GB+ | 500GB+ | 5+ replicas |

## High Availability

### Leader Election

- **Mechanism:** Valkey lease (`alga:scheduler:leader`)
- **TTL:** `SCHEDULER_LEADER_TTL` (default: 15s)
- **Behavior:** Only the leader runs the Filter/Score/Bind tick
- **Failover:** Automatic takeover on TTL expiry
- **Single replica:** When Valkey is absent, each replica acts as leader automatically; TTL=0 is overridden to 15s

### Agent Disconnect Grace

- **Purpose:** Prevent premature investigation reset during network blips
- **Mechanism:** Per-agent Valkey `SET NX` lock
- **Grace period:** `AGENT_DISCONNECT_GRACE` (default: 45s)
- **Behavior:**
  - `delegated` work: reset immediately
  - `investigating` work: wait grace period, then reset
  - Reconnect anywhere: bypass grace period

### Atomic Claims

- **Investigation binding:** `ClaimPendingInvestigationByID`
- **Mechanism:** Ent `UpdateOneID` with status filter
- **Guarantees:**
  - Status must be `pending`
  - Per-agent capacity ceiling enforced
  - No double-binding across replicas
  - Concurrent ticks cannot race

### Idempotent Queue

- **Deduplication:** Valkey `SET NX` with TTL (~5min)
- **Authoritative:** Unique index on `investigations.investigation_id`
- **Guarantees:**
  - Duplicate `InvestigateMessage` ignored
  - Database constraint prevents duplicates
  - Failed messages retried with exponential backoff
  - Dead-letter queue for final failures

### Retry Topology

RabbitMQ retry queues with three levels:

1. **Immediate retry:** Short delay (30s)
2. **Short backoff:** Medium delay (2min)
3. **Long backoff:** Long delay (5min)
4. **Dead-letter:** Final failure, manual intervention required

### Session Management

- **Valkey preferred:** Atomic Lua scripts for refresh token rotation
- **PostgreSQL fallback:** When Valkey unavailable
- **Session secrets:** HMAC-SHA-256(`SECRET_PEPPER`, value)
- **Storage:** Only hashed values persisted (plaintext never stored)
- **Security:** Constant-time comparison on all checks

### SSE Presence Tracking

- **Mechanism:** Valkey hash (`alga:agent-presence:<agentID>`)
- **TTL:** `AGENT_PRESENCE_TTL` (default: 90s)
- **Renewal:** Heartbeat every ~30s
- **Cross-replica:** Any replica can answer "is this agent online?"
- **Fallback:** Local connection map when Valkey offline

### Peer Findings Fan-out

- **Mechanism:** Valkey pub/sub channel (`alga:peer-findings`)
- **Behavior:**
  - Agents publish findings to channel
  - All replicas subscribe and re-broadcast via SSE
  - Peers react to findings across cluster

### SLA Tracking

- **Mechanism:** `SET NX` with 24h TTL on keys like `alga:sla:breach:<incidentID>:<breachType>` for deduplication
- **Sweep:** Periodic sweep detects breaches
- **Actions:** Breach triggers escalation + timeline events

### Correlation Windows

- **Mechanism:** Valkey LIST + atomic Lua script
- **Behavior:**
  - Alerts with same correlation key within window are merged
  - "Append-and-set-TTL" ensures windowing honored across replicas
  - `CORRELATION_WINDOW` controls grouping time
  - `CORRELATION_COOLDOWN_TTL` prevents duplicate investigations

## Security Architecture



### Authentication

- **Method:** Session-based with HTTP-only cookies
- **Cookies:**
  - `alga_session`: Session identifier (hashed)
  - `alga_csrf`: CSRF token (double-submit pattern)
- **Password hashing:** Argon2id (m=64MiB, t=3, p=2)
- **Pre-hash:** HMAC with `SECRET_PEPPER` before Argon2id
- **Account lockout:** 5 failed attempts = 30-minute cooldown
- **Rate limiting:** 5 attempts per 15-minute window

### Authorization

- **Model:** Role-Based Access Control (RBAC)
- **Roles:**
  - `admin`: Full access
  - `operator`: Read/write alerts, investigations, knowledge; read routes/integrations
  - `viewer`: Read-only across all resources
- **Permissions:** Per-route capability checks
- **CSRF protection:** Double-submit cookie pattern
- **Secure cookies:** HttpOnly, Secure (HTTPS), SameSite=Strict

### Encryption

- **Algorithm:** AES-256-GCM
- **Keyring:** Versioned (`ENCRYPTION_KEYS`)
- **Rotation:** In-place rotation of historical ciphertexts
- **Protected:**
  - Mattermost webhook secret
  - Slack bot token
  - Slack signing secret
  - Twilio auth token
  - Hermes platform token
- **Fail-closed:** Requires key in production environment

### Token Security

- **Webhook tokens:** HMAC-SHA-256 + non-secret prefix
- **Agent tokens:** HMAC-SHA-256 + non-secret prefix
- **Session IDs:** HMAC-SHA-256(`SECRET_PEPPER`, value)
- **Refresh tokens:** HMAC-SHA-256(`SECRET_PEPPER`, value)
- **Storage:** Only hashed values persisted
- **Validation:** Constant-time comparison
- **Reuse detection:** Refresh token family tracking

### Audit Logging

- **Scope:** All auth, alert, investigation, knowledge, and peer-ask events
- **Storage:** PostgreSQL `audit_logs` collection
- **Events:** 80+ event types covering all system actions
- **Integrity:** Immutable records with timestamps

## Production Deployment Patterns

### Single Replica

- **Use case:** Development, small deployments
- **Configuration:**
  - Set `SCHEDULER_LEADER_TTL=0`
  - Valkey optional (session PostgreSQL fallback)
  - RabbitMQ still recommended for async processing
- **Limitations:** No HA, no cross-replica coordination

### Multi-Replica with Shared Infrastructure

- **Use case:** Production, high availability
- **Requirements:**
  - Valkey/Redis required (shared across replicas)
  - RabbitMQ required (shared across replicas)
  - PostgreSQL required (shared across replicas)
  - Load balancer in front of backends
- **Benefits:**
  - HA: Replicas can fail without downtime
  - Scalability: Add replicas as needed
  - Coordination: Leader election, presence tracking work across replicas

### Database High Availability

- **PostgreSQL:**
  - Use streaming replication for read scale-out
  - Use connection pooling (PgBouncer) for connection management
  - Consider failover mechanisms (Patroni, repmgr)
- **Valkey/Redis:**
  - Use Redis Sentinel for HA
  - Use Redis Cluster for sharding (if needed)
- **RabbitMQ:**
  - Use mirrored queues for HA
  - Use cluster for scaling

## Monitoring and Observability

### Metrics

- **expvar endpoint:** `/debug/vars`
- **Readiness endpoint:** `/api/v1/readiness` (JSON snapshot)
- **Metrics:**
  - `alga_correlator_alerts_total`: Total alerts received
  - `alga_correlator_alerts_merged_total`: Alerts merged into existing windows
  - `alga_correlator_published_total`: Investigations published after correlation
  - `alga_correlator_dropped_total`: Alerts dropped
  - `alga_correlator_fail_closed_total`: Fail-closed events
  - `alga_correlator_windows_opened_total`: New correlation windows opened
  - `alga_correlator_flushes_total`: Windows flushed (expired)
  - `alga_scheduler_*`: Scheduler and agent stats
  - `alga_scheduler_nudge_total`: Scheduler nudge events
  - `alga_webhook_alert_publish_queued_total`: Webhook alerts queued for publishing
  - `alga_webhook_alert_publish_sync_fallback_total`: Webhook alerts published via sync fallback
  - `alga_webhook_alert_publish_sync_processed_total`: Webhook alerts processed synchronously

### Health Checks

- **Liveness:** `/health` (webhook receiver response)
- **Readiness:** `/api/v1/readiness` (pipeline status)
- **Components checked:**
  - Database connectivity
  - Valkey connectivity (if configured)
  - RabbitMQ connectivity (if configured)
  - Scheduler/correlator snapshots

### Logging

- **Levels:** DEBUG, INFO, WARN, ERROR, FATAL
- **Output:** Stdout (configurable file output)
- **Format:** Plain text (`[LEVEL] message`) via Go standard log package, regardless of environment
- **Context:** Request IDs, user IDs, investigation IDs

## Conclusion

Alga's architecture emphasizes:

- **High availability:** Leader election, graceful degradation, retry topology
- **Scalability:** Horizontal scaling with shared infrastructure
- **Security:** Production-grade auth, encryption, audit logging
- **Reliability:** Atomic operations, idempotent processing, graceful failures
- **Observability:** Metrics, health checks, structured logging

The system is designed to run in production environments with strict security requirements while maintaining developer ergonomics for on-premises deployments.
