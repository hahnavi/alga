<script setup lang="ts">
import { computed, type Component } from "vue";
import { Bot, Hash, Loader2, Shield, X } from "@lucide/vue";
import HeaderActionsMenu from "./HeaderActionsMenu.vue";
import type { AgentTokenRow } from "@/lib/api";

const props = defineProps<{
  agent: AgentTokenRow;
  toggling: boolean;
}>();

const emit = defineEmits<{
  toggleEnabled: [];
  editScope: [];
  regenerate: [];
  delete: [];
}>();

const toggleLabel = computed(() => {
  if (props.toggling) return props.agent.enabled ? "Disabling…" : "Enabling…";
  return props.agent.enabled ? "Disable" : "Enable";
});

const toggleIcon = computed<Component>(() => (props.toggling ? Loader2 : Shield));

type Item = {
  label: string;
  icon: Component;
  onSelect: () => void;
  disabled?: boolean;
};

const items = computed<Item[]>(() => [
  {
    label: toggleLabel.value,
    icon: toggleIcon.value,
    onSelect: () => emit("toggleEnabled"),
    disabled: props.agent.expired || props.toggling,
  },
  {
    label: "Edit scope",
    icon: Hash,
    onSelect: () => emit("editScope"),
    disabled: props.agent.expired,
  },
  {
    label: "Regenerate token",
    icon: Bot,
    onSelect: () => emit("regenerate"),
    disabled: props.agent.expired,
  },
  {
    label: "Delete",
    icon: X,
    onSelect: () => emit("delete"),
    destructive: true,
  },
]);
</script>

<template>
  <HeaderActionsMenu :items="items" label="Agent actions" />
</template>
