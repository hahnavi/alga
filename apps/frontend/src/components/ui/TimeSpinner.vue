<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ChevronUp, ChevronDown } from "@lucide/vue";
import { formatIsoTime, parseIsoTime } from "@/lib/calendar";

const props = withDefaults(
  defineProps<{
    modelValue: string;
    disabled?: boolean;
  }>(),
  {},
);

const emit = defineEmits<{
  "update:modelValue": [value: string];
}>();

const parsed = computed(() => parseIsoTime(props.modelValue));
const hour = ref(parsed.value?.h ?? 9);
const minute = ref(parsed.value?.m ?? 0);

watch(
  () => props.modelValue,
  (v) => {
    const p = parseIsoTime(v);
    if (p) {
      hour.value = p.h;
      minute.value = p.m;
    }
  },
);

function clamp(n: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, n));
}

function commit() {
  emit("update:modelValue", formatIsoTime(hour.value, minute.value));
}

function stepHour(delta: number) {
  hour.value = (hour.value + delta + 24) % 24;
  commit();
}

function stepMinute(delta: number) {
  const next = clamp(minute.value + delta, 0, 59);
  minute.value = next;
  commit();
}

function onHourInput(e: Event) {
  const v = (e.target as HTMLInputElement).value.replace(/\D/g, "").slice(0, 2);
  (e.target as HTMLInputElement).value = v;
  if (v === "") return;
  const n = clamp(Number(v), 0, 23);
  hour.value = n;
  commit();
}

function onMinuteInput(e: Event) {
  const v = (e.target as HTMLInputElement).value.replace(/\D/g, "").slice(0, 2);
  (e.target as HTMLInputElement).value = v;
  if (v === "") return;
  const n = clamp(Number(v), 0, 59);
  minute.value = n;
  commit();
}

function setNow() {
  const d = new Date();
  hour.value = d.getHours();
  minute.value = d.getMinutes();
  commit();
}

function onKeydown(e: KeyboardEvent, kind: "hour" | "minute") {
  if (e.key === "ArrowUp") {
    e.preventDefault();
    if (kind === "hour") stepHour(1);
    else stepMinute(1);
  } else if (e.key === "ArrowDown") {
    e.preventDefault();
    if (kind === "hour") stepHour(-1);
    else stepMinute(-1);
  } else if (e.key === "PageUp") {
    e.preventDefault();
    if (kind === "hour") stepHour(6);
    else stepMinute(15);
  } else if (e.key === "PageDown") {
    e.preventDefault();
    if (kind === "hour") stepHour(-6);
    else stepMinute(-15);
  }
}
</script>

<template>
  <div class="flex flex-col gap-2">
    <div class="flex items-center gap-1">
      <div class="flex flex-col items-center">
        <button
          type="button"
          :disabled="disabled"
          class="inline-flex h-6 w-12 cursor-pointer items-center justify-center rounded text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)] disabled:cursor-not-allowed disabled:opacity-40"
          aria-label="Increment hour"
          @click="stepHour(1)"
        >
          <ChevronUp class="h-3.5 w-3.5" />
        </button>
        <input
          :value="String(hour).padStart(2, '0')"
          type="text"
          inputmode="numeric"
          :disabled="disabled"
          aria-label="Hour"
          class="field w-12 px-1 py-1 text-center text-base font-semibold tabular-nums"
          maxlength="2"
          @input="onHourInput"
          @keydown="onKeydown($event, 'hour')"
        />
        <button
          type="button"
          :disabled="disabled"
          class="inline-flex h-6 w-12 cursor-pointer items-center justify-center rounded text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)] disabled:cursor-not-allowed disabled:opacity-40"
          aria-label="Decrement hour"
          @click="stepHour(-1)"
        >
          <ChevronDown class="h-3.5 w-3.5" />
        </button>
      </div>
      <span class="pb-6 text-lg font-semibold text-[var(--text-muted)]">:</span>
      <div class="flex flex-col items-center">
        <button
          type="button"
          :disabled="disabled"
          class="inline-flex h-6 w-12 cursor-pointer items-center justify-center rounded text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)] disabled:cursor-not-allowed disabled:opacity-40"
          aria-label="Increment minute"
          @click="stepMinute(1)"
        >
          <ChevronUp class="h-3.5 w-3.5" />
        </button>
        <input
          :value="String(minute).padStart(2, '0')"
          type="text"
          inputmode="numeric"
          :disabled="disabled"
          aria-label="Minute"
          class="field w-12 px-1 py-1 text-center text-base font-semibold tabular-nums"
          maxlength="2"
          @input="onMinuteInput"
          @keydown="onKeydown($event, 'minute')"
        />
        <button
          type="button"
          :disabled="disabled"
          class="inline-flex h-6 w-12 cursor-pointer items-center justify-center rounded text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)] disabled:cursor-not-allowed disabled:opacity-40"
          aria-label="Decrement minute"
          @click="stepMinute(-1)"
        >
          <ChevronDown class="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
    <div class="flex items-center justify-center border-t border-[var(--border-primary)] pt-1.5">
      <button
        type="button"
        :disabled="disabled"
        class="inline-flex h-6 cursor-pointer items-center rounded px-2 text-[0.65rem] font-semibold uppercase tracking-wider text-[var(--text-muted)] transition-colors hover:bg-[var(--bg-secondary)] hover:text-[var(--text-primary)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)] disabled:cursor-not-allowed disabled:opacity-40"
        @click="setNow"
      >
        Now
      </button>
    </div>
  </div>
</template>
