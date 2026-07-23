/** Display severity/priority from common Grafana-style labels. */
import type { AlertRecord, Severity } from "@/lib/api";

const SEVERITY_RAIL_CLASS: Record<"critical" | "high" | "warning" | "default", string> = {
  critical: "border-r-[var(--border-primary)] bg-red-500 text-white",
  high: "border-r-[var(--border-primary)] bg-orange-500 text-white",
  warning: "border-r-[var(--border-primary)] bg-amber-500 text-white",
  default: "border-r-[var(--border-primary)] bg-[var(--bg-tertiary)] text-[var(--text-primary)]",
};

const SEVERITY_RAIL_EMPTY =
  "border-r-[var(--border-primary)] bg-[var(--bg-secondary)] text-transparent";

export function severityRailClass(severity: string | null): string {
  if (severity == null) return SEVERITY_RAIL_EMPTY;
  return SEVERITY_RAIL_CLASS[severityBucket(severity)];
}

const SEVERITY_RANK: Record<string, number> = {
  critical: 4,
  high: 3,
  warning: 2,
  info: 1,
};

const STATUS_RANK: Record<string, number> = {
  firing: 1,
  resolved: 0,
};

export function severityRank(labels: Record<string, string> | undefined): number {
  return SEVERITY_RANK[(labels?.severity ?? "").toLowerCase()] ?? 0;
}

function compareAlerts(a: AlertRecord, b: AlertRecord, field: string): number {
  switch (field) {
    case "created_at":
      return new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
    case "severity":
      return severityRank(a.labels) - severityRank(b.labels);
    case "status":
      return (STATUS_RANK[a.status ?? ""] ?? 0) - (STATUS_RANK[b.status ?? ""] ?? 0);
    case "updated_at":
    default:
      return new Date(a.updated_at).getTime() - new Date(b.updated_at).getTime();
  }
}

/** Returns a new array sorted by the sortBy field. `sortBy` may be prefixed with `-` for descending. */
export function sortAlerts(alerts: AlertRecord[], sortBy: string): AlertRecord[] {
  if (!sortBy) return alerts;
  const desc = sortBy.startsWith("-");
  const field = desc ? sortBy.slice(1) : sortBy;
  return [...alerts].sort((a, b) => {
    const cmp = compareAlerts(a, b, field);
    return desc ? -cmp : cmp;
  });
}

export const INCIDENT_STATUS_TABS = [
  { value: "all", label: "All statuses" },
  { value: "active", label: "Active" },
  { value: "triaging", label: "Triaging" },
  { value: "mitigated", label: "Mitigated" },
  { value: "resolved", label: "Resolved" },
  { value: "closed", label: "Closed" },
] as const;

export const INCIDENT_PRIORITY_TABS = [
  { value: "all", label: "All priorities" },
  { value: "P1", label: "P1" },
  { value: "P2", label: "P2" },
  { value: "P3", label: "P3" },
  { value: "P4", label: "P4" },
  { value: "P5", label: "P5" },
] as const;

export function alertSeverityLabel(labels: Record<string, string> | undefined): string | null {
  if (!labels) return null;
  const v =
    labels.severity || labels.priority || labels.Severity || labels.Priority || labels.level || "";
  const t = v.trim();
  return t ? t : null;
}

/**
 * Single source of truth for the four severity buckets (critical / high /
 * warning / default). Both `severityBadgeClass` and `severityBorderColor`
 * route through this so the gradient stays consistent. Free-form label
 * values fall back via substring matching.
 */
function severityBucket(
  level: Severity | string | null | undefined,
): "critical" | "high" | "warning" | "default" {
  const raw = (level ?? "").toString().toLowerCase();
  if (
    raw.includes("critical") ||
    raw.includes("fatal") ||
    raw.includes("emergency") ||
    raw.includes("error")
  )
    return "critical";
  if (raw.includes("high")) return "high";
  if (raw.includes("warning") || raw.includes("warn") || raw.includes("medium")) return "warning";
  return "default";
}

const SEVERITY_BADGE_CLASS: Record<ReturnType<typeof severityBucket>, string> = {
  critical: "badge-red",
  high: "badge-orange",
  warning: "badge-yellow",
  default: "badge-green",
};

const SEVERITY_BORDER_VAR: Record<ReturnType<typeof severityBucket>, string> = {
  critical: "var(--text-badge-firing)",
  high: "var(--text-badge-firing)",
  warning: "var(--text-badge-warning)",
  default: "var(--text-badge-resolved)",
};

/**
 * Single source of truth for severity → badge color. Maps the four `Severity`
 * literals (`critical`/`high`/`warning`/`info`) to the four-step gradient used
 * for both alert labels and incident severity, and falls back to the same
 * colors via substring matching for free-form label values.
 */
export function severityBadgeClass(level: Severity | string): string {
  return SEVERITY_BADGE_CLASS[severityBucket(level)];
}

export function investigationStatusBadgeClass(status: string): string {
  switch (status) {
    case "pending":
    case "paused":
      return "badge-purple";
    case "investigating":
      return "badge-blue";
    case "complete":
      return "badge-green";
    case "failed":
    case "cancelled":
      return "badge-red";
    case "timed_out":
      return "badge-red";
    case "assigned":
      return "badge-purple";
    default:
      return "badge-muted";
  }
}

export function investigationStatusLabel(status: string): string {
  return labelFrom(
    {
      pending: "Pending",
      assigned: "Assigned",
      investigating: "Investigating",
      complete: "Completed",
      failed: "Failed",
      cancelled: "Cancelled",
      timed_out: "Timed Out",
      paused: "Paused",
    },
    status,
  );
}

const HEADER_LABEL_KEYS = new Set([
  "alertname",
  "severity",
  "priority",
  "Severity",
  "Priority",
  "level",
]);

/**
 * Look up a human-friendly label for an enum-like status string, falling back
 * to the raw value when no override is registered. Centralizes the
 * `switch`-of-strings → Title Case pattern previously repeated for
 * investigation/incident/service statuses.
 */
function labelFrom(map: Record<string, string>, key: string): string {
  return map[key] ?? key;
}

export function nonHeaderLabelKeys(labels: Record<string, string> | undefined): string[] {
  if (!labels) return [];
  return Object.keys(labels).filter((k) => !HEADER_LABEL_KEYS.has(k));
}

export function nonHeaderLabelEntries(
  labels: Record<string, string> | undefined,
): { key: string; value: string }[] {
  if (!labels) return [];
  return Object.entries(labels)
    .filter(([k]) => !HEADER_LABEL_KEYS.has(k))
    .map(([key, value]) => ({ key, value }));
}

export function alertCombinedStatus(alert: { status: string; acknowledged?: boolean }): {
  badge: string;
  label: string;
} {
  if (alert.status === "resolved") return { badge: "badge-green", label: "Resolved" };
  if (alert.acknowledged) return { badge: "badge-yellow", label: "Acknowledged" };
  return { badge: "badge-red", label: "Firing" };
}

export function severityBorderColor(severity: string | null | undefined): string {
  if (!severity) return "var(--border-primary)";
  return SEVERITY_BORDER_VAR[severityBucket(severity)];
}

export function incidentPriorityBadgeClass(priority: string): string {
  switch (priority) {
    case "P1":
      return "badge-red";
    case "P2":
      return "badge-orange";
    case "P3":
      return "badge-yellow";
    case "P4":
      return "badge-blue";
    case "P5":
      return "badge-gray";
    default:
      return "badge-gray";
  }
}

type StatusEntry = { badge: string; label: string };

const INCIDENT_STATUS_DISPLAY: Record<string, StatusEntry> = {
  detected: { badge: "badge-purple", label: "Detected" },
  triaging: { badge: "badge-purple", label: "Triaging" },
  active: { badge: "badge-red", label: "Active" },
  mitigated: { badge: "badge-orange", label: "Mitigated" },
  resolved: { badge: "badge-green", label: "Resolved" },
  closed: { badge: "badge-muted", label: "Closed" },
  cancelled: { badge: "badge-muted", label: "Cancelled" },
};

const SERVICE_STATUS_DISPLAY: Record<string, StatusEntry> = {
  operational: { badge: "badge-green", label: "Operational" },
  degraded: { badge: "badge-yellow", label: "Degraded" },
  partial_outage: { badge: "badge-orange", label: "Partial Outage" },
  major_outage: { badge: "badge-red", label: "Major Outage" },
};

const POST_MORTEM_STATUS_DISPLAY: Record<string, StatusEntry> = {
  draft: { badge: "badge-muted", label: "Draft" },
  in_review: { badge: "badge-yellow", label: "In Review" },
  approved: { badge: "badge-blue", label: "Approved" },
  published: { badge: "badge-green", label: "Published" },
};

const ACTION_ITEM_TYPE_DISPLAY: Record<string, StatusEntry> = {
  prevent: { badge: "badge-blue", label: "Prevent" },
  mitigate: { badge: "badge-yellow", label: "Mitigate" },
  detect: { badge: "badge-purple", label: "Detect" },
  investigate: { badge: "badge-muted", label: "Investigate" },
};

function statusDisplay(map: Record<string, StatusEntry>, key: string): StatusEntry {
  return map[key] ?? { badge: "badge-muted", label: key };
}

export function incidentStatusDisplay(status: string): StatusEntry {
  return statusDisplay(INCIDENT_STATUS_DISPLAY, status);
}

export function serviceStatusDisplay(status: string): StatusEntry {
  return statusDisplay(SERVICE_STATUS_DISPLAY, status);
}

export function postMortemStatusDisplay(status: string): StatusEntry {
  return statusDisplay(POST_MORTEM_STATUS_DISPLAY, status);
}

export function actionItemTypeDisplay(type: string): StatusEntry {
  return statusDisplay(ACTION_ITEM_TYPE_DISPLAY, type);
}

export function incidentStatusBadgeClass(status: string): string {
  return incidentStatusDisplay(status).badge;
}

export function incidentStatusLabel(status: string): string {
  return incidentStatusDisplay(status).label;
}

export function serviceStatusBadgeClass(status: string): string {
  return serviceStatusDisplay(status).badge;
}

export function serviceStatusLabel(status: string): string {
  return serviceStatusDisplay(status).label;
}

export function postMortemStatusBadgeClass(status: string): string {
  return postMortemStatusDisplay(status).badge;
}

export function actionItemTypeBadgeClass(type: string): string {
  return actionItemTypeDisplay(type).badge;
}

export function incidentPriorityBorderColor(priority: string): string {
  switch (priority) {
    case "P1":
      return "var(--color-red-500, #ef4444)";
    case "P2":
      return "var(--color-orange-500, #f97316)";
    case "P3":
      return "var(--color-yellow-500, #eab308)";
    case "P4":
      return "var(--color-blue-500, #3b82f6)";
    case "P5":
      return "var(--color-gray-500, #6b7280)";
    default:
      return "var(--color-gray-500, #6b7280)";
  }
}
