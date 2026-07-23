---
title: Use Cases
description: Practical use cases for DevOps, SRE, incident response, AI agents, dependency management, on-call, compliance, and multi-tenancy teams.
---

# Use Cases

Alga is a flexible, open-source alert management and incident response platform designed for modern DevOps and SRE teams. Here are the primary use cases and real-world scenarios where Alga excels.

## DevOps/SRE Team Alert Management

### Centralized Alert Ingestion

**Problem:** Your infrastructure monitoring tools (Prometheus, Grafana, Datadog, etc.) generate alerts across multiple channels, making it hard to maintain visibility and consistency.

**Alga Solution:** Alga provides a unified webhook endpoint that ingests alerts from any source, normalizes them, and routes them based on configurable rules.

```yaml
# Example: Route Prometheus alerts
rules:
  - name: Critical Database Alerts
    conditions:
      - field: labels.alertname
        operator: contains
        value: database
      - field: labels.severity
        operator: equals
        value: critical
    destinations:
      - channel: "#incidents-db"
        provider: slack
      - channel: "ops-team"
        provider: mattermost
```

### Intelligent Alert Routing

**Problem:** Alerts flood general channels, causing noise fatigue and making it hard for the right teams to respond.

**Alga Solution:** Use provider-aware routing to send alerts to the right teams and channels based on labels, annotations, and content.

- **Label-based routing:** Route by `team`, `service`, `environment`, `severity`
- **Content matching:** Use regex, prefix, suffix matching on alert titles/annotations
- **Multi-destination:** Send to both Slack and Mattermost simultaneously
- **Fallback defaults:** Configure default channels for unmatched alerts

### Alert Correlation and Deduplication

**Problem:** Multiple related alerts fire for the same underlying issue, creating noise and confusion.

**Alga Solution:** Alga's correlator groups related alerts within a configurable time window:

```bash
# Configure correlation window
CORRELATION_WINDOW=5m
CORRELATION_COOLDOWN_TTL=30m
```

Alerts sharing the same `deployment`, `statefulset`, `daemonset`, or `job` (combined with `namespace` and `alertname`) within the window are automatically correlated and can be investigated together.

## Slack-First Alert Management

### Native Slack Integration

**Problem:** Your team lives in Slack, but traditional alerting tools force context switching to separate dashboards.

**Alga Solution:** Alga provides bidirectional Slack integration:

- **Alert delivery:** Alerts posted directly to Slack channels with rich formatting
- **Command processing:** Use Slack slash commands to acknowledge, resolve, and investigate
- **Thread conversations:** Investigation discussions happen in alert threads
- **Mention support:** `@user` and `@group` mentions create notifications and escalate to the right people

### Mobile-Friendly Alert Response

**Problem:** On-call engineers need to respond from mobile devices without complex workflows.

**Alga Solution:** Alga's Slack integration is optimized for mobile:

- Quick actions via buttons (Acknowledge, Resolve, Investigate)
- Rich notifications with severity badges
- Real-time updates via SSE
- Voice notifications via Twilio integration for critical alerts

### Enterprise Slack Features

**Problem:** Large organizations have complex Slack workspaces with many channels and teams.

**Alga Solution:** Alga supports enterprise Slack features:

- **OAuth installation:** Team members can install the Alga Slack app to their workspace
- **Multiple workspaces:** Connect to multiple Slack workspaces
- **Channel discovery:** Auto-discover and list available channels
- **Granular permissions:** Respect Slack's permission model

## Incident Response Workflows

### Structured Incident Lifecycle

**Problem:** Incidents lack structure, leading to inconsistent response and poor post-incident learning.

**Alga Solution:** Alga implements a formal incident lifecycle:

```
open → active → mitigated → resolved → closed
```

(with `cancelled` and `reopened` branches)

Each transition is:
- Logged to the incident timeline
- Triggers notifications and escalations
- Updates service status and dependencies
- Captures context for post-mortems

### SLA Tracking and Enforcement

**Problem:** Teams struggle to meet response and resolution SLAs without visibility and alerts.

**Alga Solution:** Built-in SLA tracking with automatic breach detection:

```yaml
# Service-level SLA configuration
sla_target_respond_minutes: 15  # Time to first response
sla_target_resolve_minutes: 60  # Time to resolution
```

Alga automatically:
- Tracks SLA deadlines in sorted sets
- Detects breaches and fires escalation events
- Records SLA compliance metrics
- Generates reports for compliance and improvement

### Escalation Policies

**Problem:** On-call engineers don't respond, and there's no automated escalation path.

**Alga Solution:** Multi-tier escalation policies with configurable delays:

```yaml
# Example escalation policy
policy:
  - delay: 5m
    targets:
      - type: on_call
        schedule_id: "primary-oncall"
  - delay: 10m
    targets:
      - type: team
        team_id: "platform-team"
  - delay: 15m
    targets:
      - type: user
        user_id: "engineering-manager"
```

Escalation stops automatically when the incident is acknowledged.

### Structured Incident Response with ICS

**Problem:** During major incidents, roles are unclear, communication is chaotic, and critical tasks fall through the cracks.

**Alga Solution:** Alga implements the Incident Command System (ICS) framework, adapted from emergency management for production incidents:

- **Automatic role assignment:** When an incident is created, Alga assigns ICS roles based on the affected service's on-call schedule and escalation policy:
  - **Incident Commander** — Overall coordination and decision-making
  - **Operations Lead** — Technical remediation and execution
  - **Scribe** — Timeline documentation, decisions, and action items
  - **Liaison** — Stakeholder communication and status updates

- **Role-based permissions:** Each ICS role has specific capabilities within the incident (e.g., only the IC can declare an incident resolved, only the Scribe can create timeline entries)

- **IC handoffs:** When the Incident Commander's shift ends, Alga facilitates a structured handoff — transferring command authority to the next on-call responder with full context

- **Scribe documentation:** The Scribe role ensures all decisions, actions, and communications are captured in the incident timeline, providing a complete record for post-mortems

- **Clear accountability:** Every action and decision is attributed to a role, eliminating confusion about who is responsible for what during high-pressure incidents

## Automated Investigation with AI Agents

### Agent-Powered Triage and Diagnosis

**Problem:** Alert volume is too high for manual investigation, leading to slow response times.

**Alga Solution:** SRE agents (powered by Hermes + OpenClaw) automatically investigate alerts:

- **Triage:** Classify alert severity and impact
- **Diagnosis:** Analyze logs, metrics, and recent changes
- **Root cause analysis:** Identify likely root causes with evidence
- **Resolution suggestions:** Provide actionable fix recommendations

### Peer-to-Peer Agent Collaboration

**Problem:** Complex incidents require knowledge that spans multiple systems and teams.

**Alga Solution:** Agents can ask each other for help via peer-ask:

```typescript
// Agent can request peer assistance
await agent.peerAsk({
  topic: "database.performance",
  context: { query: "slow_query_123" },
  urgency: "high"
});
```

This enables distributed problem-solving across your agent fleet.

### Memory and Knowledge Integration

**Problem:** Solutions to past problems aren't captured or reused effectively.

**Alga Solution:** Agent memory system with vector search:

- **Automatic extraction:** Extract lessons learned from investigations
- **Vector similarity:** Find relevant past incidents via semantic search
- **Knowledge base:** Curate operational knowledge for agents
- **Continuous learning:** Agents get smarter over time

### Playbook-Driven Remediation

**Problem:** Known alert patterns require the same manual steps every time, leading to inconsistent remediation and repeated human effort.

**Alga Solution:** Alga's playbooks provide automated, structured response procedures matched to incoming alerts:

- **Automatic matching:** When an investigation starts, Alga evaluates all playbooks' label selectors against the alert's labels. If a playbook matches, its step-by-step procedure is immediately attached to the investigation, giving responders (both human and AI agents) a clear remediation path.

- **Step-by-step procedures:** Each playbook defines an ordered list of steps — diagnostic checks, remediation actions, verification steps, and escalation triggers. Steps can include conditional branching based on investigation findings.

- **Label-based selectors:** Playbooks are matched using flexible label selectors (exact match, contains, regex) on alert labels like `alertname`, `service`, `severity`, `namespace`, or any custom label. This means playbooks can be broad ("all database alerts") or specific ("critical replication lag on the payments database").

- **AI agent integration:** When an AI agent (Hermes/OpenClaw) receives an investigation with an attached playbook, it follows the defined steps rather than diagnosing from scratch. This dramatically reduces investigation time for known alert patterns.

- **Continuous improvement:** After each playbook execution, teams can review the outcome, refine steps, and add new playbooks based on recurring patterns identified through the knowledge base and memories.

## Service Dependency Impact Analysis

### Service Catalog and Dependency Graph

**Problem:** Outages cascade through services, but it's hard to understand the full impact.

**Alga Solution:** Built-in service catalog with dependency tracking:

```yaml
services:
  - id: "api-gateway"
    name: "API Gateway"
    dependencies:
      - "user-service"
      - "payment-service"
      - "notification-service"
  - id: "payment-service"
    name: "Payment Service"
    dependencies:
      - "database-primary"
      - "database-replica"
```

### Automatic Cascade Detection

**Problem:** When a service fails, it's unclear which downstream services are affected.

**Alga Solution:** Automatic dependency cascade on incidents:

- When a service is in a major outage, dependent services are automatically marked as degraded
- Cascade events are added to incident timelines
- Alerts from affected services are correlated to the root incident

### Service Status Propagation

**Problem:** Service status is manually updated and often out of date.

**Alga Solution:** Automatic status propagation:

- Incidents automatically update service status
- Dependencies propagate status changes
- Real-time status dashboard across all services
- Health checks and readiness endpoints

## On-Call Management

### Rotation and Schedule Management

**Problem:** On-call schedules are complex, with multiple teams and coverage requirements.

**Alga Solution:** Flexible on-call scheduling:

- **Rotations:** Daily, weekly, custom rotation periods
- **Layers:** Multiple on-call layers (primary, secondary, escalation)
- **Overrides:** Temporary coverage changes and swaps
- **Follow-the-sun:** Geographic handoffs between time zones
- **Vacation handling:** Automatic skip during time off

### On-Call Notifications

**Problem:** On-call engineers miss notifications or get overwhelmed by noise.

**Alga Solution:** Intelligent notification routing:

- **Per-user preferences:** Each user controls their notification channels
- **Quiet hours:** Respect on-call engineer's sleep schedule
- **Severity filtering:** Only notify for relevant severity levels
- **Multi-channel:** Send to Slack, email, SMS, voice calls
- **Reminders:** Shift start reminders (default 15 minutes before)

### Post-Incident Handoff

**Problem:** Post-incident context gets lost during on-call handoffs.

**Alga Solution:** Rich incident context for handoffs:

- Complete incident timeline
- Linked alerts and investigations
- Agent findings and recommendations
- Service impact analysis
- Post-mortem action items

## Compliance and Auditing

### Comprehensive Audit Trail

**Problem:** Regulatory requirements demand complete audit trails of incident response.

**Alga Solution:** All actions are logged to PostgreSQL audit logs:

```json
{
  "event": "incident_escalated",
  "user_id": "user-123",
  "incident_id": "456",
  "timestamp": "2026-05-10T14:30:00Z",
  "details": {
    "previous_level": 1,
    "new_level": 2,
    "reason": "no_response"
  }
}
```

### Incident Metrics and Reporting

**Problem:** Teams lack data to improve their incident response processes.

**Alga Solution:** Built-in metrics and reporting:

- **MTTA (Mean Time To Acknowledge):** Time to first response
- **MTTR (Mean Time To Resolve):** Time to resolution
- **MTTM (Mean Time To Mitigate):** Time to mitigation
- **SLA compliance:** Response and resolution SLA percentages
- **Escalation frequency:** How often incidents escalate
- **Agent effectiveness:** Agent contribution to resolution

### Post-Mortem Workflow

**Problem:** Post-mortems are ad-hoc and often skipped.

**Alga Solution:** Structured post-mortem process:

- **Template-driven:** Standardized post-mortem templates
- **Review workflow:** Draft → Review → Approve → Publish
- **Action items:** Track follow-up tasks with owners and due dates
- **Knowledge base:** Store post-mortems for future reference

## Multi-Tenancy and Scalability

### Team Isolation

**Problem:** Multiple teams share the platform but need data isolation.

**Alga Solution:** Role-based access control and team isolation:

- **Three built-in roles:** admin, operator, viewer
- **Team-scoped data:** Incidents, by service, by on-call schedule
- **Granular permissions:** Per-capability access control
- **Audit logging:** All actions traced to users

### Horizontal Scalability

**Problem:** Alert volume grows beyond the capacity of a single instance.

**Alga Solution:** Horizontal scaling with shared infrastructure:

- **Valkey/Redis:** Shared state across replicas
- **RabbitMQ:** Distributed message queue with retry topology
- **Leader election:** Scheduler runs on one replica, fails over automatically
- **SSE fan-out:** Cross-replica event distribution
- **Database:** PostgreSQL with connection pooling via pgx

### High Availability

**Problem:** Downtime during incidents is unacceptable.

**Alga Solution:** Built-in HA patterns:

- **Graceful shutdown:** Drains connections before shutdown
- **Leader failover:** Scheduler leadership transfers on TTL expiry
- **Connection resilience:** Retry with exponential backoff
- **State persistence:** All state in PostgreSQL, recoverable from restarts

## Getting Started with Your Use Case

### Quick Start for Common Scenarios

1. **DevOps team:** Start with webhook ingestion and Slack routing
2. **On-call team:** Configure on-call schedules and escalation policies
3. **Incident manager:** Set up incident workflows and post-mortem templates
4. **SRE team:** Enable agent investigations and memory system
5. **Platform team:** Build service catalog and dependency graphs
6. **Incident commander:** Enable ICS roles and playbook-driven remediation

### Example Configurations

See the [Configuration Guide](/configuration/environment-variables) for detailed setup examples for each use case.

### Community Use Cases

Join the [Alga community](https://github.com/hahnavi/alga/discussions) to learn how other teams are using Alga and share your own use cases.
