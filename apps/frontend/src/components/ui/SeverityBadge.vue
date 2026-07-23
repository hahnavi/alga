<script setup lang="ts">
import { severityBadgeClass } from "@/lib/alertLabels";
defineProps<{ severity: string }>();
</script>
<template>
  <span :class="[severityBadgeClass(severity), 'badge']">{{ severity }}</span>
</template>
