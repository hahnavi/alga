<script setup lang="ts">
import type { PlaybookRecord } from "@/lib/api";
import Card from "@/components/ui/Card.vue";
import { FileText } from "@lucide/vue";

defineProps<{
  playbook: PlaybookRecord;
}>();

defineEmits<{
  click: [];
}>();
</script>

<template>
  <Card class="cursor-pointer transition-shadow hover:shadow-md" @click="$emit('click')">
    <div class="flex items-start justify-between gap-2">
      <div class="min-w-0 flex-1 space-y-1">
        <div class="flex items-center gap-2">
          <FileText class="h-4 w-4 shrink-0 text-[var(--text-muted)]" />
          <p class="truncate text-sm font-medium">{{ playbook.title }}</p>
        </div>
        <div class="flex items-center gap-2">
          <span
            class="rounded px-1.5 py-0.5 text-xs font-semibold uppercase"
            :class="
              playbook.kind === 'mitigation'
                ? 'bg-[var(--bg-code)] text-orange-500'
                : 'bg-[var(--bg-code)] text-blue-500'
            "
          >
            {{ playbook.kind }}
          </span>
          <span v-if="playbook.steps?.length" class="text-xs text-[var(--text-muted)]">
            {{ playbook.steps.length }} step{{ playbook.steps.length !== 1 ? "s" : "" }}
          </span>
        </div>
        <p
          v-if="playbook.summary"
          class="line-clamp-2 whitespace-pre-wrap text-xs text-[var(--text-secondary)]"
        >
          {{ playbook.summary }}
        </p>
        <div v-if="playbook.tags?.length" class="flex flex-wrap gap-1">
          <span
            v-for="tag in playbook.tags"
            :key="tag"
            class="rounded bg-[var(--bg-code)] px-1.5 py-0.5 text-xs font-semibold uppercase tracking-wide text-[var(--text-secondary)]"
          >
            {{ tag }}
          </span>
        </div>
      </div>
    </div>
  </Card>
</template>
