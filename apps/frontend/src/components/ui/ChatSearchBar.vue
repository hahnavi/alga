<script setup lang="ts">
import { nextTick, onMounted, ref } from "vue";
import { ChevronDown, ChevronUp, Search, X } from "@lucide/vue";
import IconBtn from "@/components/ui/IconBtn.vue";

defineProps<{
  query: string;
  matchCount: number;
  currentIndex: number;
}>();

const emit = defineEmits<{
  "update:query": [value: string];
  next: [];
  prev: [];
  close: [];
}>();

const inputRef = ref<HTMLInputElement | null>(null);

onMounted(() => {
  nextTick(() => {
    inputRef.value?.focus();
  });
});
</script>

<template>
  <div class="flex items-center gap-2 py-2">
    <Search class="h-4 w-4 shrink-0 text-[var(--text-muted)]" />
    <input
      ref="inputRef"
      :value="query"
      type="text"
      placeholder="Search messages..."
      class="min-w-0 flex-1 bg-transparent text-sm text-[var(--text-primary)] placeholder:text-[var(--text-muted)] outline-none"
      @input="emit('update:query', ($event.target as HTMLInputElement).value)"
    />
    <span
      v-if="query"
      class="shrink-0 whitespace-nowrap text-xs tabular-nums text-[var(--text-muted)]"
    >
      {{ matchCount === 0 ? "No results" : `${currentIndex + 1} of ${matchCount}` }}
    </span>
    <div class="flex items-center">
      <IconBtn
        :icon="ChevronUp"
        label="Previous match (Shift+Enter)"
        size="sm"
        :disabled="matchCount === 0"
        @click="emit('prev')"
      />
      <IconBtn
        :icon="ChevronDown"
        label="Next match (Enter)"
        size="sm"
        :disabled="matchCount === 0"
        @click="emit('next')"
      />
      <IconBtn :icon="X" label="Close (Escape)" size="sm" @click="emit('close')" />
    </div>
  </div>
</template>
