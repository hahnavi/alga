<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ChevronDown, ChevronLeft, ChevronRight } from "@lucide/vue";
import { useDropdownLifecycle } from "@/composables/useDropdownLifecycle";
import IconBtn from "@/components/ui/IconBtn.vue";

const props = withDefaults(
  defineProps<{
    modelValue: Date;
    minYear?: number;
    maxYear?: number;
    showToday?: boolean;
  }>(),
  {
    minYear: 1970,
    maxYear: 2100,
    showToday: true,
  },
);

const emit = defineEmits<{
  "update:modelValue": [date: Date];
}>();

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

const FULL_MONTHS = [
  "January",
  "February",
  "March",
  "April",
  "May",
  "June",
  "July",
  "August",
  "September",
  "October",
  "November",
  "December",
];

const open = ref(false);
const rootRef = ref<HTMLElement | null>(null);

useDropdownLifecycle(open, rootRef);

const today = new Date();
const todayYear = today.getFullYear();
const todayMonth = today.getMonth();

const viewYear = ref(props.modelValue.getFullYear());

const currentYear = computed(() => props.modelValue.getFullYear());
const currentMonth = computed(() => props.modelValue.getMonth());

const triggerLabel = computed(() => `${FULL_MONTHS[currentMonth.value]} ${currentYear.value}`);

const isViewYearAtMin = computed(() => viewYear.value <= props.minYear);
const isViewYearAtMax = computed(() => viewYear.value >= props.maxYear);

function toggle() {
  open.value = !open.value;
}

function prevYear() {
  if (isViewYearAtMin.value) return;
  viewYear.value -= 1;
}

function nextYear() {
  if (isViewYearAtMax.value) return;
  viewYear.value += 1;
}

function selectMonth(month: number) {
  const next = new Date(props.modelValue);
  next.setFullYear(viewYear.value, month, 1);
  emit("update:modelValue", next);
  open.value = false;
}

function goToday() {
  emit("update:modelValue", new Date(todayYear, todayMonth, 1));
  open.value = false;
}

watch(open, (isOpen) => {
  if (isOpen) {
    viewYear.value = props.modelValue.getFullYear();
  }
});
</script>

<template>
  <div ref="rootRef" class="relative inline-block">
    <button
      type="button"
      :aria-expanded="open"
      aria-haspopup="dialog"
      class="inline-flex min-w-[10rem] cursor-pointer items-center justify-between gap-2 rounded border border-[var(--border-primary)] bg-[var(--bg-card)] px-3 py-1 text-sm font-semibold tracking-tight text-[var(--text-primary)] transition-colors hover:bg-[var(--bg-secondary)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
      @click="toggle"
    >
      <span>{{ triggerLabel }}</span>
      <ChevronDown
        class="h-3.5 w-3.5 text-[var(--text-muted)] transition-transform duration-200"
        :class="open ? 'rotate-180' : ''"
      />
    </button>

    <Transition
      enter-active-class="transition duration-150 ease-out"
      enter-from-class="opacity-0 -translate-y-1"
      enter-to-class="opacity-100 translate-y-0"
      leave-active-class="transition duration-100 ease-in"
      leave-from-class="opacity-100 translate-y-0"
      leave-to-class="opacity-0 -translate-y-1"
    >
      <div
        v-if="open"
        role="dialog"
        aria-label="Select month and year"
        class="absolute right-0 z-50 mt-1 w-64 overflow-hidden rounded border border-[var(--border-primary)] bg-[var(--bg-dialog)] p-2 shadow-xl"
      >
        <div class="mb-1 flex items-center justify-between gap-1 px-1">
          <IconBtn
            :icon="ChevronLeft"
            label="Previous year"
            size="sm"
            :disabled="isViewYearAtMin"
            @click="prevYear"
          />
          <span
            class="flex-1 text-center text-sm font-semibold text-[var(--text-primary)]"
            aria-live="polite"
          >
            {{ viewYear }}
          </span>
          <IconBtn
            :icon="ChevronRight"
            label="Next year"
            size="sm"
            :disabled="isViewYearAtMax"
            @click="nextYear"
          />
        </div>

        <div class="grid grid-cols-3 gap-1">
          <button
            v-for="(label, i) in MONTHS"
            :key="i"
            type="button"
            :class="[
              'inline-flex h-9 cursor-pointer items-center justify-center rounded text-xs font-medium transition-colors focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]',
              viewYear === currentYear && i === currentMonth
                ? 'bg-[var(--accent-primary)] text-white hover:bg-[var(--accent-primary)]'
                : viewYear === todayYear && i === todayMonth
                  ? 'text-[var(--accent-primary)] hover:bg-[var(--bg-secondary)]'
                  : 'text-[var(--text-secondary)] hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)]',
            ]"
            :aria-label="`${FULL_MONTHS[i]} ${viewYear}`"
            :aria-pressed="viewYear === currentYear && i === currentMonth"
            @click="selectMonth(i)"
          >
            {{ label }}
          </button>
        </div>

        <div
          v-if="showToday"
          class="mt-1 flex items-center justify-center border-t border-[var(--border-primary)] pt-1"
        >
          <button
            type="button"
            class="inline-flex h-7 cursor-pointer items-center justify-center rounded px-3 text-xs font-medium text-[var(--accent-primary)] transition-colors hover:bg-[var(--bg-secondary)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
            @click="goToday"
          >
            Today
          </button>
        </div>
      </div>
    </Transition>
  </div>
</template>
