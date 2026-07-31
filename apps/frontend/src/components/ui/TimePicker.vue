<script setup lang="ts">
import { computed, nextTick, ref, watch } from "vue";
import { Clock, ChevronDown, X } from "@lucide/vue";
import { useDropdownLifecycle } from "@/composables/useDropdownLifecycle";
import { parseIsoTime } from "@/lib/calendar";
import TimeSpinner from "@/components/ui/TimeSpinner.vue";

const props = withDefaults(
  defineProps<{
    id?: string;
    modelValue: string;
    placeholder?: string;
    disabled?: boolean;
    error?: string;
  }>(),
  {
    placeholder: "Select time",
  },
);

const emit = defineEmits<{
  "update:modelValue": [value: string];
}>();

const open = ref(false);
const rootRef = ref<HTMLElement | null>(null);

useDropdownLifecycle(open, rootRef);

const parsed = computed(() => parseIsoTime(props.modelValue));

const displayLabel = computed(() => {
  if (!parsed.value) return "";
  return `${String(parsed.value.h).padStart(2, "0")}:${String(parsed.value.m).padStart(2, "0")}`;
});

function toggle() {
  if (props.disabled) return;
  open.value = !open.value;
}

function clearValue(e: MouseEvent) {
  e.stopPropagation();
  emit("update:modelValue", "");
}

function onUpdate(v: string) {
  emit("update:modelValue", v);
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
      const input = rootRef.value?.querySelector<HTMLInputElement>('input[aria-label="Hour"]');
      input?.focus();
      input?.select();
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
        <Clock class="h-4 w-4 shrink-0 text-[var(--text-muted)]" aria-hidden="true" />
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
          aria-label="Clear time"
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
        aria-label="Select time"
        class="absolute left-0 z-50 mt-1 w-44 overflow-hidden rounded border border-[var(--border-primary)] bg-[var(--bg-dialog)] p-3 shadow-xl"
        @click.stop
      >
        <TimeSpinner
          :model-value="modelValue"
          :disabled="disabled"
          @update:model-value="onUpdate"
        />
      </div>
    </Transition>
  </div>
</template>
