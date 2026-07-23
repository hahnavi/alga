<script setup lang="ts">
import { ref } from "vue";
import { ExternalLink, Repeat2, User, UserPlus } from "@lucide/vue";
import type { UserInfo } from "@/lib/api";
import Card from "@/components/ui/Card.vue";
import IconBtn from "@/components/ui/IconBtn.vue";
import UserPickerDialog from "@/components/ui/UserPickerDialog.vue";
import { formatTimeFull } from "@/lib/time";
import { getAgentAvatarSrc } from "@/lib/agentAvatar";

type TimelineEntry = {
  type: string;
  timestamp: string;
  label: string;
  subline: string;
  dotClass: string;
  lineClass: string;
  iconClass: string;
  icon: unknown;
};

type ResolvedDeliveryTarget = {
  channel: string;
  channelLabel: string;
  icon: string;
  href: string | null;
};

type Assignee = {
  name: string;
  isAgent: boolean;
  agentType?: string;
};

defineProps<{
  runbookHref: string | null;
  deliveryTargets: ResolvedDeliveryTarget[];
  timeline: TimelineEntry[];
  assignee: Assignee | null;
  users?: UserInfo[];
  canAssign?: boolean;
  assigneeId?: string;
}>();

const emit = defineEmits<{
  openDeliveryThread: [target: ResolvedDeliveryTarget];
  assign: [assigneeType: "user" | "agent", assigneeId?: string];
}>();

const showPicker = ref(false);

function assigneeInitial(name: string): string {
  return name.trim().charAt(0).toUpperCase() || "?";
}

function pickUser(userId: string) {
  showPicker.value = false;
  emit("assign", "user", userId);
}

function pickAgent() {
  showPicker.value = false;
  emit("assign", "agent");
}
</script>

<template>
  <Card v-if="runbookHref">
    <h3 class="field-label mb-3">Runbook</h3>
    <a
      :href="runbookHref"
      :title="runbookHref"
      target="_blank"
      rel="noopener noreferrer"
      class="inline-flex items-center gap-1.5 text-sm font-medium text-[var(--text-primary)] underline decoration-[var(--border-secondary)] underline-offset-2 transition-colors hover:text-[var(--text-secondary)] hover:decoration-[var(--text-muted)]"
    >
      Open runbook
      <ExternalLink class="h-4 w-4 shrink-0 text-[var(--text-muted)]" aria-hidden="true" />
    </a>
  </Card>

  <Card>
    <div class="mb-3 flex items-center justify-between gap-2">
      <h3 class="field-label mb-0">Assignee</h3>
      <IconBtn
        v-if="assignee && canAssign"
        :icon="Repeat2"
        label="Reassign investigation"
        size="sm"
        @click="showPicker = true"
      />
    </div>
    <div v-if="assignee" class="flex items-center gap-3">
      <div
        v-if="assignee.isAgent"
        class="flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-full bg-[var(--bg-tertiary)]"
      >
        <img
          :src="getAgentAvatarSrc(assignee.agentType)"
          :alt="assignee.name"
          class="h-full w-full object-cover"
          loading="lazy"
          decoding="async"
        />
      </div>
      <div
        v-else
        class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-[var(--bg-tertiary)] text-sm font-semibold text-[var(--text-secondary)]"
        :title="assignee.name"
      >
        {{ assigneeInitial(assignee.name) }}
      </div>
      <div class="min-w-0 flex-1">
        <div class="truncate text-sm font-medium text-[var(--text-primary)]">
          {{ assignee.name }}
        </div>
        <div class="text-xs text-[var(--text-muted)]">
          {{ assignee.isAgent ? "Agent" : "User" }}
        </div>
      </div>
      <User
        v-if="!assignee.isAgent"
        class="h-4 w-4 shrink-0 text-[var(--text-muted)]"
        aria-hidden="true"
      />
    </div>
    <div v-else class="flex items-center gap-3">
      <div
        class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full border border-dashed border-[var(--border-primary)] text-[var(--text-muted)]"
      >
        <User class="h-4 w-4" aria-hidden="true" />
      </div>
      <div class="min-w-0 flex-1 text-sm text-[var(--text-muted)]">Unassigned</div>
      <button
        v-if="canAssign"
        type="button"
        class="inline-flex h-7 shrink-0 cursor-pointer items-center gap-1 rounded-md bg-[var(--accent-primary)] px-2.5 text-xs font-medium text-white transition-opacity hover:opacity-90 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
        @click="showPicker = true"
      >
        <UserPlus class="h-3 w-3" aria-hidden="true" />
        Assign
      </button>
    </div>

    <UserPickerDialog
      :open="showPicker"
      :users="users ?? []"
      :selected-user-id="assignee?.isAgent ? undefined : assigneeId"
      :show-agent-option="true"
      :agent-selected="Boolean(assignee?.isAgent)"
      title="Assign investigation"
      @close="showPicker = false"
      @pick-user="pickUser"
      @pick-agent="pickAgent"
    />
  </Card>

  <Card>
    <h3 class="field-label mb-3">Notifications</h3>
    <div v-if="deliveryTargets.length === 0" class="text-sm text-[var(--text-muted)]">
      No routed threads.
    </div>
    <div v-else class="flex flex-col gap-2">
      <button
        v-for="(dt, i) in deliveryTargets"
        :key="i"
        type="button"
        class="flex w-full cursor-pointer items-center gap-2 rounded-md border border-[var(--border-primary)] bg-[var(--bg-secondary)] px-3 py-2 text-left text-sm text-[var(--text-primary)] transition-colors hover:bg-[var(--btn-default-hover)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)] disabled:cursor-not-allowed disabled:opacity-50"
        :disabled="!dt.href"
        :title="dt.href ? 'Open thread' : 'Thread link unavailable'"
        @click="$emit('openDeliveryThread', dt)"
      >
        <img
          :src="dt.icon"
          alt=""
          class="h-5 w-5 shrink-0 rounded-sm"
          loading="lazy"
          decoding="async"
        />
        <span class="min-w-0 flex-1 truncate font-medium">{{ dt.channelLabel }}</span>
        <ExternalLink class="ml-auto h-3.5 w-3.5 shrink-0 text-[var(--text-muted)]" />
      </button>
    </div>
  </Card>

  <slot name="after-notifications" />

  <Card>
    <h3 class="field-label mb-4">Timeline</h3>
    <div v-if="timeline.length === 0" class="text-sm text-[var(--text-muted)]">
      No events recorded.
    </div>
    <div v-else class="relative">
      <div v-for="(entry, idx) in timeline" :key="idx" class="relative flex gap-4 pb-6 last:pb-0">
        <div
          v-if="idx < timeline.length - 1"
          class="absolute left-[9px] top-5 h-full w-px border-l-2 border-dashed border-[var(--border-primary)]"
          :class="entry.lineClass"
        />

        <div
          class="relative z-10 mt-1 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-[var(--bg-secondary)]"
        >
          <component :is="entry.icon" class="h-3 w-3" :class="entry.iconClass" />
        </div>

        <div class="min-w-0">
          <div class="font-medium">{{ entry.label }}</div>
          <div v-if="entry.subline" class="text-sm text-[var(--text-secondary)]">
            {{ entry.subline }}
          </div>
          <div class="text-xs text-[var(--text-muted)]">
            {{ formatTimeFull(entry.timestamp) }}
          </div>
        </div>
      </div>
    </div>
  </Card>
</template>
