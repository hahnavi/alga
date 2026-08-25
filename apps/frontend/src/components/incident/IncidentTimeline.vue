<script setup lang="ts">
import { computed } from "vue";
import {
  Clock,
  Wrench,
  AlertTriangle,
  CheckCircle2,
  XCircle,
  ArrowRight,
  MessageSquare,
  Plus,
} from "@lucide/vue";
import type { IncidentTimelineRecord } from "@/lib/api";
import { formatTimeAgo, formatTimeFull } from "@/lib/time";
defineOptions({ name: "IncidentTimeline" });

const props = defineProps<{
  entries: IncidentTimelineRecord[];
}>();

type Tone = "success" | "info" | "warning" | "danger" | "neutral";

const displayEntries = computed(() =>
  props.entries
    .filter((entry) => entry.event_type !== "investigation_created")
    .map((entry) => ({
      ...entry,
      message:
        entry.event_type === "postmortem_created"
          ? entry.message.replace(
              /:\s*[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
              "",
            )
          : entry.message,
    }))
    .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime()),
);

const EVENT_META: Record<string, { icon: typeof Clock; tone: Tone }> = {
  status_changed: { icon: ArrowRight, tone: "info" },
  comment: { icon: MessageSquare, tone: "neutral" },
  note: { icon: MessageSquare, tone: "neutral" },
  manual: { icon: MessageSquare, tone: "neutral" },
  created: { icon: Plus, tone: "success" },
  acknowledged: { icon: CheckCircle2, tone: "info" },
  mitigated: { icon: Wrench, tone: "warning" },
  resolved: { icon: CheckCircle2, tone: "success" },
  closed: { icon: XCircle, tone: "neutral" },
  reopened: { icon: AlertTriangle, tone: "warning" },
  cancelled: { icon: XCircle, tone: "danger" },
  postmortem_created: { icon: MessageSquare, tone: "info" },
};

function eventMeta(eventType: string): { icon: typeof Clock; tone: Tone } {
  return EVENT_META[eventType] ?? { icon: Clock, tone: "neutral" };
}

function eventLabel(eventType: string): string {
  switch (eventType) {
    case "status_changed":
      return "Status";
    case "comment":
      return "Comment";
    case "created":
      return "Created";
    case "acknowledged":
      return "Acked";
    case "mitigated":
      return "Mitigated";
    case "resolved":
      return "Resolved";
    case "closed":
      return "Closed";
    case "reopened":
      return "Reopened";
    case "cancelled":
      return "Cancelled";
    case "note":
      return "Note";
    case "manual":
      return "Manual";
    case "postmortem_created":
      return "Post-mortem";
    default:
      return eventType.replace(/_/g, " ");
  }
}

function actorDisplay(entry: IncidentTimelineRecord): string {
  if (entry.metadata?.actor_display_name) return entry.metadata.actor_display_name as string;
  if (entry.actor_type === "system") return "System";
  if (entry.actor_type === "agent") return "Agent";
  if (entry.actor_type === "user") return "User";
  return "";
}
</script>

<template>
  <div class="space-y-1">
    <div
      v-if="displayEntries.length === 0"
      class="flex flex-col items-center justify-center gap-2 py-7 text-center"
    >
      <Clock class="h-6 w-6 text-[var(--text-muted)] opacity-40" aria-hidden="true" />
      <p class="text-sm text-[var(--text-muted)]">No timeline events yet.</p>
    </div>
    <ol v-else class="relative">
      <li
        v-for="(entry, idx) in displayEntries"
        :key="entry.id"
        class="relative flex gap-3 pb-5 last:pb-0"
      >
        <div
          v-if="idx < displayEntries.length - 1"
          class="absolute bottom-0 left-[13px] top-8 w-px bg-[var(--border-primary)]"
          aria-hidden="true"
        />

        <div
          class="tone-icon relative z-10 flex h-[26px] w-[26px] shrink-0 items-center justify-center rounded-full"
          :class="`tone-${eventMeta(entry.event_type).tone}`"
        >
          <component
            :is="eventMeta(entry.event_type).icon"
            class="h-3.5 w-3.5"
            aria-hidden="true"
          />
        </div>

        <div class="min-w-0 flex-1 pt-0.5">
          <div class="flex items-start justify-between gap-x-3 gap-y-1">
            <p class="text-sm leading-snug text-[var(--text-primary)]">
              {{ entry.message }}
            </p>
            <span
              class="tone-chip mt-0.5 shrink-0 rounded-full px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide"
              :class="`tone-chip-${eventMeta(entry.event_type).tone}`"
            >
              {{ eventLabel(entry.event_type) }}
            </span>
          </div>
          <div
            class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-[var(--text-muted)]"
          >
            <span v-if="actorDisplay(entry)" class="text-[var(--text-secondary)]">
              {{ actorDisplay(entry) }}
            </span>
            <span
              v-if="actorDisplay(entry)"
              aria-hidden="true"
              class="text-[var(--border-secondary)]"
              >·</span
            >
            <time
              :title="formatTimeAgo(entry.created_at)"
              :datetime="entry.created_at"
              class="tabular-nums"
            >
              {{ formatTimeFull(entry.created_at) }}
            </time>
          </div>
        </div>
      </li>
    </ol>
  </div>
</template>

<style scoped>
.tone-success {
  color: var(--text-success);
  background: color-mix(in srgb, var(--text-success) 16%, transparent);
}
.tone-info {
  color: var(--text-badge-info);
  background: color-mix(in srgb, var(--text-badge-info) 16%, transparent);
}
.tone-warning {
  color: var(--text-badge-warning);
  background: color-mix(in srgb, var(--text-badge-warning) 16%, transparent);
}
.tone-danger {
  color: var(--text-error);
  background: color-mix(in srgb, var(--text-error) 16%, transparent);
}
.tone-neutral {
  color: var(--text-secondary);
  background: var(--bg-secondary);
}

.tone-chip-success {
  color: var(--text-success);
  background: color-mix(in srgb, var(--text-success) 12%, transparent);
}
.tone-chip-info {
  color: var(--text-badge-info);
  background: color-mix(in srgb, var(--text-badge-info) 12%, transparent);
}
.tone-chip-warning {
  color: var(--text-badge-warning);
  background: color-mix(in srgb, var(--text-badge-warning) 12%, transparent);
}
.tone-chip-danger {
  color: var(--text-error);
  background: color-mix(in srgb, var(--text-error) 12%, transparent);
}
.tone-chip-neutral {
  color: var(--text-muted);
  background: var(--bg-secondary);
}
</style>
