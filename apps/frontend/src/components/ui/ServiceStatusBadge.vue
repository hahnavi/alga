<script setup lang="ts">
import { serviceStatusBadgeClass, serviceStatusLabel } from "@/lib/alertLabels";
defineProps<{ status: string }>();
</script>
<template>
  <span :class="[serviceStatusBadgeClass(status), 'badge']">{{ serviceStatusLabel(status) }}</span>
</template>
