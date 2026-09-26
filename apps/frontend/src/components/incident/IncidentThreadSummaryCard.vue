<script setup lang="ts">
import { ChevronRight, MessageSquare, type LucideIcon } from "@lucide/vue";
import TypingIndicator from "@/components/ui/TypingIndicator.vue";
import { participantInitial, type ThreadParticipant } from "@/lib/incidentEvents";

defineOptions({ name: "IncidentThreadSummaryCard" });

/**
 * The collapsed summary card for one of the incident's two side threads
 * (coordination / investigation): participants stack, label, typing
 * indicator, message count, and the toggle button that opens the thread
 * drawer.
 */
const props = defineProps<{
  icon: LucideIcon;
  title: string;
  participants: ThreadParticipant[];
  participantLabel: string;
  messageCount: number;
  expanded: boolean;
  typing: boolean;
  emptyParticipantIcon?: LucideIcon;
}>();

const emit = defineEmits<{
  toggle: [];
}>();
</script>

<template>
  <div class="rounded border border-[var(--border-primary)] bg-[var(--bg-secondary)]">
    <div class="rounded-t">
      <div class="flex items-center justify-between gap-2 px-4 py-3">
        <div class="flex min-w-0 items-center gap-2">
          <h3 class="field-label mb-0">
            <component
              :is="props.icon"
              class="inline h-4 w-4 align-text-bottom text-[var(--text-muted)]"
            />
            {{ props.title }}
          </h3>
        </div>
      </div>

      <button
        type="button"
        class="flex w-full cursor-pointer items-center justify-between gap-3 border-t border-[var(--border-primary)] px-4 py-3 text-left transition-colors hover:bg-[var(--btn-default-hover)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
        :aria-expanded="props.expanded"
        aria-controls="incident-thread-drawer"
        @click="emit('toggle')"
      >
        <div class="flex min-w-0 items-center gap-2 text-sm">
          <div class="flex -space-x-1.5">
            <div
              v-for="participant in props.participants.slice(0, 3)"
              :key="participant.key"
              class="flex h-5 w-5 items-center justify-center overflow-hidden rounded-full border border-[var(--bg-secondary)] bg-[var(--bg-tertiary)] text-[10px] font-semibold text-[var(--text-secondary)]"
              :title="participant.name"
            >
              <img
                v-if="participant.avatarSrc"
                :src="participant.avatarSrc"
                :alt="participant.name"
                class="h-full w-full object-cover rounded-full"
                loading="lazy"
                decoding="async"
              />
              <span v-else>{{ participantInitial(participant.name) }}</span>
            </div>
            <div
              v-if="props.participants.length === 0"
              class="flex h-5 w-5 items-center justify-center rounded-full bg-[var(--bg-tertiary)] text-[var(--text-muted)]"
            >
              <component :is="props.emptyParticipantIcon ?? MessageSquare" class="h-3 w-3" />
            </div>
          </div>
          <span class="truncate font-semibold text-[var(--text-secondary)]">
            {{ props.participantLabel }}
          </span>
          <TypingIndicator v-if="props.typing" class="shrink-0" />
        </div>
        <div class="flex shrink-0 items-center gap-2">
          <span
            v-if="props.messageCount > 0"
            class="flex items-center gap-1 text-xs text-[var(--text-muted)]"
          >
            <MessageSquare class="h-3 w-3" />
            {{ props.messageCount }}
          </span>
          <ChevronRight
            class="h-5 w-5 text-[var(--text-muted)] transition-transform duration-200"
            :class="props.expanded ? 'rotate-180' : ''"
          />
        </div>
      </button>
    </div>
  </div>
</template>
