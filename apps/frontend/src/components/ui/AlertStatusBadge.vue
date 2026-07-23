<script setup lang="ts">
import { alertCombinedStatus } from "@/lib/alertLabels";
defineProps<{ status: string; acknowledged?: boolean }>();
</script>
<template>
  <span :class="[alertCombinedStatus({ status, acknowledged }).badge, 'badge']">
    {{ alertCombinedStatus({ status, acknowledged }).label }}
  </span>
</template>
