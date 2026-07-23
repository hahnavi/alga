<script setup lang="ts">
import { HatGlasses, User } from "@lucide/vue";

export type MentionItem = {
  id: string;
  label: string;
  mentionType: "user" | "agent";
  subtitle?: string;
};

defineProps<{
  items: MentionItem[];
  selectedIndex: number;
}>();

const emit = defineEmits<{
  select: [item: MentionItem];
  hover: [index: number];
}>();
</script>

<template>
  <div class="markdown-editor__mentions">
    <button
      v-for="(item, i) in items"
      :key="item.id"
      type="button"
      class="markdown-editor__mention-item"
      :class="{ 'markdown-editor__mention-item--selected': i === selectedIndex }"
      @click="emit('select', item)"
      @mouseenter="emit('hover', i)"
    >
      <User v-if="item.mentionType === 'user'" class="w-3.5 h-3.5 shrink-0" aria-hidden="true" />
      <HatGlasses v-else class="w-3.5 h-3.5 shrink-0" aria-hidden="true" />
      <span>{{ item.label }}</span>
      <span
        v-if="item.mentionType === 'agent'"
        class="markdown-editor__mention-badge markdown-editor__mention-badge--agent"
        >Agent</span
      >
      <span v-else-if="item.subtitle" class="markdown-editor__mention-role">{{
        item.subtitle
      }}</span>
    </button>
  </div>
</template>
