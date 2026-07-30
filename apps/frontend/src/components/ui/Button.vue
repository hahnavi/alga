<script setup lang="ts">
import { computed } from "vue";
import { cva } from "class-variance-authority";
import { Loader2 } from "@lucide/vue";

const props = withDefaults(
  defineProps<{
    variant?: "default" | "primary" | "destructive" | "outline" | "link";
    size?: "sm" | "md";
    type?: "button" | "submit" | "reset";
    disabled?: boolean;
    loading?: boolean;
  }>(),
  {
    variant: "default",
    size: "md",
    type: "button",
    disabled: false,
    loading: false,
  },
);

const classes = computed(() =>
  cva(
    "inline-flex items-center justify-center gap-1 rounded border font-medium transition-colors cursor-pointer focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)] disabled:cursor-not-allowed disabled:opacity-50",
    {
      variants: {
        variant: {
          default:
            "border-[var(--btn-default-border)] bg-[var(--btn-default-bg)] text-[var(--btn-default-text)] hover:bg-[var(--btn-default-hover)]",
          primary:
            "border-transparent bg-[var(--accent)] text-white shadow-sm hover:bg-[var(--accent-strong)]",
          destructive:
            "border-[var(--btn-destructive-border)] bg-[var(--btn-destructive-bg)] text-[var(--btn-destructive-text)] shadow-sm hover:bg-[var(--btn-destructive-hover)]",
          outline:
            "border-[var(--btn-outline-border)] bg-[var(--btn-outline-bg)] text-[var(--btn-outline-text)] hover:bg-[var(--btn-outline-hover)]",
          link: "border-transparent bg-transparent text-[var(--text-tertiary)] underline hover:text-[var(--text-primary)]",
        },
        size: {
          sm: "px-2.5 py-1 text-xs",
          md: "px-3 py-1.5 text-sm",
        },
      },
    },
  )({ variant: props.variant, size: props.size }),
);
</script>

<template>
  <button :type="type" :class="classes" :disabled="disabled || loading">
    <Loader2 v-if="loading" class="h-4 w-4 animate-spin" />
    <slot v-else />
  </button>
</template>
