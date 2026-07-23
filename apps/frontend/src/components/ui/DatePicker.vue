<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { Calendar, ChevronDown, X } from "@lucide/vue";
import { useDropdownLifecycle } from "@/composables/useDropdownLifecycle";
import { isoDate, parseIsoDate } from "@/lib/calendar";
import CalendarPanel from "@/components/ui/CalendarPanel.vue";

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
    placeholder: "Select date",
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

const textInput = ref("");

const parsedSelected = computed(() => parseIsoDate(props.modelValue));

const displayLabel = computed(() => {
  if (!parsedSelected.value) return "";
  return parsedSelected.value.toLocaleDateString(undefined, {
    weekday: "short",
    year: "numeric",
    month: "short",
    day: "numeric",
  });
});

function todayIso(): string {
  return isoDate(new Date());
}

function toggle() {
  if (props.disabled) return;
  open.value = !open.value;
  if (open.value) {
    textInput.value = "";
  }
}

function clearValue(e: MouseEvent | KeyboardEvent) {
  e.stopPropagation();
  emit("update:modelValue", "");
  textInput.value = "";
}

function onSelect(v: string) {
  emit("update:modelValue", v);
  open.value = false;
}

function commitText() {
  const parsed = parseIsoDate(textInput.value.trim());
  if (parsed) {
    emit("update:modelValue", isoDate(parsed));
  }
  textInput.value = "";
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
        <span v-if="displayLabel" class="truncate text-[var(--text-input)]">
          {{ displayLabel }}
        </span>
        <span v-else class="text-[var(--text-muted)]">{{ placeholder }}</span>
      </span>
      <span class="flex shrink-0 items-center gap-1">
        <button
          v-if="modelValue"
          type="button"
          class="cursor-pointer rounded p-0.5 text-[var(--text-muted)] transition-colors hover:text-[var(--text-primary)]"
          aria-label="Clear date"
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
        aria-label="Select date"
        class="absolute left-0 z-50 mt-1 w-72 overflow-hidden rounded-lg border border-[var(--border-primary)] bg-[var(--bg-card)] p-3 shadow-xl"
        @click.stop
      >
        <CalendarPanel
          :model-value="modelValue"
          :min-year="minYear"
          :max-year="maxYear"
          @update:model-value="onSelect"
          @close="open = false"
        />
        <div
          v-if="!modelValue"
          class="mt-2 flex items-center justify-center border-t border-[var(--border-primary)] pt-2"
        >
          <button
            type="button"
            class="inline-flex h-7 cursor-pointer items-center rounded px-2 text-xs font-medium text-[var(--text-secondary)] transition-colors hover:bg-[var(--bg-secondary)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[var(--focus-ring)]"
            @click="onSelect(todayIso())"
          >
            Use today
          </button>
        </div>
      </div>
    </Transition>

    <input
      v-model="textInput"
      type="text"
      class="sr-only"
      tabindex="-1"
      aria-hidden="true"
      :placeholder="placeholder"
      @blur="commitText"
      @keydown.enter.prevent="commitText"
    />
  </div>
</template>
