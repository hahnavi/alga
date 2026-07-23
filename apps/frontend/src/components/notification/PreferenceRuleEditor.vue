<script setup lang="ts">
import type { NotificationPreferenceRule } from "@/lib/api";
import Switch from "@/components/ui/Switch.vue";
import Select from "@/components/ui/Select.vue";
import Checkbox from "@/components/ui/Checkbox.vue";

const props = defineProps<{
  rule: NotificationPreferenceRule;
  index: number;
}>();

const emit = defineEmits<{
  update: [data: Partial<NotificationPreferenceRule>];
  remove: [];
}>();

const NOTIFICATION_TYPES = [
  "incident_assigned_commander",
  "incident_assigned_responder",
  "incident_escalation",
  "incident_sla_breach_response",
  "incident_sla_breach_resolve",
  "incident_status_change",
  "incident_severity_change",
  "on_call_shift_starting",
  "post_mortem_review_requested",
  "post_mortem_action_item",
  "service_status_change",
] as const;

const CHANNELS = ["in_app", "email", "mattermost", "slack", "voice"] as const;

function toggleChannel(channel: string) {
  const channels = [...(props.rule.channels ?? [])];
  const idx = channels.indexOf(channel);
  if (idx === -1) {
    channels.push(channel);
  } else {
    channels.splice(idx, 1);
  }
  emit("update", { channels });
}
</script>

<template>
  <tr class="border-b border-[var(--border-primary)] last:border-0">
    <td class="px-3 py-2">
      <Select
        :model-value="rule.notification_type"
        class="text-xs"
        @update:model-value="emit('update', { notification_type: $event })"
      >
        <option v-for="t in NOTIFICATION_TYPES" :key="t" :value="t">
          {{ t.replace(/_/g, " ") }}
        </option>
      </Select>
    </td>
    <td class="px-3 py-2">
      <div class="flex flex-wrap gap-2">
        <label
          v-for="ch in CHANNELS"
          :key="ch"
          class="inline-flex cursor-pointer items-center gap-1 text-xs text-[var(--text-secondary)]"
        >
          <Checkbox
            :model-value="rule.channels?.includes(ch)"
            @update:model-value="toggleChannel(ch)"
          />
          {{ ch.replace(/_/g, " ") }}
        </label>
      </div>
    </td>
    <td class="px-3 py-2">
      <Switch
        :model-value="rule.enabled"
        @update:model-value="emit('update', { enabled: $event })"
      />
    </td>
    <td class="px-3 py-2 text-right">
      <button
        class="rounded p-1 text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)]"
        title="Remove rule"
        @click="emit('remove')"
      >
        &times;
      </button>
    </td>
  </tr>
</template>
