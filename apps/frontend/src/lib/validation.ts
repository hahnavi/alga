import { z, type ZodTypeAny } from "zod";

// Runtime response validation for the Alga API client (W14).
//
// `request<T>` in `@/lib/api` unwraps the `{data}` envelope and casts the
// payload to `T`. That cast is compile-time only: a malformed response that
// diverges from the contract would be silently accepted and crash later in a
// component. These schemas let us check the wire shape at the boundary, right
// after parsing, and fail loudly (with `ResponseValidationError`) instead of
// propagating a half-valid object.
//
// Schemas are intentionally derived from the W2/W3 contract
// (`refs/plan/api-reference/api-contract.md`, `refs/plan/api-reference/openapi.yaml`)
// for the core envelopes and a few representative resource DTOs. They are NOT
// exhaustive — see `refs/plan/architecture/frontend-validation.md` for how to
// extend them to more endpoints.

// Error thrown when an API response fails its schema check.
export class ResponseValidationError extends Error {
  readonly issues: z.ZodIssue[];
  constructor(issues: z.ZodIssue[], message = "API response failed schema validation") {
    super(message);
    this.name = "ResponseValidationError";
    this.issues = issues;
  }
}

// Validate `data` against `schema`, throwing `ResponseValidationError` on
// mismatch. `data` is `unknown` — the only thing we trust from the network is
// bytes, never a type. The inferred schema type is returned so callers get a
// compile-time-narrowed value without an `any` cast.
export function validate<T extends ZodTypeAny>(schema: T, data: unknown): z.infer<T> {
  const result = schema.safeParse(data);
  if (!result.success) {
    throw new ResponseValidationError(result.error.issues);
  }
  return result.data;
}

// ---------------------------------------------------------------------------
// Core API envelopes (W2 §2)
// ---------------------------------------------------------------------------

// `{data: <resource>}` — single-resource success envelope.
export function successEnvelope<T extends ZodTypeAny>(resource: T) {
  return z.object({ data: resource });
}

// `{data: {items, total}, meta: {total}}` — paginated list envelope.
export function paginatedEnvelope<T extends ZodTypeAny>(item: T) {
  return z.object({
    data: z.object({
      items: z.array(item),
      total: z.number().int(),
    }),
    meta: z.object({ total: z.number().int() }),
  });
}

// The inner `{items, total}` payload after the paginated envelope is unwrapped
// (what `request<T>` already hands back for list endpoints).
export function paginatedData<T extends ZodTypeAny>(item: T) {
  return z.object({
    items: z.array(item),
    total: z.number().int(),
  });
}

// `{error: {code, message, details[]}}` — every non-2xx response (W2 §1).
export const errorEnvelope = z.object({
  error: z.object({
    code: z.string(),
    message: z.string(),
    details: z.array(z.object({ field: z.string(), message: z.string() })).optional(),
  }),
});

// ---------------------------------------------------------------------------
// Representative resource DTOs
// ---------------------------------------------------------------------------

// Alert event `type` and `source` are free-form, non-empty strings at the DB
// layer (apps/backend/db/models/alert_event.go); the backend emits values
// beyond any fixed enum (e.g. "acknowledged" alongside "acked", and sources
// like "triage_auto_resolve"). Accept any string here so the boundary check
// validates the wire shape rather than an incomplete enum catalog.
const alertEventSchema = z.object({
  type: z.string().min(1),
  timestamp: z.string(),
  actor_username: z.string().optional(),
  actor_display_name: z.string().optional(),
  actor_user_id: z.string().optional(),
  source: z.string().optional(),
});

const alertInvestigationSummarySchema = z.object({
  alert_investigation_id: z.string(),
  status: z.string(),
  agent_id: z.string().optional(),
  agent_name: z.string().optional(),
  // `agent_type` and `assignee_type` are free-form strings at the DB layer
  // (apps/backend/db/models/alert_investigation.go) with no enum constraint;
  // new agent/assignee types must not break the boundary check. Accept any
  // string here, mirroring the alert-event `type`/`source` policy below.
  agent_type: z.string().optional(),
  assignee_type: z.string().optional(),
  promoted_incident_id: z.string().optional(),
  promoted_incident_number: z.number().int().optional(),
});

export const alertRecordSchema = z.object({
  fingerprint: z.string(),
  alert_number: z.number().int().optional(),
  status: z.enum(["firing", "resolved"]),
  acknowledged: z.boolean().optional(),
  silenced: z.boolean().optional(),
  delivery_targets: z.array(z.unknown()).optional(),
  labels: z.record(z.string(), z.string()),
  annotations: z.record(z.string(), z.string()),
  // Backend serializes a nil map as JSON `null` (no `omitempty` on the Go
  // struct field).
  values: z.union([z.record(z.string(), z.unknown()), z.null()]).optional(),
  starts_at: z.string(),
  // Backend serializes a nil `*time.Time` as JSON `null` (no `omitempty`).
  ends_at: z.union([z.string(), z.null()]).optional(),
  generator_url: z.string().optional(),
  events: z.array(alertEventSchema).optional(),
  updated_at: z.string(),
  created_at: z.string(),
  deleted_at: z.union([z.string(), z.null()]).optional(),
  investigation: alertInvestigationSummarySchema.optional(),
});

const alertDetailSchema = z.object({
  alert: alertRecordSchema,
  alert_investigation: z.unknown().optional(),
});

export const incidentRecordSchema = z
  .object({
    id: z.string(),
    incident_number: z.number().int(),
    title: z.string(),
    description: z.string(),
    summary: z.string().optional(),
    status: z.enum([
      "detected",
      "triaging",
      "active",
      "mitigated",
      "resolved",
      "closed",
      "cancelled",
    ]),
    severity: z.enum(["critical", "high", "warning", "info"]),
    impact_level: z.enum(["high", "medium", "low"]),
    priority: z.enum(["P1", "P2", "P3", "P4", "P5"]),
    incident_type: z.string(),
    commander_id: z.string().optional(),
    service_id: z.string().optional(),
    // Backend serializes these with `omitempty` (apps/backend/store/incident.go),
    // so they are absent from the JSON whenever empty. Accept that wire shape
    // instead of failing the boundary check.
    conference_url: z.string().optional(),
    slack_channel_id: z.string().optional(),
    slack_channel_name: z.string().optional(),
    slack_channel_archived: z.boolean(),
    tags: z.array(z.string()).optional(),
    custom_fields: z.record(z.string(), z.unknown()).optional(),
    sla_target_respond_at: z.string().optional(),
    sla_target_resolve_at: z.string().optional(),
    started_at: z.string().optional(),
    mitigated_at: z.string().optional(),
    resolved_at: z.string().optional(),
    closed_at: z.string().optional(),
    created_at: z.string(),
    updated_at: z.string(),
    deleted_at: z.union([z.string(), z.null()]).optional(),
    // `.passthrough()` keeps backend-emitted fields the schema doesn't enumerate
    // (e.g. `google_meet_space_name`, `war_room_channel_id`, `ics_roles`,
    // `timeline`, `sla_acknowledged_at`). Zod's default is to strip unknown
    // keys, which silently breaks UI that reads those fields.
  })
  .passthrough();

// Schemas reused at the boundary in `api.ts`.
export const alertListSchema = z.array(alertRecordSchema).nullable();
export const alertDetailResponseSchema = alertDetailSchema;
export const incidentListSchema = paginatedData(incidentRecordSchema);
export const incidentDetailSchema = z.object({ incident: incidentRecordSchema }).passthrough();
