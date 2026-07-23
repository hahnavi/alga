<script setup lang="ts">
withDefaults(
  defineProps<{
    src?: string;
    letter?: string;
    /**
     * Tailwind class string applied to the avatar background (e.g. `bg-blue-500`).
     * Note: this is a class, not a CSS color, despite the legacy `bgColor` name.
     */
    bgClass?: string;
    title?: string;
    grayed?: boolean;
  }>(),
  { grayed: false },
);
</script>

<template>
  <div
    class="shrink-0 h-7 w-7 rounded-full flex items-center justify-center text-xs font-semibold text-white"
    :class="[bgClass, grayed ? 'opacity-50 grayscale' : '']"
    :title="title"
  >
    <img
      v-if="src"
      :src="src"
      alt=""
      class="h-full w-full object-cover rounded-full"
      loading="lazy"
      decoding="async"
    />
    <template v-else-if="letter">{{ letter }}</template>
    <slot v-else />
  </div>
</template>
