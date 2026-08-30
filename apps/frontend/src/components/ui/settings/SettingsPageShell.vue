<script setup lang="ts">
defineOptions({ name: "SettingsPageShell" });

defineProps<{
  /** Muted intro line rendered above the page body. */
  description?: string;
}>();
</script>

<template>
  <section class="mx-auto max-w-4xl space-y-6 px-4 py-4 md:px-6 md:py-6">
    <p v-if="description" class="text-sm text-[var(--text-muted)]">{{ description }}</p>
    <slot />
  </section>
</template>
