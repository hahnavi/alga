<script setup lang="ts">
import { computed } from "vue";
import type { UserInfo } from "@/lib/api";

const props = defineProps<{
  user?: UserInfo | null;
  userId?: string;
  fallback?: string;
}>();

const display = computed(() => {
  const u = props.user;
  if (u) {
    const name = u.full_name?.trim();
    if (name) return name;
    if (u.email?.trim()) return u.email.trim();
  }
  if (props.userId) return props.userId.slice(0, 8);
  return props.fallback ?? "Unknown";
});
</script>

<template>
  <span>{{ display }}</span>
</template>
