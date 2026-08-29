<script setup lang="ts">
import { computed, type Component } from "vue";
import { Trash2 } from "@lucide/vue";
import HeaderActionsMenu from "./HeaderActionsMenu.vue";

const props = withDefaults(
  defineProps<{
    workflowStatus: string;
    statusBusy: boolean;
    canWrite: boolean;
    canDelete: boolean;
    canCreateIncident: boolean;
    showAckButton: boolean;
    icon?: "vertical" | "horizontal";
  }>(),
  {
    icon: "vertical",
  },
);

const emit = defineEmits<{
  resolve: [];
  reopen: [];
  delete: [];
  createIncident: [];
}>();

type Item = {
  label: string;
  icon?: Component;
  onSelect: () => void;
  destructive?: boolean;
  disabled?: boolean;
};

const items = computed<Item[]>(() => {
  const out: Item[] = [];
  if (props.canWrite && !props.showAckButton) {
    if (props.workflowStatus === "open") {
      out.push({
        label: "Mark resolved",
        onSelect: () => emit("resolve"),
        disabled: props.statusBusy,
      });
    } else {
      out.push({
        label: "Re-open",
        onSelect: () => emit("reopen"),
        disabled: props.statusBusy,
      });
    }
  }
  if (props.canCreateIncident) {
    out.push({ label: "Create incident", onSelect: () => emit("createIncident") });
  }
  if (props.canDelete) {
    out.push({
      label: "Delete",
      icon: Trash2,
      onSelect: () => emit("delete"),
      destructive: true,
    });
  }
  return out;
});
</script>

<template>
  <HeaderActionsMenu :items="items" :icon="icon" label="Alert actions" />
</template>
