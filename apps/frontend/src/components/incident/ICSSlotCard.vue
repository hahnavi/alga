<script setup lang="ts">
import { computed } from "vue";
import { Headset, Repeat2, Shield, User, UserPlus, X } from "@lucide/vue";
import type { ICSRoleRecord, ICSRoleType } from "@/lib/api";
import { formatTimeAgo } from "@/lib/time";
import { getAgentAvatarSrc } from "@/lib/agentAvatar";
import Avatar from "@/components/ui/Avatar.vue";
import IconBtn from "@/components/ui/IconBtn.vue";
defineOptions({ name: "ICSSlotCard" });

const props = defineProps<{
  roleType: ICSRoleType;
  role: ICSRoleRecord | null;
  canManage: boolean;
  responsibilityHint?: string;
}>();

const emit = defineEmits<{
  assign: [];
  replace: [];
  end: [roleId: string];
}>();

const config = computed(() => {
  switch (props.roleType) {
    case "incident_commander":
      return {
        label: "Incident Commander",
        short: "Commander",
        accent: "ic",
        icon: Shield,
      };
    case "communications_lead":
      return {
        label: "Communicator",
        short: "Comms",
        accent: "comms",
        icon: Headset,
      };
    default:
      return {
        label: props.roleType,
        short: props.roleType,
        accent: "default",
        icon: User,
      };
  }
});

const assigneeName = computed(() => {
  const r = props.role;
  if (!r) return "";
  if (r.assignee_type === "agent") return r.agent_name || "Agent";
  return r.user_name || r.user_email || r.user_id || "Unknown";
});

const isAgent = computed(() => props.role?.assignee_type === "agent");
const isAgentRevoked = computed(() => isAgent.value && !!props.role?.agent_revoked);
const accentClass = computed(() => {
  switch (config.value.accent) {
    case "ic":
      return {
        ring: "ring-1 ring-inset ring-[var(--accent-primary)]/30",
        avatar: "bg-[var(--accent-primary)]/12 !text-[var(--accent-primary)]",
        label: "text-[var(--accent-primary)]",
      };
    case "comms":
      return {
        ring: "ring-1 ring-inset ring-[var(--text-badge-orange)]/40",
        avatar: "bg-[var(--text-badge-orange)]/15 !text-[var(--text-badge-orange)]",
        label: "text-[var(--text-badge-orange)]",
      };
    default:
      return {
        ring: "ring-1 ring-inset ring-[var(--border-primary)]",
        avatar: "bg-[var(--bg-secondary)] !text-[var(--text-muted)]",
        label: "text-[var(--text-muted)]",
      };
  }
});

const RoleIcon = computed(() => config.value.icon);
</script>

<template>
  <div
    :class="[
      'rounded-lg border border-[var(--border-primary)] bg-[var(--bg-primary)] p-3',
      role ? accentClass.ring : '',
    ]"
  >
    <div class="flex items-center gap-2">
      <span class="text-[10px] font-semibold uppercase tracking-[0.12em] text-[var(--text-muted)]">
        {{ config.short }}
      </span>
      <span class="h-px flex-1 bg-[var(--border-primary)]" />
      <component
        :is="RoleIcon"
        :class="['h-3.5 w-3.5', role ? accentClass.label : 'text-[var(--text-muted)]']"
      />
    </div>

    <div v-if="role" class="mt-2.5 flex items-start gap-3">
      <Avatar
        :src="isAgent ? getAgentAvatarSrc(role.agent_type) : undefined"
        :letter="!isAgent ? assigneeName.charAt(0).toUpperCase() || '?' : undefined"
        :bg-class="isAgent ? 'bg-[var(--bg-tertiary,rgb(148_163_184/0.1))]' : accentClass.avatar"
        :title="assigneeName"
        :grayed="isAgentRevoked"
        class="!h-9 !w-9 !text-sm"
      />
      <div class="min-w-0 flex-1">
        <p
          class="truncate text-sm font-semibold text-[var(--text-primary)]"
          :class="{ italic: isAgentRevoked }"
        >
          {{ assigneeName }}
        </p>
        <p v-if="isAgentRevoked" class="mt-0.5 truncate text-xs text-[var(--text-muted)]">
          Agent deleted
        </p>
        <p
          v-else-if="role.scope_description"
          class="mt-0.5 truncate text-xs text-[var(--text-secondary)]"
          :title="role.scope_description"
        >
          {{ role.scope_description }}
        </p>
        <p class="mt-0.5 text-[11px] text-[var(--text-muted)]">
          since {{ formatTimeAgo(role.started_at) }}
        </p>
      </div>
      <div v-if="canManage" class="flex shrink-0 items-center gap-1">
        <IconBtn
          :icon="Repeat2"
          label="Replace the current assignee"
          size="sm"
          @click="emit('replace')"
        />
        <button
          type="button"
          class="inline-flex h-7 w-7 items-center justify-center rounded-md text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-error)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
          :title="`End ${config.label}`"
          :aria-label="`End ${config.label}`"
          @click="emit('end', role.id)"
        >
          <X class="h-3.5 w-3.5" />
        </button>
      </div>
    </div>

    <div v-else class="mt-2.5 flex items-center gap-3">
      <div
        class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full border border-dashed border-[var(--border-primary)] text-[var(--text-muted)]"
      >
        <component :is="RoleIcon" class="h-4 w-4" />
      </div>
      <div class="min-w-0 flex-1">
        <p class="text-sm font-medium text-[var(--text-muted)]">No {{ config.label }} assigned</p>
        <p v-if="responsibilityHint" class="mt-0.5 text-xs text-[var(--text-muted)]">
          {{ responsibilityHint }}
        </p>
      </div>
      <button
        v-if="canManage"
        type="button"
        class="inline-flex h-7 shrink-0 items-center gap-1 rounded-md bg-[var(--accent-primary)] px-2.5 text-xs font-medium text-white transition-opacity hover:opacity-90 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
        @click="emit('assign')"
      >
        <UserPlus class="h-3 w-3" />
        Assign
      </button>
    </div>
  </div>
</template>
