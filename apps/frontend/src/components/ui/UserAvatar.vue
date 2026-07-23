<script setup lang="ts">
import type { UserInfo } from "@/lib/api";
import Avatar from "./Avatar.vue";

withDefaults(
  defineProps<{
    user?: UserInfo | null;
    grayed?: boolean;
  }>(),
  { grayed: false },
);

function initials(user: UserInfo | null | undefined): string {
  if (!user) return "?";
  const fullName = user.full_name?.trim();
  if (fullName) {
    const parts = fullName.split(/\s+/).filter(Boolean);
    if (parts.length >= 2 && parts[0] && parts[1]) {
      return `${parts[0][0]}${parts[1][0]}`.toUpperCase();
    }
    return fullName.slice(0, 2).toUpperCase();
  }
  return (user.email ?? "").slice(0, 2).toUpperCase();
}
</script>

<template>
  <Avatar :letter="initials(user)" :grayed="grayed" :title="user?.full_name || user?.email" />
</template>
