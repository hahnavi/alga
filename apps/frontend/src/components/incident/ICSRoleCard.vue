<script setup lang="ts">
import { computed } from "vue";
import { X } from "@lucide/vue";
import type { ICSRoleRecord } from "@/lib/api";
import { formatTimeAgo } from "@/lib/time";
import { useEntityPermissions } from "@/composables/useEntityPermissions";
import { getAgentAvatarSrc } from "@/lib/agentAvatar";
import Avatar from "@/components/ui/Avatar.vue";
import DeletedBadge from "@/components/ui/DeletedBadge.vue";
import IconBtn from "@/components/ui/IconBtn.vue";

const props = defineProps<{
  role: ICSRoleRecord;
}>();

defineEmits<{
  endRole: [roleId: string];
}>();

const { canCommand } = useEntityPermissions("incidents");

const assigneeName = computed(() => {
  if (props.role.assignee_type === "agent") {
    return props.role.agent_name || "Agent";
  }
  return props.role.user_name || props.role.user_email || props.role.user_id || "Unknown";
});

const roleLabel = computed(() => {
  switch (props.role.role_type) {
    case "incident_commander":
      return "Incident Commander";
    case "communications_lead":
      return "Communicator";
    case "responder":
      return "Responder";
    default:
      return props.role.role_type;
  }
});

const isAgent = computed(() => props.role.assignee_type === "agent");
const isAgentRevoked = computed(() => isAgent.value && !!props.role.agent_revoked);
</script>

<template>
  <div
    class="flex items-start gap-3 rounded-md border border-[var(--border-primary)] p-3 transition-colors hover:bg-[var(--bg-secondary)]/40"
    :class="{ 'ring-1 ring-[var(--focus-ring)]': role.role_type === 'incident_commander' }"
  >
    <Avatar
      :src="isAgent ? getAgentAvatarSrc(role.agent_type) : undefined"
      :letter="!isAgent ? assigneeName.charAt(0).toUpperCase() || '?' : undefined"
      :bg-class="
        isAgent ? 'bg-[var(--bg-tertiary)]' : 'bg-[var(--bg-secondary)] !text-[var(--text-muted)]'
      "
      :title="assigneeName"
      :grayed="isAgentRevoked"
      class="!h-8 !w-8 !text-xs"
    />
    <div class="min-w-0 flex-1">
      <div class="flex flex-wrap items-center gap-2">
        <span
          class="truncate text-sm font-medium text-[var(--text-primary)]"
          :class="{ italic: isAgentRevoked }"
        >
          {{ assigneeName }}
        </span>
        <span
          v-if="role.role_type !== 'responder'"
          class="rounded bg-[var(--bg-secondary)] px-1.5 py-0.5 text-[11px] font-medium text-[var(--text-muted)]"
        >
          {{ roleLabel }}
        </span>
        <DeletedBadge v-if="isAgentRevoked" title="This agent was deleted" />
      </div>
      <div class="mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-[11px] text-[var(--text-muted)]">
        <span>since {{ formatTimeAgo(role.started_at) }}</span>
        <span v-if="role.scope_description">Scope: {{ role.scope_description }}</span>
        <span v-if="role.ended_at">ended {{ formatTimeAgo(role.ended_at) }}</span>
        <span v-if="role.ended_reason" class="text-[var(--text-error)]"
          >({{ role.ended_reason }})</span
        >
      </div>
    </div>
    <IconBtn
      v-if="canCommand && role.status === 'active'"
      :icon="X"
      label="End role"
      size="sm"
      @click="$emit('endRole', role.id)"
    />
  </div>
</template>
