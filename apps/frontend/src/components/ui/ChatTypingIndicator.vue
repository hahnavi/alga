<script setup lang="ts">
import Avatar from "./Avatar.vue";
import TypingIndicator from "./TypingIndicator.vue";

defineProps<{
  avatarSrc?: string;
  avatarBg: string;
  avatarTitle: string;
  displayName: string;
  dimmed?: boolean;
}>();
</script>

<template>
  <div class="flex items-center gap-3 py-2.5 pl-4" :class="{ 'opacity-40': dimmed }">
    <Avatar :src="avatarSrc" :bg-class="avatarBg" :title="avatarTitle" />
    <div class="flex items-center gap-1.5">
      <span class="text-sm text-[var(--text-muted)]">{{ displayName }} is typing</span>
      <TypingIndicator />
    </div>
  </div>
</template>
