import type { RouteCondition } from "@/lib/api";

/**
 * Human-readable summary of a single route/selector condition.
 * Shared by RoutesPage, KnowledgePage, and AgentsPage.
 */
export function summarizeCondition(
  condition: Pick<RouteCondition, "source" | "field" | "operator" | "value">,
): string {
  if (condition.operator === "exists" || condition.operator === "not_exists") {
    return `${condition.source}.${condition.field} ${condition.operator}`;
  }
  return `${condition.source}.${condition.field} ${condition.operator} "${condition.value ?? ""}"`;
}

// Display labels are Capitalized; values stay the wire-format operator/source names.
export const CONDITION_SOURCE_OPTIONS = [
  { value: "labels", label: "Labels" },
  { value: "annotations", label: "Annotations" },
  { value: "alert", label: "Alert" },
] as const;

export const CONDITION_OPERATOR_OPTIONS = [
  { value: "exact", label: "Exact" },
  { value: "contains", label: "Contains" },
  { value: "prefix", label: "Prefix" },
  { value: "suffix", label: "Suffix" },
  { value: "wildcard", label: "Wildcard" },
  { value: "regex", label: "Regex" },
  { value: "exists", label: "Exists" },
  { value: "not_exists", label: "Not Exists" },
] as const;
