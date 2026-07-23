<script setup lang="ts">
import { computed } from "vue";
import HeaderActionsMenu from "@/components/ui/HeaderActionsMenu.vue";
import type { IncidentStatus } from "@/lib/api";
import { incidentListMenuItemsFor } from "@/lib/incidentActions";

const props = defineProps<{
  status: IncidentStatus;
  statusBusy: boolean;
  canCommand: boolean;
  canDelete: boolean;
}>();

const emit = defineEmits<{
  resolve: [];
  close: [];
  reopen: [];
  acknowledge: [];
  mitigate: [];
  cancel: [];
  delete: [];
}>();

const items = computed(() =>
  incidentListMenuItemsFor(
    props.status,
    {
      canCommand: props.canCommand,
      canDelete: props.canDelete,
      conferenceHref: null,
      loading: props.statusBusy,
      escalating: false,
    },
    {
      resolve: () => emit("resolve"),
      close: () => emit("close"),
      reopen: () => emit("reopen"),
      acknowledge: () => emit("acknowledge"),
      mitigate: () => emit("mitigate"),
      cancel: () => emit("cancel"),
      delete: () => emit("delete"),
    },
  ),
);
</script>

<template>
  <HeaderActionsMenu :items="items" label="Incident actions" />
</template>
