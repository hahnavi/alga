<script setup lang="ts">
import { computed } from "vue";
import {
  CheckCircle,
  ClipboardList,
  MessageSquare,
  Plus,
  Search,
  Wrench,
  XCircle,
} from "@lucide/vue";
import type { CoordinationTask } from "@/lib/api";
import Card from "@/components/ui/Card.vue";
import Button from "@/components/ui/Button.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
defineOptions({ name: "CoordinationTaskBoard" });

const props = withDefaults(
  defineProps<{
    tasks: CoordinationTask[];
    canCommand?: boolean;
  }>(),
  { canCommand: false },
);

const emit = defineEmits<{
  dispatch: [];
  cancel: [task: CoordinationTask];
}>();

const KIND_ICON = {
  investigate: Search,
  communicate: MessageSquare,
  verify: CheckCircle,
  mitigate: Wrench,
  synthesize: ClipboardList,
} as const;

const STATUS_STYLE: Record<CoordinationTask["status"], string> = {
  pending: "bg-slate-500/15 text-slate-600 dark:text-slate-300",
  assigned: "bg-amber-500/15 text-amber-600 dark:text-amber-400",
  in_progress: "bg-amber-500/15 text-amber-600 dark:text-amber-400",
  complete: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400",
  failed: "bg-red-500/15 text-red-600 dark:text-red-400",
  cancelled: "bg-gray-500/15 text-gray-500 dark:text-gray-400",
};

const CANCELLABLE = new Set<CoordinationTask["status"]>(["pending", "assigned", "in_progress"]);

const sortedTasks = computed<CoordinationTask[]>(() => {
  return [...props.tasks].sort((a, b) => {
    const pa = a.priority ?? 0;
    const pb = b.priority ?? 0;
    if (pa !== pb) return pb - pa;
    return new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
  });
});

function kindIcon(task: CoordinationTask) {
  return KIND_ICON[task.kind] ?? ClipboardList;
}

function statusLabel(status: CoordinationTask["status"]): string {
  return status.replace("_", " ");
}

function assigneeLabel(task: CoordinationTask): string {
  const role = task.assignee_role;
  const name = task.assignee_agent_name;
  return name ? `${role} · ${name}` : role;
}

function resultSummary(task: CoordinationTask): string | null {
  if (task.status !== "complete" || !task.result) return null;
  const r = task.result as Record<string, unknown>;
  const candidate =
    typeof r.finding === "string"
      ? (r.finding as string)
      : typeof r.action_taken === "string"
        ? (r.action_taken as string)
        : typeof r.summary === "string"
          ? (r.summary as string)
          : null;
  if (!candidate) return null;
  return candidate.length > 160 ? `${candidate.slice(0, 159)}…` : candidate;
}

function dueCountdown(task: CoordinationTask): string | null {
  if (!task.due_at) return null;
  if (!CANCELLABLE.has(task.status)) return null;
  const ms = new Date(task.due_at).getTime() - Date.now();
  if (!Number.isFinite(ms)) return null;
  const absMin = Math.abs(Math.round(ms / 60000));
  const sign = ms < 0 ? "-" : "";
  if (absMin < 60) return `${sign}${absMin}m`;
  const h = Math.floor(absMin / 60);
  const m = absMin % 60;
  if (h < 24) return `${sign}${h}h ${m}m`;
  const d = Math.floor(h / 24);
  return `${sign}${d}d ${h % 24}h`;
}
</script>

<template>
  <Card class="hover:shadow-md transition-all duration-300">
    <div
      class="mb-3 flex items-center justify-between gap-2 border-b border-[var(--border-primary)] pb-2"
    >
      <div class="flex items-center gap-2">
        <ClipboardList class="h-4 w-4 text-[var(--text-secondary)]" />
        <h3 class="text-sm font-semibold text-[var(--text-primary)]">Coordination Tasks</h3>
        <span
          v-if="sortedTasks.length"
          class="rounded-full bg-[var(--bg-secondary)] px-1.5 py-0.5 text-xs text-[var(--text-muted)]"
        >
          {{ sortedTasks.length }}
        </span>
      </div>
      <Button v-if="canCommand" size="sm" @click="emit('dispatch')">
        <Plus class="h-3.5 w-3.5" />
        Dispatch task
      </Button>
    </div>

    <EmptyState v-if="sortedTasks.length === 0" message="No coordination tasks yet.">
      <template #icon>
        <ClipboardList class="h-8 w-8" />
      </template>
    </EmptyState>

    <ul v-else class="flex flex-col gap-2">
      <li
        v-for="task in sortedTasks"
        :key="task.id"
        class="rounded-md border border-[var(--border-primary)] bg-[var(--bg-card)] p-2.5"
      >
        <div class="flex items-start gap-2">
          <component
            :is="kindIcon(task)"
            class="mt-0.5 h-4 w-4 shrink-0 text-[var(--text-secondary)]"
          />
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-1.5">
              <span
                class="rounded px-1.5 py-0.5 text-xs font-medium capitalize"
                :class="STATUS_STYLE[task.status]"
              >
                {{ statusLabel(task.status) }}
              </span>
              <span class="text-xs text-[var(--text-muted)] capitalize">
                {{ task.kind }}
              </span>
              <span class="text-xs text-[var(--text-muted)]">·</span>
              <span class="text-xs text-[var(--text-secondary)]">{{ assigneeLabel(task) }}</span>
              <span
                v-if="dueCountdown(task)"
                class="text-xs font-medium"
                :class="dueCountdown(task)?.startsWith('-') ? 'text-red-500' : 'text-amber-500'"
                :title="`Due ${task.due_at}`"
              >
                ⏱ {{ dueCountdown(task) }}
              </span>
            </div>

            <p class="mt-1 text-sm text-[var(--text-primary)]">{{ task.goal }}</p>

            <p v-if="resultSummary(task)" class="mt-1 text-xs text-[var(--text-secondary)]">
              {{ resultSummary(task) }}
            </p>

            <p
              v-if="task.status === 'failed' && task.failure_reason"
              class="mt-1 text-xs text-red-500"
            >
              {{ task.failure_reason }}
            </p>

            <div class="mt-1.5 flex flex-wrap items-center gap-2">
              <router-link
                v-if="task.linked_investigation_id"
                :to="`/incidents/${task.incident_id ?? ''}/investigations/${task.linked_investigation_id}`"
                class="text-xs text-[var(--accent)] hover:underline"
              >
                View investigation
              </router-link>
              <span
                v-if="task.created_by_name"
                class="text-xs text-[var(--text-muted)]"
                :title="`Created by ${task.created_by_name}`"
              >
                by {{ task.created_by_name }}
              </span>
              <button
                v-if="canCommand && CANCELLABLE.has(task.status)"
                type="button"
                class="ml-auto inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-xs text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-red-500"
                @click="emit('cancel', task)"
              >
                <XCircle class="h-3.5 w-3.5" />
                Cancel
              </button>
            </div>
          </div>
        </div>
      </li>
    </ul>
  </Card>
</template>
