---
title: Triage
description: Alga's rule-then-LLM triage pipeline classifies, prioritizes, and suppresses alert noise before it reaches a human — with feedback loops that improve accuracy.
---

# Triage

Alga's triage system evaluates correlated alerts and decides whether they should become incidents. It acts as the gate between alert correlation and incident creation — only alerts classified as incident-worthy proceed to the [incident management pipeline](/incident-management/lifecycle).

## How Triage Works

```
Correlated Alerts → Triage Rules (ordered) → Decision → Incident or Dismiss
```

1. **Correlation** groups related alerts within the `CORRELATION_WINDOW` (see [AI Investigation](./investigation.md))
2. **Triage engine** evaluates the correlated alert group against ordered triage rules
3. **First matching rule** determines the decision
4. **Result is stored** with confidence score, reasoning, and suggested actions
5. **If the decision is `investigate` or `escalate`**, an investigation is dispatched automatically (not an incident). Incidents are created separately by the incident worker for correlated critical-severity alerts.
6. **Operators can override** any triage decision after review

### Triage Decisions

| Decision | Description |
|----------|-------------|
| `investigate` | Dispatch an agent investigation (no incident created by triage itself) |
| `escalate` | Dispatch an investigation and trigger immediate escalation |
| `suppress` | Dismiss the alert group — no incident created |
| `auto_resolve` | Automatically resolve the alerts without incident |
| `enrich_only` | Enrich alert metadata without creating an incident |

> **Note (gated decisions):** The `auto_resolve` and `suppress` decisions are gated by the config flags `TRIAGE_AUTO_RESOLVE_ENABLED` and `TRIAGE_SUPPRESS_ENABLED` respectively. When a flag is disabled, that decision downgrades to `enrich_only`. Additionally, any decision whose confidence falls below `TRIAGE_CONFIDENCE_THRESHOLD` (default `0.7`) also downgrades to `enrich_only`.

### Triage Outcomes

Each triage result has an outcome that tracks operator review:

| Outcome | Description |
|---------|-------------|
| `pending` | Awaiting operator review |
| `confirmed` | Operator agreed with the automated decision |
| `overridden` | Operator changed the decision |

## Triage Rules

Triage rules are evaluated in priority order (lower priority number = evaluated first). The first matching rule determines the outcome.

### Rule Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Rule name (required) |
| `description` | string | Rule description |
| `conditions` | array | Condition objects matching against alert labels/annotations |
| `match_mode` | string | How conditions are combined: `"all"` or `"any"` |
| `decision` | string | Triage decision (`investigate`, `escalate`, `suppress`, `auto_resolve`, `enrich_only`) |
| `severity` | string | Severity to assign if an incident is created |
| `category` | string | Category label for grouping |
| `enrichment` | object | Additional data to attach to the result |
| `priority` | integer | Rule priority — lower values are evaluated first |
| `enabled` | boolean | Whether the rule is active |

### Creating a Rule

```bash
curl -X POST http://localhost:8080/api/v1/triage/rules \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $SESSION" \
  -d '{
    "name": "Critical production alerts",
    "description": "Escalate all critical alerts from production",
    "conditions": [
      {"field": "labels.severity", "operator": "exact", "value": "critical"},
      {"field": "labels.namespace", "operator": "exact", "value": "production"}
    ],
    "match_mode": "all",
    "decision": "escalate",
    "severity": "critical",
    "category": "infrastructure",
    "priority": 10,
    "enabled": true
  }'
```

### Ordering Rules

Rules are evaluated in priority order. Reorder them with a single call:

```bash
curl -X PUT http://localhost:8080/api/v1/triage/rules/reorder \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $SESSION" \
  -d '{"ids": ["rule-id-1", "rule-id-2", "rule-id-3"]}'
```

Rules are evaluated in the order specified. Use this to ensure high-priority rules are checked first.

### Listing Rules

```bash
# List all rules
curl http://localhost:8080/api/v1/triage/rules \
  -H "Authorization: Bearer $SESSION"

# Filter to enabled rules only
curl "http://localhost:8080/api/v1/triage/rules?enabled=true" \
  -H "Authorization: Bearer $SESSION"

# Search rules by name
curl "http://localhost:8080/api/v1/triage/rules?search=critical" \
  -H "Authorization: Bearer $SESSION"
```

## Triage Results

Every triage evaluation produces a result record that captures the decision, reasoning, and confidence score.

### Result Fields

| Field | Description |
|-------|-------------|
| `triage_number` | Unique sequential identifier |
| `correlation_key` | Correlation key of the alert group |
| `alert_count` | Number of alerts in the group |
| `alert_fingerprints` | Fingerprints of the grouped alerts |
| `alert_labels` | Merged labels from the alert group |
| `decision` | Automated triage decision |
| `confidence` | Confidence score (0.0 – 1.0) |
| `severity_classified` | Severity determined by triage |
| `category` | Category label |
| `reasoning` | Explanation of the decision |
| `suggested_actions` | Recommended next steps |
| `outcome` | Review status (`pending`, `confirmed`, `overridden`) |
| `overridden_to` | New decision if overridden |
| `model_used` | AI model used for classification (if applicable) |
| `triage_duration_ms` | Time taken to evaluate |
| `severity_input` | Original severity from the alert group before classification |
| `enrichment` | Additional data attached to the result |
| `context_used` | Context sources referenced during evaluation |
| `overridden_by` | User ID who overrode the decision |
| `overridden_at` | Timestamp when the override occurred |
| `trace_id` | Correlation trace ID for debugging |

### Viewing Results

```bash
# List all results
curl http://localhost:8080/api/v1/triage/results \
  -H "Authorization: Bearer $SESSION"

# Filter by decision
curl "http://localhost:8080/api/v1/triage/results?decision=suppress" \
  -H "Authorization: Bearer $SESSION"

# Filter by outcome
curl "http://localhost:8080/api/v1/triage/results?outcome=pending" \
  -H "Authorization: Bearer $SESSION"

# Filter by date range
curl "http://localhost:8080/api/v1/triage/results?start_date=2026-05-01&end_date=2026-05-10" \
  -H "Authorization: Bearer $SESSION"
```

### Overriding a Decision

Operators with the `triage:override` permission can change a triage decision:

```bash
curl -X POST http://localhost:8080/api/v1/triage/results/{id} \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $SESSION" \
  -d '{
    "decision": "investigate",
    "reason": "This alert group indicates a real outage"
  }'
```

### Triage Stats

The stats endpoint tracks accuracy over time:

```bash
curl http://localhost:8080/api/v1/triage/stats \
  -H "Authorization: Bearer $SESSION"
```

Response includes:

| Field | Description |
|-------|-------------|
| `total` | Total triage evaluations |
| `accuracy` | Percentage of confirmed vs overridden results |
| `by_decision` | Count breakdown by decision type |
| `by_category` | Count breakdown by category |
| `avg_confidence` | Average confidence score across all results |
| `avg_duration_ms` | Average evaluation time in milliseconds |
| `volume_trend_30d` | Daily volume counts for the last 30 days |

## CLI Commands

```bash
# Show triage accuracy and volume stats
./alga triage stats
```

Output:

```
Triage Stats:
  Total: 142 (confirmed: 128, overridden: 6, pending: 8)
  Accuracy: 95.5%
  By decision: map[investigate:89 suppress:38 escalate:15]
```

## API Endpoints

### Rules

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/triage/rules` | `triage:read` | List triage rules (supports `?search=`, `?enabled=true`, `?limit=`, `?skip=`) |
| `POST` | `/api/v1/triage/rules` | `triage:write` | Create triage rule |
| `GET` | `/api/v1/triage/rules/{id}` | `triage:read` | Get triage rule |
| `PUT` | `/api/v1/triage/rules/{id}` | `triage:write` | Update triage rule |
| `DELETE` | `/api/v1/triage/rules/{id}` | `triage:write` | Delete triage rule |
| `PUT` | `/api/v1/triage/rules/reorder` | `triage:write` | Reorder triage rules |

### Results

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/triage/results` | `triage:read` | List triage results (supports `?decision=`, `?outcome=`, `?category=`, `?severity=`, `?search=`, `?start_date=`, `?end_date=`, `?limit=`, `?skip=`) |
| `GET` | `/api/v1/triage/results/{id}` | `triage:read` | Get triage result |
| `POST` | `/api/v1/triage/results/{id}` | `triage:override` | Override triage decision |

### Stats

| Method | Path | Permission | Description |
|--------|------|------------|-------------|
| `GET` | `/api/v1/triage/stats` | `triage:read` | Get triage accuracy stats |

## Best Practices

- **Start with broad rules, then refine.** Begin with a few high-priority rules that catch obvious cases (e.g., suppress known noise), then add more specific rules as you observe triage results.
- **Order rules by specificity.** Place more specific rules (many conditions, `match_mode: "all"`) before catch-all rules (`match_mode: "any"`).
- **Review pending results regularly.** Use the `?outcome=pending` filter to find results that need operator confirmation. Confirming or overriding results improves accuracy tracking.
- **Use `suppress` for known noise.** Alerts from test environments, synthetic monitors, or known maintenance should be suppressed to avoid incident fatigue.
- **Assign categories.** Categories help you track triage patterns over time and identify areas where rules need tuning.
- **Monitor accuracy trends.** Watch the `accuracy` stat and `volume_trend_30d` to spot degradation. A dropping accuracy rate means your rules need updating.
- **Enrich rather than discard.** Prefer `enrich_only` over `suppress` when alerts might be useful later — enriched metadata is available for future triage evaluations.
- **Test rule changes in small batches.** Disable rules before editing them, then re-enable to avoid mid-evaluation inconsistencies.
