<script setup lang="ts">
withDefaults(
  defineProps<{
    required?: boolean;
    for?: string;
    /** Render the label muted (used when the field below is disabled). */
    disabled?: boolean;
    /** Quiet secondary hint shown after the label content (e.g. "(optional)"). */
    hint?: string;
    /** Compact uppercase style for dense form sections. */
    compact?: boolean;
  }>(),
  {
    required: false,
    disabled: false,
    compact: false,
  },
);
</script>
<template>
  <label
    class="field-label"
    :class="[
      compact ? 'text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]' : '',
      disabled ? 'text-[var(--text-muted)]' : '',
    ]"
    :for="for"
  >
    <slot />
    <span v-if="hint" class="ml-1 text-xs font-normal text-[var(--text-muted)]">{{ hint }}</span>
    <span v-if="required" class="text-[var(--text-error)]" aria-hidden="true">*</span>
  </label>
</template>
