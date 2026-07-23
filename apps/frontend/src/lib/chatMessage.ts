import type { IncidentCoordinationMessage, OwnerThreadMessage } from "@/lib/api";

/** Messages that participate in grouping/dedup/display. */
export type ChatMessage = OwnerThreadMessage | IncidentCoordinationMessage;

/** Source strings shared by OwnerThreadMessage and IncidentCoordinationMessage. */
export type ChatSource = ChatMessage["source"];

/**
 * Display name for a chat message. OwnerThread carries `username`; incident
 * coordination messages carry `actor_display_name`. Falls back to a role
 * label and finally "User"/"Responder".
 */
export function displayName(message: ChatMessage): string {
  if ("actor_display_name" in message && message.actor_display_name) {
    return message.actor_display_name;
  }
  if ("username" in message && message.username) return message.username;
  if (message.source === "agent") return "Agent";
  if (message.source === "system") return "System";
  if (message.source === "slack") return "Slack user";
  return "Responder";
}

/** Avatar background tint per source. */
export function sourceAvatarBg(source: ChatSource | string): string {
  switch (source) {
    case "agent":
      return "bg-transparent";
    case "system":
      return "bg-blue-100 dark:bg-blue-900/30";
    case "slack":
      return "bg-emerald-600";
    default:
      return "bg-[var(--bg-secondary)]";
  }
}

/** Incident-coordination avatar background (slight palette differences). */
export function avatarBg(message: IncidentCoordinationMessage): string {
  if (message.actor_type === "agent") return "bg-transparent";
  if (message.actor_type === "system") return "bg-slate-600";
  if (message.source === "slack") return "bg-emerald-600";
  return "bg-blue-600";
}

/** First letter of the display name, used as a fallback avatar initial. */
export function avatarLetter(message: ChatMessage): string {
  return displayName(message).slice(0, 1).toUpperCase() || "?";
}

/** Left-border accent per source. */
export function sourceColor(source: ChatSource | string): string {
  switch (source) {
    case "agent":
      return "border-l-purple-500";
    case "system":
      return "border-l-blue-500";
    case "mattermost":
      return "border-l-indigo-500";
    case "slack":
      return "border-l-emerald-500";
    default:
      return "border-l-[var(--border-primary)]";
  }
}

/**
 * Border accent derived from incident-coordination `kind` (decision/action/
 * investigation_summary/agent_reply). Falls back to sourceColor.
 */
export function borderClass(message: IncidentCoordinationMessage): string {
  if (message.kind === "decision") return "border-l-emerald-500";
  if (message.kind === "action") return "border-l-amber-500";
  if (message.kind === "agent_reply") return "border-l-purple-500";
  if (message.kind === "investigation_summary") return "border-l-cyan-500";
  return sourceColor(message.source);
}

/** Absolute permalink to a message within the current page. */
export function messagePermalink(messageId: string): string {
  if (typeof window === "undefined") return `#msg-${messageId}`;
  return `${window.location.origin}${window.location.pathname}#msg-${messageId}`;
}

/** Avatar src for agent-sourced messages; undefined for human/system. The
 *  caller resolves the actual URL via `getAgentAvatarSrc(agentType)` —
 *  this file deliberately doesn't import `agentAvatar` so it stays
 *  cheap to load (and testable without the asset pipeline). */
export type AvatarSrcResolver = (agentType?: string) => string | undefined;

export function shouldShowAgentAvatar(
  message: ChatMessage & { source: ChatSource | string },
): boolean {
  return message.source === "agent";
}

/**
 * Group incident-coordination messages by `parent_message_id`. Roots
 * (messages without a `parent_message_id`) are keyed under `""`.
 */
export function groupMessagesByParent(
  messages: IncidentCoordinationMessage[],
): Map<string, IncidentCoordinationMessage[]> {
  const map = new Map<string, IncidentCoordinationMessage[]>();
  for (const m of messages) {
    const key = m.parent_message_id ?? "";
    const bucket = map.get(key);
    if (bucket) bucket.push(m);
    else map.set(key, [m]);
  }
  return map;
}

/** Root messages (no parent_message_id), sorted chronologically. */
export function rootMessages(
  messages: IncidentCoordinationMessage[],
  byParent: Map<string, IncidentCoordinationMessage[]>,
): IncidentCoordinationMessage[] {
  const roots = byParent.get("") ?? [];
  const knownIds = new Set(messages.map((m) => m.id));
  const orphans = messages.filter((m) => {
    const p = m.parent_message_id;
    return !!p && !knownIds.has(p);
  });
  return [...roots, ...orphans].sort(
    (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
  );
}

/** Children of a given parent, sorted chronologically. */
export function childrenOf(
  parentId: string,
  byParent: Map<string, IncidentCoordinationMessage[]>,
): IncidentCoordinationMessage[] {
  return (byParent.get(parentId) ?? [])
    .slice()
    .sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime());
}
