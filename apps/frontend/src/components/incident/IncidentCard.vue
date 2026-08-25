<script setup lang="ts">
import { computed } from "vue";
import { Clock, Shield, Hash } from "@lucide/vue";
import type { IncidentRecord } from "@/lib/api";
import { incidentStatusBadgeClass, incidentStatusLabel } from "@/lib/alertLabels";
import { formatTimeAgo } from "@/lib/time";
import InteractiveCard from "@/components/ui/InteractiveCard.vue";
import IncidentListActionsMenu from "@/components/incident/IncidentListActionsMenu.vue";
defineOptions({ name: "IncidentCard" });

const props = withDefaults(
  defineProps<{
    incident: IncidentRecord;
    loading?: boolean;
    canCommand?: boolean;
    canDelete?: boolean;
    statusBusy?: boolean;
  }>(),
  {
    loading: false,
    canCommand: false,
    canDelete: false,
    statusBusy: false,
  },
);

defineEmits<{
  navigate: [];
  resolve: [];
  close: [];
  reopen: [];
  delete: [];
}>();

const currentIC = computed(() => {
  const icRole = props.incident.ics_roles?.find(
    (r) => r.role_type === "incident_commander" && r.status === "active",
  );
  return icRole?.user_name || icRole?.user_email || null;
});

function priorityRailClass(priority: string | null | undefined): string {
  switch (priority) {
    case "P1":
      return "bg-red-600 text-white";
    case "P2":
      return "bg-orange-500 text-white";
    case "P3":
      return "bg-amber-500 text-white";
    case "P4":
      return "bg-sky-500 text-white";
    case "P5":
      return "bg-gray-400 text-white";
    default:
      return "bg-[var(--bg-secondary)] text-transparent";
  }
}

function incidentSeverityRailClass(severity: string | null | undefined): string {
  switch (severity) {
    case "critical":
      return "bg-red-500 text-white";
    case "high":
      return "bg-orange-500 text-white";
    case "warning":
      return "bg-amber-500 text-white";
    case "info":
      return "bg-emerald-500 text-white";
    default:
      return "bg-[var(--bg-secondary)] text-transparent";
  }
}

function impactRailClass(impact: string | null | undefined): string {
  switch (impact) {
    case "high":
      return "bg-red-500 text-white";
    case "medium":
      return "bg-amber-500 text-white";
    case "low":
      return "bg-emerald-500 text-white";
    default:
      return "bg-[var(--bg-secondary)] text-transparent";
  }
}

function classificationLabel(value: string | null | undefined): string {
  if (!value) return "";
  return value.toUpperCase();
}

function railSegmentHeight(value: string | null | undefined): string {
  const label = classificationLabel(value);
  return `calc(${Math.max(label.length, 2)}ch + 1rem)`;
}
</script>

<template>
  <InteractiveCard
    :loading="loading"
    class="group/incident flex !p-0 !rounded"
    @navigate="$emit('navigate')"
  >
    <div
      class="flex w-6 shrink-0 flex-col overflow-hidden rounded-l border-r border-r-[var(--border-primary)] text-[0.55rem] font-semibold uppercase tracking-[0.14em]"
      aria-hidden="true"
    >
      <div
        :class="priorityRailClass(incident.priority)"
        :style="{ height: railSegmentHeight(incident.priority) }"
        class="flex shrink-0 items-center justify-center"
      >
        <span class="-rotate-90 whitespace-nowrap">{{ incident.priority }}</span>
      </div>
      <div
        :class="incidentSeverityRailClass(incident.severity)"
        :style="{ height: railSegmentHeight(incident.severity) }"
        class="flex shrink-0 items-center justify-center border-t border-white/35"
      >
        <span class="-rotate-90 whitespace-nowrap">{{
          classificationLabel(incident.severity)
        }}</span>
      </div>
      <div
        :class="impactRailClass(incident.impact_level)"
        :style="{ minHeight: railSegmentHeight(incident.impact_level) }"
        class="flex flex-1 items-center justify-center border-t border-white/35"
      >
        <span class="-rotate-90 whitespace-nowrap">{{
          classificationLabel(incident.impact_level)
        }}</span>
      </div>
    </div>
    <div class="min-w-0 flex-1 p-3 sm:p-4">
      <div class="flex items-start justify-between gap-3">
        <div class="min-w-0 flex-1">
          <span class="min-w-0 break-words font-medium text-[var(--text-primary)]">
            {{ incident.title }}
          </span>

          <div class="mt-1 flex items-center gap-2 text-xs text-[var(--text-muted)]">
            <span v-if="incident.incident_number" class="shrink-0 font-mono">
              #{{ incident.incident_number }}
            </span>
            <span class="flex items-center gap-1" :title="incident.created_at">
              <Clock class="h-3 w-3" />
              {{ formatTimeAgo(incident.created_at) }}
            </span>
            <span v-if="incident.incident_type">{{ incident.incident_type }}</span>
            <span v-if="currentIC" class="flex items-center gap-1">
              <Shield class="h-3 w-3" />
              {{ currentIC }}
            </span>
            <span v-if="incident.war_room_channel_id" class="flex items-center gap-1">
              <Hash class="h-3 w-3 text-[var(--text-success)]" />
              War Room
            </span>
          </div>

          <p
            v-if="incident.description"
            class="mt-1.5 line-clamp-2 text-sm text-[var(--text-secondary)]"
          >
            {{ incident.description }}
          </p>

          <div v-if="incident.tags?.length" class="mt-2 flex flex-wrap items-center gap-1.5">
            <span
              v-for="tag in incident.tags"
              :key="tag"
              class="rounded border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-1.5 py-0.5 text-xs"
            >
              {{ tag }}
            </span>
          </div>
        </div>
        <div class="flex shrink-0 flex-col items-end gap-2 self-stretch">
          <span :class="incidentStatusBadgeClass(incident.status)">
            {{ incidentStatusLabel(incident.status) }}
          </span>
          <div class="mt-auto flex items-center gap-1.5">
            <IncidentListActionsMenu
              v-if="canCommand || canDelete"
              :status="incident.status"
              :status-busy="statusBusy"
              :can-command="canCommand"
              :can-delete="canDelete"
              @resolve="$emit('resolve')"
              @close="$emit('close')"
              @reopen="$emit('reopen')"
              @delete="$emit('delete')"
            />
          </div>
        </div>
      </div>
    </div>
  </InteractiveCard>
</template>
