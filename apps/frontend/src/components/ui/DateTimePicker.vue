<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { Calendar, ChevronDown, X } from "@lucide/vue";
import { useDropdownLifecycle } from "@/composables/useDropdownLifecycle";
import { formatIsoDatetime, isoDate, parseIsoDate, splitIsoDatetime } from "@/lib/calendar";
import CalendarPanel from "@/components/ui/CalendarPanel.vue";
import TimeSpinner from "@/components/ui/TimeSpinner.vue";

const props = withDefaults(
  defineProps<{
    id?: string;
    modelValue: string;
    placeholder?: string;
    disabled?: boolean;
    minYear?: number;
    maxYear?: number;
    error?: string;
  }>(),
  {
    placeholder: "Select date and time",
    minYear: 1970,
    maxYear: 2100,
  },
);

const emit = defineEmits<{
  "update:modelValue": [value: string];
}>();

const open = ref(false);
const rootRef = ref<HTMLElement | null>(null);

useDropdownLifecycle(open, rootRef);

const split = computed(() => splitIsoDatetime(props.modelValue));
const selectedDate = computed(() => split.value.date);
const selectedTime = computed(() => (split.value.time ? split.value.time : "09:00"));

const parsedSelected = computed(() => parseIsoDate(selectedDate.value));

const displayLabel = computed(() => {
  if (!parsedSelected.value) return "";
  const time = split.value.time || "00:00";
  const d = parsedSelected.value;
  return `${d.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  })} ${time}`;
});

function toggle() {
  if (props.disabled) return;
  open.value = !open.value;
}

function clearValue(e: MouseEvent) {
  e.stopPropagation();
  emit("update:modelValue", "");
}

function onDateSelect(v: string) {
  emit("update:modelValue", formatIsoDatetime(v, selectedTime.value));
}

function onTimeUpdate(v: string) {
  const date = selectedDate.value || isoDate(new Date());
  emit("update:modelValue", formatIsoDatetime(date, v));
}

function setNow() {
  const d = new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  emit(
    "update:modelValue",
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`,
  );
}

function onTriggerKeydown(ev: KeyboardEvent) {
  if (ev.key === "Escape" && open.value) {
    ev.stopPropagation();
    open.value = false;
  }
}

watch(open, (is) => {
  if (is) {
    void nextTick(() => {
      rootRef.value?.querySelector<HTMLElement>("[data-date]")?.focus();
    });
  }
});
</script>

<template>
  <div ref="rootRef" class="relative w-full">
    <button
      :id="id"
      type="button"
      :disabled="disabled"
      :aria-expanded="open"
      :aria-invalid="error ? 'true' : undefined"
      aria-haspopup="dialog"
      class="field flex w-full cursor-pointer items-center justify-between gap-2 text-left"
      :class="[{ 'border-[var(--border-error)]': error, 'opacity-50': disabled }]"
      @click="toggle"
      @keydown="onTriggerKeydown"
    >
      <span class="flex min-w-0 items-center gap-2">
        <Calendar class="h-4 w-4 shrink-0 text-[var(--text-muted)]" aria-hidden="true" />
        <span v-if="displayLabel" class="truncate font-mono text-[var(--text-input)]">
          {{ displayLabel }}
        </span>
        <span v-else class="text-[var(--text-muted)]">{{ placeholder }}</span>
      </span>
      <span class="flex shrink-0 items-center gap-1">
        <button
          v-if="modelValue"
          type="button"
          class="cursor-pointer rounded p-0.5 text-[var(--text-muted)] transition-colors hover:text-[var(--text-primary)]"
          aria-label="Clear"
          tabindex="-1"
          @click="clearValue"
        >
          <X class="h-3.5 w-3.5" />
        </button>
        <ChevronDown
          class="h-4 w-4 text-[var(--text-muted)] transition-transform duration-200"
          :class="open ? 'rotate-180' : ''"
        />
      </span>
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
        aria-label="Select date and time"
        class="absolute left-0 z-50 mt-1 flex w-[20rem] flex-col gap-3 overflow-hidden rounded-lg border border-[var(--border-primary)] bg-[var(--bg-card)] p-3 shadow-xl sm:flex-row"
        @click.stop
      >
        <div class="flex-1">
          <CalendarPanel
            :model-value="selectedDate"
            :min-year="minYear"
            :max-year="maxYear"
            @update:model-value="onDateSelect"
          />
        </div>
        <div
          class="flex shrink-0 flex-col gap-1 border-t border-[var(--border-primary)] pt-3 sm:w-28 sm:border-l sm:border-t-0 sm:pl-3 sm:pt-0"
        >
          <span
            class="text-[0.65rem] font-semibold uppercase tracking-wider text-[var(--text-muted)]"
            >Time</span
          >
          <TimeSpinner
            :model-value="selectedTime"
            :disabled="disabled"
            @update:model-value="onTimeUpdate"
          />
          <button
            type="button"
            class="mt-auto inline-flex h-7 cursor-pointer items-center justify-center rounded text-[0.65rem] font-semibold uppercase tracking-wider text-[var(--accent-primary)] transition-colors hover:bg-[var(--bg-secondary)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
            @click="setNow"
          >
            Use now
          </button>
        </div>
      </div>
    </Transition>
  </div>
</template>
