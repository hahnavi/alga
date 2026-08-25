<script setup lang="ts">
import { computed } from "vue";
import HeaderActionsMenu from "@/components/ui/HeaderActionsMenu.vue";
import type { IncidentStatus } from "@/lib/api";
import { incidentMenuItemsFor } from "@/lib/incidentActions";
defineOptions({ name: "IncidentActionsMenu" });

const props = defineProps<{
  status: IncidentStatus;
  loading: boolean;
  canCommand: boolean;
  canDelete: boolean;
  escalating: boolean;
  conferenceHref: string | null;
}>();

const emit = defineEmits<{
  edit: [];
  escalate: [];
  conference: [];
  acknowledge: [];
  mitigate: [];
  resolve: [];
  close: [];
  reopen: [];
  cancel: [];
  promote: [];
  delete: [];
}>();

const items = computed(() =>
  incidentMenuItemsFor(
    props.status,
    {
      canCommand: props.canCommand,
      canDelete: props.canDelete,
      conferenceHref: props.conferenceHref,
      loading: props.loading,
      escalating: props.escalating,
    },
    {
      edit: () => emit("edit"),
      escalate: () => emit("escalate"),
      acknowledge: () => emit("acknowledge"),
      mitigate: () => emit("mitigate"),
      resolve: () => emit("resolve"),
      close: () => emit("close"),
      reopen: () => emit("reopen"),
      cancel: () => emit("cancel"),
      promote: () => emit("promote"),
      delete: () => emit("delete"),
    },
  ),
);
</script>

<template>
  <HeaderActionsMenu :items="items" label="Incident actions" />
</template>
