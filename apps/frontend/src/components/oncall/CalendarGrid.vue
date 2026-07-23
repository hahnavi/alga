<script setup lang="ts">
import type { ScheduleShift } from "@/lib/api";
import { WEEK_DAYS, type CalendarCell } from "@/lib/scheduleTimeline";

defineProps<{
  cells: CalendarCell[];
  nameForUserId: (id: string) => string;
}>();

function isCurrentWeek(weekStartCell: Date): boolean {
  const now = new Date();
  const lead = (now.getDay() + 6) % 7;
  const ws = new Date(weekStartCell);
  ws.setDate(weekStartCell.getDate() - lead);
  ws.setHours(0, 0, 0, 0);
  const start = new Date(now);
  start.setDate(now.getDate() - lead);
  start.setHours(0, 0, 0, 0);
  return ws.getTime() === start.getTime();
}
</script>

<template>
  <div class="overflow-x-auto">
    <div class="min-w-[640px] overflow-hidden rounded-md border border-[var(--border-primary)]">
      <div
        class="grid grid-cols-[3rem_repeat(7,1fr)] border-b border-[var(--border-primary)] bg-[var(--bg-secondary)]"
      >
        <div
          class="px-2 py-1.5 text-center text-[0.65rem] font-semibold uppercase tracking-wider text-[var(--text-muted)]"
        >
          Wk
        </div>
        <div
          v-for="d in WEEK_DAYS"
          :key="d"
          class="px-2 py-1.5 text-center text-[0.65rem] font-semibold uppercase tracking-wider text-[var(--text-muted)]"
        >
          {{ d }}
        </div>
      </div>
      <div class="grid grid-cols-[3rem_repeat(7,1fr)]">
        <template v-for="(cell, i) in cells" :key="i">
          <div
            v-if="i % 7 === 0"
            :class="[
              'flex items-start justify-center border-b border-r border-[var(--border-secondary)] py-2 text-[0.65rem] font-medium',
              isCurrentWeek(cell.date)
                ? 'bg-[var(--accent-primary)]/10 text-[var(--accent-primary)]'
                : 'bg-[var(--bg-secondary)] text-[var(--text-muted)]',
              i >= 35 ? 'border-b-0' : '',
            ]"
          >
            W{{ cell.weekNumber }}
          </div>
          <div
            :class="[
              'min-h-[6rem] border-b border-r border-[var(--border-secondary)] p-1.5 transition-colors hover:bg-[var(--bg-secondary)]/60',
              !cell.inMonth ? 'bg-[var(--bg-secondary)]/40 opacity-50' : '',
              cell.isToday ? 'ring-1 ring-inset ring-[var(--accent-primary)]' : '',
              i >= 35 ? 'border-b-0' : '',
            ]"
          >
            <div class="mb-1 flex items-center justify-between">
              <span
                :class="[
                  'inline-flex h-5 w-5 items-center justify-center rounded-full text-[0.7rem] font-medium',
                  cell.isToday
                    ? 'bg-[var(--accent-primary)] text-white'
                    : cell.isWeekend
                      ? 'text-[var(--text-muted)]'
                      : 'text-[var(--text-secondary)]',
                ]"
              >
                {{ cell.date.getDate() }}
              </span>
              <span
                v-if="cell.shifts.length > 0"
                class="rounded-full bg-[var(--bg-secondary)] px-1.5 py-0.5 text-[0.6rem] font-medium text-[var(--text-muted)]"
              >
                {{ cell.shifts.length }}
              </span>
            </div>
            <div class="space-y-0.5">
              <div
                v-for="s in (cell.shifts as ScheduleShift[]).slice(0, 3)"
                :key="s.start"
                :class="[
                  'flex items-center gap-1 truncate rounded px-1 py-0.5 text-[0.6rem] font-medium',
                  s.source === 'override'
                    ? 'bg-amber-500/20 text-amber-700 dark:text-amber-300'
                    : 'bg-emerald-500/20 text-emerald-700 dark:text-emerald-300',
                ]"
                :title="`${nameForUserId(s.user_id)} (${s.source})`"
              >
                <span
                  :class="[
                    'inline-block h-1.5 w-1.5 shrink-0 rounded-full',
                    s.source === 'override' ? 'bg-amber-500' : 'bg-emerald-500',
                  ]"
                />
                <span class="truncate">{{ nameForUserId(s.user_id) }}</span>
              </div>
              <div v-if="cell.shifts.length > 3" class="text-[0.6rem] text-[var(--text-muted)]">
                +{{ cell.shifts.length - 3 }} more
              </div>
            </div>
          </div>
        </template>
      </div>
    </div>
  </div>
</template>
