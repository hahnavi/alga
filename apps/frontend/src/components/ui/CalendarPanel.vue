<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { ChevronLeft, ChevronRight } from "@lucide/vue";
import {
  WEEKDAY_LABELS_SHORT,
  buildMonthGrid,
  isoDate,
  parseIsoDate,
  type CalendarCell,
} from "@/lib/calendar";
import IconBtn from "@/components/ui/IconBtn.vue";

const props = withDefaults(
  defineProps<{
    modelValue: string;
    minYear?: number;
    maxYear?: number;
    autoFocus?: boolean;
  }>(),
  {
    minYear: 1970,
    maxYear: 2100,
    autoFocus: true,
  },
);

const emit = defineEmits<{
  "update:modelValue": [date: string];
  close: [];
}>();

const today = new Date();

const parsedSelected = computed(() => parseIsoDate(props.modelValue));
const viewYear = ref(parsedSelected.value?.getFullYear() ?? today.getFullYear());
const viewMonth = ref(parsedSelected.value?.getMonth() ?? today.getMonth());
const focusedDate = ref<Date>(
  parsedSelected.value ?? new Date(today.getFullYear(), today.getMonth(), 1),
);
const gridRef = ref<HTMLElement | null>(null);

const cells = computed<CalendarCell[]>(() =>
  buildMonthGrid(viewYear.value, viewMonth.value, today),
);

const monthLabel = computed(() =>
  new Date(viewYear.value, viewMonth.value, 1).toLocaleDateString(undefined, {
    month: "long",
    year: "numeric",
  }),
);

const isPrevDisabled = computed(
  () =>
    viewYear.value < props.minYear || (viewYear.value === props.minYear && viewMonth.value === 0),
);
const isNextDisabled = computed(
  () =>
    viewYear.value > props.maxYear || (viewYear.value === props.maxYear && viewMonth.value === 11),
);

function focusCell(date: Date) {
  focusedDate.value = date;
  void nextTick(() => {
    const root = gridRef.value;
    if (!root) return;
    const target = root.querySelector<HTMLElement>(`[data-date="${isoDate(date)}"]`);
    target?.focus();
  });
}

function selectDay(date: Date) {
  emit("update:modelValue", isoDate(date));
  emit("close");
}

function goToday() {
  const d = new Date(today.getFullYear(), today.getMonth(), today.getDate());
  viewYear.value = d.getFullYear();
  viewMonth.value = d.getMonth();
  focusCell(d);
}

function prevMonth() {
  if (isPrevDisabled.value) return;
  const d = new Date(viewYear.value, viewMonth.value - 1, 1);
  viewYear.value = d.getFullYear();
  viewMonth.value = d.getMonth();
}

function nextMonth() {
  if (isNextDisabled.value) return;
  const d = new Date(viewYear.value, viewMonth.value + 1, 1);
  viewYear.value = d.getFullYear();
  viewMonth.value = d.getMonth();
}

function onKeydown(ev: KeyboardEvent) {
  const cur = focusedDate.value;
  switch (ev.key) {
    case "ArrowLeft":
      ev.preventDefault();
      focusCell(new Date(cur.getFullYear(), cur.getMonth(), cur.getDate() - 1));
      break;
    case "ArrowRight":
      ev.preventDefault();
      focusCell(new Date(cur.getFullYear(), cur.getMonth(), cur.getDate() + 1));
      break;
    case "ArrowUp":
      ev.preventDefault();
      focusCell(new Date(cur.getFullYear(), cur.getMonth(), cur.getDate() - 7));
      break;
    case "ArrowDown":
      ev.preventDefault();
      focusCell(new Date(cur.getFullYear(), cur.getMonth(), cur.getDate() + 7));
      break;
    case "PageUp":
      ev.preventDefault();
      focusCell(new Date(cur.getFullYear(), cur.getMonth() - 1, cur.getDate()));
      break;
    case "PageDown":
      ev.preventDefault();
      focusCell(new Date(cur.getFullYear(), cur.getMonth() + 1, cur.getDate()));
      break;
    case "Home":
      ev.preventDefault();
      focusCell(new Date(cur.getFullYear(), cur.getMonth(), 1));
      break;
    case "End":
      ev.preventDefault();
      focusCell(new Date(cur.getFullYear(), cur.getMonth() + 1, 0));
      break;
    case "Enter":
    case " ":
      ev.preventDefault();
      selectDay(cur);
      break;
  }
}

watch(
  () => props.modelValue,
  (v) => {
    const parsed = parseIsoDate(v);
    if (parsed) {
      viewYear.value = parsed.getFullYear();
      viewMonth.value = parsed.getMonth();
      focusedDate.value = parsed;
    }
  },
);

watch(
  () => props.autoFocus,
  (v) => {
    if (v) {
      void nextTick(() => {
        const root = gridRef.value;
        if (!root) return;
        const target =
          root.querySelector<HTMLElement>(`[data-date="${isoDate(focusedDate.value)}"]`) ??
          root.querySelector<HTMLElement>("[data-date]");
        target?.focus();
      });
    }
  },
  { immediate: true },
);
</script>

<template>
  <div class="flex flex-col">
    <div class="flex items-center justify-between gap-1 px-1 pb-2">
      <IconBtn
        :icon="ChevronLeft"
        label="Previous month"
        size="sm"
        :disabled="isPrevDisabled"
        @click="prevMonth"
      />
      <div
        class="flex-1 text-center text-sm font-semibold tracking-tight text-[var(--text-primary)]"
        aria-live="polite"
      >
        {{ monthLabel }}
      </div>
      <IconBtn
        :icon="ChevronRight"
        label="Next month"
        size="sm"
        :disabled="isNextDisabled"
        @click="nextMonth"
      />
    </div>

    <div
      ref="gridRef"
      role="grid"
      :aria-label="`Calendar for ${monthLabel}`"
      class="grid grid-cols-7 gap-0.5"
      @keydown="onKeydown"
    >
      <div
        v-for="(label, i) in WEEKDAY_LABELS_SHORT"
        :key="`wd-${i}`"
        role="columnheader"
        class="flex h-7 items-center justify-center text-[0.65rem] font-semibold uppercase tracking-wider text-[var(--text-muted)]"
      >
        {{ label.slice(0, 2) }}
      </div>
      <button
        v-for="cell in cells"
        :key="cell.key"
        type="button"
        role="gridcell"
        :data-date="isoDate(cell.date)"
        :tabindex="isoDate(cell.date) === isoDate(focusedDate) ? 0 : -1"
        :aria-selected="isoDate(cell.date) === modelValue"
        :aria-label="
          cell.date.toLocaleDateString(undefined, {
            weekday: 'long',
            year: 'numeric',
            month: 'long',
            day: 'numeric',
          })
        "
        class="inline-flex h-8 w-full cursor-pointer items-center justify-center rounded text-xs font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
        :class="[
          isoDate(cell.date) === modelValue
            ? 'bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-primary)]'
            : cell.isToday && cell.inCurrentMonth
              ? 'text-[var(--accent-primary)] hover:bg-[var(--bg-secondary)]'
              : cell.inCurrentMonth
                ? 'text-[var(--text-primary)] hover:bg-[var(--bg-secondary)]'
                : 'text-[var(--text-muted)] hover:bg-[var(--bg-secondary)] hover:text-[var(--text-secondary)]',
        ]"
        @click="selectDay(cell.date)"
        @focus="focusedDate = cell.date"
      >
        {{ cell.date.getDate() }}
      </button>
    </div>

    <div
      class="mt-2 flex items-center justify-between border-t border-[var(--border-primary)] pt-2"
    >
      <button
        type="button"
        class="inline-flex h-7 cursor-pointer items-center rounded px-2 text-xs font-medium text-[var(--accent-primary)] transition-colors hover:bg-[var(--bg-secondary)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
        @click="goToday"
      >
        Today
      </button>
    </div>
  </div>
</template>
