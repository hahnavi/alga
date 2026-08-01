<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import {
  Calendar,
  Clock,
  ChevronLeft,
  ChevronRight,
  Download,
  Repeat,
  CalendarDays,
  UserCheck,
} from "@lucide/vue";
import Button from "@/components/ui/Button.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import LoadingSpinner from "@/components/ui/LoadingSpinner.vue";
import MonthYearPicker from "@/components/ui/MonthYearPicker.vue";
import { api } from "@/lib/api";
import type { ScheduleShift, UserInfo } from "@/lib/api";
import { resolveDisplayName } from "@/lib/userDisplay";
import { useToast } from "@/lib/toast";
import TimelineLane from "./TimelineLane.vue";
import CalendarGrid from "./CalendarGrid.vue";
import {
  RANGE_DAYS,
  RANGE_LABEL,
  buildCalendarDays,
  buildTimeScale,
  computeRangeBounds,
  formatRangeLabel,
  startOfDay,
  type TimelineRange,
} from "@/lib/scheduleTimeline";

type ViewKind = "timeline" | "calendar";

const props = defineProps<{
  scheduleId: string;
  users: UserInfo[];
  canEdit: boolean;
}>();

const { push } = useToast();

const view = ref<ViewKind>("timeline");
const range = ref<TimelineRange>("1w");
const cursor = ref(startOfDay(new Date()));

const rangeStart = computed(() => startOfDay(cursor.value));
const rangeMs = computed(() => RANGE_DAYS[range.value] * 24 * 60 * 60 * 1000);
const rangeEnd = computed(() => {
  const d = new Date(rangeStart.value);
  d.setDate(d.getDate() + RANGE_DAYS[range.value]);
  return d;
});

const rangeBounds = computed(() => computeRangeBounds(cursor.value, view.value, range.value));

const shifts = ref<ScheduleShift[]>([]);
const loading = ref(false);

async function load() {
  if (!props.scheduleId) return;
  loading.value = true;
  try {
    shifts.value = await api.getScheduleTimeline(props.scheduleId, rangeBounds.value);
  } catch (err) {
    shifts.value = [];
    push("Failed to load schedule", "error");
  } finally {
    loading.value = false;
  }
}

watch([rangeBounds, () => props.scheduleId], load, { immediate: true });

watch(view, (next) => {
  if (next === "calendar") {
    const d = startOfDay(new Date());
    d.setDate(1);
    cursor.value = d;
  }
});

const rotationShifts = computed(() => shifts.value.filter((s) => s.source === "rotation"));
const overrideShifts = computed(() => shifts.value.filter((s) => s.source === "override"));
const finalShifts = computed(() => shifts.value);

const timeScale = computed(() =>
  buildTimeScale(rangeStart.value, range.value, RANGE_DAYS[range.value]),
);

const calendarDays = computed(() => buildCalendarDays(cursor.value, shifts.value));

const rangeLabelText = computed(() => formatRangeLabel(rangeStart.value, range.value));

function prev() {
  if (view.value === "calendar") {
    const d = new Date(cursor.value);
    d.setMonth(d.getMonth() - 1);
    cursor.value = d;
  } else {
    const d = new Date(cursor.value);
    d.setDate(d.getDate() - RANGE_DAYS[range.value]);
    cursor.value = d;
  }
}
function next() {
  if (view.value === "calendar") {
    const d = new Date(cursor.value);
    d.setMonth(d.getMonth() + 1);
    cursor.value = d;
  } else {
    const d = new Date(cursor.value);
    d.setDate(d.getDate() + RANGE_DAYS[range.value]);
    cursor.value = d;
  }
}
function goToday() {
  if (view.value === "calendar") {
    const d = startOfDay(new Date());
    d.setDate(1);
    cursor.value = d;
  } else {
    cursor.value = startOfDay(new Date());
  }
}

const now = ref(Date.now());
let nowTimer: number | undefined;
onMounted(() => {
  nowTimer = setInterval(() => {
    now.value = Date.now();
  }, 30_000);
});
onBeforeUnmount(() => {
  if (nowTimer) clearInterval(nowTimer);
});

const nowFraction = computed(() => {
  return (now.value - rangeStart.value.getTime()) / rangeMs.value;
});
const nowInRange = computed(() => nowFraction.value >= 0 && nowFraction.value <= 1);
const nowLeftStyle = computed(() => ({
  left: `calc(8.5rem + (100% - 8.5rem) * ${nowFraction.value})`,
}));
const nowLabel = computed(() => {
  const d = new Date(now.value);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`;
});

function nameForUserId(userId: string): string {
  const s = shifts.value.find((x) => x.user_id === userId);
  if (s?.user_display_name) return s.user_display_name;
  return resolveDisplayName({ userId, users: props.users, fallback: userId.slice(0, 8) });
}

function downloadICal() {
  const url = api.scheduleICalUrl(props.scheduleId);
  window.open(url, "_blank", "noopener,noreferrer");
}
</script>

<template>
  <div class="space-y-3">
    <!-- Controls: time range + view mode + navigation -->
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div class="flex flex-wrap items-center gap-2">
        <div
          v-if="view === 'timeline'"
          class="inline-flex rounded-md border border-[var(--border-primary)] p-0.5"
        >
          <button
            v-for="opt in ['1d', '1w', '2w', '1m'] as TimelineRange[]"
            :key="opt"
            :class="[
              'inline-flex items-center rounded px-2.5 py-1 text-xs font-medium transition-colors',
              range === opt
                ? 'bg-[var(--accent-primary)] text-white'
                : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]',
            ]"
            @click="range = opt"
          >
            {{ RANGE_LABEL[opt] }}
          </button>
        </div>
        <div class="inline-flex rounded-md border border-[var(--border-primary)] p-0.5">
          <button
            v-for="opt in ['timeline', 'calendar'] as ViewKind[]"
            :key="opt"
            :class="[
              'inline-flex items-center gap-1.5 rounded px-2.5 py-1 text-xs font-medium capitalize transition-colors',
              view === opt
                ? 'bg-[var(--accent-primary)] text-white'
                : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]',
            ]"
            @click="view = opt"
          >
            <component :is="opt === 'calendar' ? Calendar : Clock" class="h-3.5 w-3.5" />
            {{ opt }}
          </button>
        </div>
        <div class="hidden items-center gap-2 sm:flex">
          <span
            class="inline-flex items-center gap-1 rounded-full bg-blue-500/20 px-2 py-0.5 text-[0.65rem] font-medium text-blue-600 dark:text-blue-300"
          >
            <span class="h-1.5 w-1.5 rounded-full bg-blue-500" /> Rotation
          </span>
          <span
            class="inline-flex items-center gap-1 rounded-full bg-amber-500/20 px-2 py-0.5 text-[0.65rem] font-medium text-amber-700 dark:text-amber-300"
          >
            <span class="h-1.5 w-1.5 rounded-full bg-amber-500" /> Override
          </span>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <template v-if="view === 'calendar'">
          <Button variant="outline" size="sm" title="Previous month" @click="prev">
            <ChevronLeft class="h-3.5 w-3.5" />
          </Button>
          <MonthYearPicker v-model="cursor" />
          <Button variant="outline" size="sm" title="Next month" @click="next">
            <ChevronRight class="h-3.5 w-3.5" />
          </Button>
          <Button variant="outline" size="sm" @click="goToday">Today</Button>
        </template>

        <template v-else>
          <Button variant="outline" size="sm" @click="prev">
            <ChevronLeft class="h-3.5 w-3.5" />
          </Button>
          <Button variant="outline" size="sm" @click="goToday">Today</Button>
          <Button variant="outline" size="sm" @click="next">
            <ChevronRight class="h-3.5 w-3.5" />
          </Button>
          <span class="min-w-[9rem] text-center text-xs font-medium text-[var(--text-secondary)]">
            {{ rangeLabelText }}
          </span>
        </template>

        <Button variant="outline" size="sm" title="Export to iCal" @click="downloadICal">
          <Download class="h-3.5 w-3.5" /> iCal
        </Button>
      </div>
    </div>

    <LoadingSpinner v-if="loading" centered />
    <EmptyState
      v-else-if="view === 'timeline' && shifts.length === 0"
      message="No on-call shifts in this range."
    />

    <div v-else-if="view === 'timeline'" class="overflow-x-auto">
      <div class="relative min-w-[640px]">
        <div
          v-if="nowInRange"
          class="pointer-events-none absolute top-0 bottom-0 z-10 w-px bg-rose-500"
          :style="nowLeftStyle"
        >
          <span
            class="absolute top-0 left-0 -translate-x-1/2 whitespace-nowrap rounded bg-rose-500 px-1 text-[0.6rem] font-medium text-white shadow-sm"
          >
            {{ nowLabel }}
          </span>
        </div>
        <div class="flex gap-2 border-b border-[var(--border-primary)]">
          <div class="w-32 shrink-0"></div>
          <div class="relative h-6 flex-1">
            <div
              v-for="(tick, i) in timeScale"
              :key="`hsep-${i}`"
              class="pointer-events-none absolute top-0 bottom-0 w-px bg-[var(--border-secondary)]"
              :style="{ left: `${tick.pct}%` }"
            ></div>
            <div
              v-for="(tick, i) in timeScale"
              :key="i"
              class="absolute top-0 h-full"
              :style="{ left: `${tick.pct}%` }"
            >
              <span
                v-if="tick.label"
                :class="[
                  'absolute top-0.5 left-1 whitespace-nowrap text-[0.65rem] text-[var(--text-muted)]',
                  tick.isToday ? 'font-semibold text-[var(--accent-primary)]' : '',
                ]"
              >
                {{ tick.label }}
              </span>
            </div>
          </div>
        </div>

        <TimelineLane
          title="Rotations"
          :icon="Repeat"
          icon-class="text-blue-500"
          shift-class="bg-blue-500/80"
          :shifts="rotationShifts"
          :range-start-ms="rangeStart.getTime()"
          :range-end-ms="rangeEnd.getTime()"
          :range-ms="rangeMs"
          :name-for-user-id="nameForUserId"
          :time-scale="timeScale"
          empty-label="No rotations"
        />
        <TimelineLane
          title="Overrides"
          :icon="CalendarDays"
          icon-class="text-amber-500"
          shift-class="bg-amber-500/80"
          :shifts="overrideShifts"
          :range-start-ms="rangeStart.getTime()"
          :range-end-ms="rangeEnd.getTime()"
          :range-ms="rangeMs"
          :name-for-user-id="nameForUserId"
          :time-scale="timeScale"
          empty-label="No overrides"
        />
        <TimelineLane
          title="Final On-Call"
          :icon="UserCheck"
          icon-class="text-emerald-500"
          shift-class="bg-emerald-500/80"
          override-shift-class="bg-amber-500/80"
          shift-color-by-source
          :shifts="finalShifts"
          :range-start-ms="rangeStart.getTime()"
          :range-end-ms="rangeEnd.getTime()"
          :range-ms="rangeMs"
          :name-for-user-id="nameForUserId"
          :time-scale="timeScale"
          empty-label="No shifts"
        />
      </div>
    </div>

    <CalendarGrid v-else :cells="calendarDays" :name-for-user-id="nameForUserId" />
  </div>
</template>
