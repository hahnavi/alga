<script setup lang="ts" generic="T extends ScheduleShift">
import { computed, type Component } from "vue";
import type { ScheduleShift } from "@/lib/api";
import {
  getParticipants,
  shiftStyle,
  shiftWithinRange,
  type TimeTick,
} from "@/lib/scheduleTimeline";

const props = defineProps<{
  /** Display name for the lane. */
  title: string;
  /** Lane icon (lucide-vue-next component). */
  icon: Component;
  /** Color of the lane icon (Tailwind text-* class). */
  iconClass: string;
  /** Background color class for shift blocks. */
  shiftClass: string;
  /** Source color class for shift blocks (when source is "override"). */
  overrideShiftClass?: string;
  /** Whether to fall back to source-aware coloring (used by the "final" lane). */
  shiftColorBySource?: boolean;
  /** Shifts in this lane. */
  shifts: T[];
  /** Range start (ms) and end (ms). */
  rangeStartMs: number;
  rangeEndMs: number;
  /** Total range span in ms. */
  rangeMs: number;
  /** Function that returns the user display name for a shift. */
  nameForUserId: (id: string) => string;
  /** Time-scale ticks (used for grid separators). */
  timeScale: TimeTick[];
  /** Empty-state label when no participants. */
  emptyLabel: string;
}>();

const userIds = computed(() => getParticipants(props.shifts));
</script>

<template>
  <div class="space-y-1 pt-3 first:pt-2">
    <div class="flex items-center gap-2 border-b border-[var(--border-secondary)] py-1.5">
      <div
        class="flex w-32 shrink-0 items-center gap-1.5 px-2 text-xs font-semibold text-[var(--text-primary)]"
      >
        <component :is="icon" :class="['h-3.5 w-3.5', iconClass]" />
        {{ title }}
      </div>
      <div class="flex-1"></div>
    </div>
    <div class="relative space-y-1">
      <template v-if="userIds.length === 0">
        <div class="flex items-center gap-2 border-b border-[var(--border-secondary)] py-1">
          <div class="w-32 shrink-0 px-2 text-xs text-[var(--text-muted)]">{{ emptyLabel }}</div>
          <div class="relative h-7 flex-1 rounded bg-[var(--bg-secondary)]"></div>
        </div>
      </template>
      <div
        v-for="uid in userIds"
        :key="uid"
        class="flex items-center gap-2 border-b border-[var(--border-secondary)] py-1"
      >
        <div class="w-32 shrink-0 truncate px-2 text-xs text-[var(--text-secondary)]">
          {{ nameForUserId(uid) }}
        </div>
        <div class="relative h-7 flex-1 rounded bg-[var(--bg-secondary)]">
          <template
            v-for="s in (shifts as ScheduleShift[]).filter(
              (x) => x.user_id === uid && shiftWithinRange(x, rangeStartMs, rangeEndMs),
            )"
            :key="s.start"
          >
            <div
              :class="[
                'absolute top-0.5 bottom-0.5 z-[2] truncate rounded px-1.5 text-[0.65rem] font-medium text-white',
                shiftColorBySource
                  ? s.source === 'override'
                    ? (overrideShiftClass ?? shiftClass)
                    : shiftClass
                  : shiftClass,
              ]"
              :style="shiftStyle(s, rangeStartMs, rangeMs)"
              :title="`${nameForUserId(uid)} (${(s as ScheduleShift).source})`"
            >
              {{ nameForUserId(uid) }}
            </div>
          </template>
        </div>
      </div>
      <!-- Grid separators -->
      <div
        v-for="(tick, i) in timeScale"
        :key="`sep-${i}`"
        class="pointer-events-none absolute top-0 bottom-0 z-[1] w-px bg-[var(--border-secondary)]"
        :style="{ left: `calc(8.5rem + (100% - 8.5rem) * ${tick.pct / 100})` }"
      ></div>
    </div>
  </div>
</template>
