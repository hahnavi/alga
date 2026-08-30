<script setup lang="ts">
import type { ActionItemRecord } from "@/lib/api";
import { actionItemTypeBadgeClass } from "@/lib/alertLabels";
import Button from "@/components/ui/Button.vue";
import Select from "@/components/ui/Select.vue";
import DatePicker from "@/components/ui/DatePicker.vue";
import Input from "@/components/ui/Input.vue";
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

// The API accepts date-only values (normalized to end-of-day UTC) and full
// RFC3339 timestamps; DatePicker emits date-only, so pass it through. An
// empty string clears the due date (the backend distinguishes absent vs
// empty), so it must be sent explicitly rather than dropped as undefined.
function onDueDateChange(value: string) {
  emit("update", { due_date: value });
}

function onAssigneeNameChange(value: string) {
  emit("update", { assignee_name: value });
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
    <td class="px-3 py-2">
      <DatePicker
        :model-value="item.due_date ? item.due_date.slice(0, 10) : ''"
        placeholder="Due date"
        @update:model-value="onDueDateChange"
      />
    </td>
    <td class="px-3 py-2">
      <Input
        :model-value="item.assignee_name ?? ''"
        placeholder="Assignee"
        class="w-32 text-xs"
        @change="onAssigneeNameChange(($event.target as HTMLInputElement).value)"
      />
      <span v-if="item.assignee_id && !item.assignee_name" class="text-xs text-[var(--text-muted)]">
        Assigned
      </span>
    </td>
    <td class="px-3 py-2 text-right">
      <Button size="sm" variant="destructive" @click="emit('delete')">Delete</Button>
    </td>
  </tr>
</template>
