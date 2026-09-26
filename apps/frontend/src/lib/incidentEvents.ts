import type { IncidentTimelineRecord, OwnerThreadMessage } from "@/lib/api";
import { getAgentAvatarSrc } from "@/lib/agentAvatar";

/**
 * Shared SSE payload guards for incident-detail events. The backend publishes
 * several event families with inconsistently named incident keys
 * (`incident_number`, `incident_id`) or investigation-scoped keys
 * (`alert_investigation_id`, `investigation_id`), so every handler on the page
 * funnels through these guards instead of hand-rolling `as` casts.
 */

type OwnerThreadKind = "incident_inv" | "incident_coord";

/**
 * Matches events that name an incident, accepting both `incident_number`
 * (number or numeric string) and `incident_id` (the scheduler emits
 * `incident_id` holding the incident number, e.g. `ics_role_assigned`).
 */
export function isIncidentEvent(data: unknown): data is {
  incident_number?: number | string;
  incident_id?: string;
} {
  if (typeof data !== "object" || data === null) return false;
  const d = data as Record<string, unknown>;
  const num = d.incident_number;
  const id = d.incident_id;
  const hasNumber = typeof num === "number" || typeof num === "string";
  return hasNumber || typeof id === "string";
}

/** Normalizes an incident-scoped event payload to the incident number. */
export function incidentNumberFromEvent(data: {
  incident_number?: number | string;
  incident_id?: string;
}): number {
  if (typeof data.incident_number === "number") return data.incident_number;
  if (typeof data.incident_number === "string") return parseInt(data.incident_number, 10);
  return parseInt(data.incident_id ?? "", 10);
}

/** True when the event names the given incident. */
export function isForIncident(data: unknown, incidentNumber: number | string): boolean {
  if (!isIncidentEvent(data)) return false;
  return incidentNumberFromEvent(data) === Number(incidentNumber);
}

/** True when the event is an owner-thread event bound to the given thread. */
export function isOwnerThreadEvent(
  data: unknown,
  kind: OwnerThreadKind,
  incidentNumber: number | string,
): boolean {
  if (typeof data !== "object" || data === null) return false;
  const d = data as { owner_type?: string; owner_id?: string };
  return d.owner_type === kind && String(d.owner_id) === String(incidentNumber);
}

/**
 * True when the event belongs to an investigation on the given incident.
 * Incident-scoped investigation events carry `incident_number`; alert-scoped
 * ones carry `alert_investigation_id` and must not trigger an incident reload.
 */
export function isIncidentInvestigationEvent(
  data: unknown,
  incidentNumber: number | string,
): boolean {
  if (typeof data !== "object" || data === null) return false;
  const d = data as Record<string, unknown>;
  if ("alert_investigation_id" in d) return false;
  if (d.incident_number !== undefined) {
    return isForIncident(data, incidentNumber);
  }
  // `investigation_updated` from incident assign/create publishes the whole
  // investigation record; it names the incident via `incident_number` (set
  // above) or `incident_investigation_id`, which is not resolvable to a
  // number here — treat it as not matching rather than reloading every page.
  return false;
}

export type ThreadParticipant = {
  key: string;
  name: string;
  avatarSrc?: string;
};

export function addThreadParticipant(
  map: Map<string, ThreadParticipant>,
  key: string | undefined,
  fallbackKey: string,
  name: string,
  avatarSrc?: string,
): void {
  const normalizedName = name.trim() || "User";
  const normalizedKey = (key?.trim() || normalizedName).toLowerCase();
  if (!map.has(normalizedKey)) {
    map.set(normalizedKey, {
      key: fallbackKey,
      name: normalizedName,
      avatarSrc,
    });
  }
}

export function participantInitial(name: string): string {
  return name.trim().charAt(0).toUpperCase() || "U";
}

export function participantLabel(participants: ThreadParticipant[], fallback: string): string {
  if (participants.length === 0) return fallback;
  if (participants.length === 1) return participants[0].name;
  const others = participants.length - 1;
  return `${participants[0].name} and ${others} ${others === 1 ? "other" : "others"}`;
}

/** Participants derived from an owner-thread (investigation) message list. */
export function ownerThreadParticipants(messages: OwnerThreadMessage[]): ThreadParticipant[] {
  const map = new Map<string, ThreadParticipant>();
  for (const message of messages) {
    const isAgent = message.source === "agent";
    const avatar = isAgent ? getAgentAvatarSrc(message.agent_type) : undefined;
    addThreadParticipant(
      map,
      message.user_id ?? ownerThreadDisplayName(message),
      message.id,
      ownerThreadDisplayName(message),
      avatar,
    );
  }
  return [...map.values()];
}

export function ownerThreadDisplayName(message: OwnerThreadMessage): string {
  if (message.username?.trim()) return message.username.trim();
  if (message.source === "agent") return "Agent";
  if (message.source === "system") return "System";
  if (message.source === "slack") return "Slack";
  if (message.source === "mattermost") return "Mattermost";
  return "User";
}

/**
 * Timeline entries visible on the incident page: `investigation_created`
 * rows are agent-internal bookkeeping and are hidden from the count and the
 * list (see IncidentTimeline). Keeps the page's badge count in sync with the
 * rendered list.
 */
export function visibleTimelineEntries(
  entries: IncidentTimelineRecord[],
): IncidentTimelineRecord[] {
  return entries.filter((e) => e.event_type !== "investigation_created");
}
