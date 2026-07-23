<script setup lang="ts">
import { Loader2 } from "@lucide/vue";
import Card from "./Card.vue";

defineOptions({ inheritAttrs: false });

withDefaults(
  defineProps<{
    loading?: boolean;
  }>(),
  { loading: false },
);

defineEmits<{
  navigate: [];
}>();
</script>

<template>
  <Card
    v-bind="$attrs"
    class="relative cursor-pointer transition-all duration-150 hover:border-[var(--border-secondary)] hover:shadow-md focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
    :class="loading ? 'pointer-events-none opacity-70' : ''"
    role="button"
    :tabindex="loading ? -1 : 0"
    :aria-busy="loading"
    @click="!loading && $emit('navigate')"
    @keydown.enter="!loading && $emit('navigate')"
    @keydown.space.prevent="!loading && $emit('navigate')"
  >
    <div
      v-if="loading"
      class="absolute inset-0 z-10 flex items-center justify-center rounded-[inherit] bg-[var(--bg-primary)]/60"
    >
      <Loader2 class="h-5 w-5 animate-spin text-[var(--text-muted)]" />
    </div>
    <slot />
  </Card>
</template>
