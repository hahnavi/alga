<script setup lang="ts">
import type { ActionItemRecord } from "@/lib/api";
import { actionItemTypeBadgeClass } from "@/lib/alertLabels";
import Button from "@/components/ui/Button.vue";
import Select from "@/components/ui/Select.vue";
import { formatDate } from "@/lib/time";
defineOptions({ name: "ActionItemRow" });

const props = defineProps<{
  item: ActionItemRecord;
}>();

const emit = defineEmits<{
  update: [data: Partial<ActionItemRecord>];
  delete: [];
}>();

const statusOptions = ["open", "in_progress", "completed", "cancelled"] as const;

function priorityClass(p: string): string {
  switch (p) {
    case "high":
      return "badge badge-red";
    case "medium":
      return "badge badge-yellow";
    case "low":
      return "badge badge-green";
    default:
      return "badge badge-muted";
  }
}

function statusLabel(s: string): string {
  return s.replace("_", " ");
}
</script>

<template>
  <tr class="border-b border-[var(--border-primary)] last:border-0">
    <td class="px-3 py-2 text-sm text-[var(--text-primary)]">{{ item.description }}</td>
    <td class="px-3 py-2">
      <span :class="actionItemTypeBadgeClass(item.type)" class="text-xs">{{ item.type }}</span>
    </td>
    <td class="px-3 py-2">
      <span :class="priorityClass(item.priority)" class="text-xs">{{ item.priority }}</span>
    </td>
    <td class="px-3 py-2">
      <Select
        :model-value="item.status"
        class="text-xs"
        @update:model-value="emit('update', { status: $event as ActionItemRecord['status'] })"
      >
        <option v-for="s in statusOptions" :key="s" :value="s">{{ statusLabel(s) }}</option>
      </Select>
    </td>
    <td class="px-3 py-2 text-xs text-[var(--text-muted)]">
      {{ item.due_date ? formatDate(item.due_date) : "—" }}
    </td>
    <td class="px-3 py-2 text-xs text-[var(--text-muted)]">
      {{ item.assignee_name || (item.assignee_id ? "Assigned" : "—") }}
    </td>
    <td class="px-3 py-2 text-right">
      <Button size="sm" variant="destructive" @click="emit('delete')">Delete</Button>
    </td>
  </tr>
</template>
