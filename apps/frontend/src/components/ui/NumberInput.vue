<script setup lang="ts">
import { computed } from "vue";
import { ChevronUp, ChevronDown } from "@lucide/vue";

const props = withDefaults(
  defineProps<{
    modelValue?: number | string | null;
    min?: number | string;
    max?: number | string;
    step?: number | string;
    placeholder?: string;
    disabled?: boolean;
    error?: string;
    id?: string;
  }>(),
  {
    step: 1,
    disabled: false,
  },
);

const emit = defineEmits<{
  "update:modelValue": [value: number];
}>();

const numValue = computed(() => {
  const v = Number(props.modelValue);
  return Number.isFinite(v) ? v : NaN;
});

const numMin = computed(() => {
  const v = Number(props.min);
  return Number.isFinite(v) ? v : NaN;
});

const numMax = computed(() => {
  const v = Number(props.max);
  return Number.isFinite(v) ? v : NaN;
});

const numStep = computed(() => {
  const v = Number(props.step);
  return Number.isFinite(v) && v > 0 ? v : 1;
});

const stepScale = computed(() => {
  const s = String(props.step);
  const dot = s.indexOf(".");
  return dot === -1 ? 1 : Math.pow(10, s.length - dot - 1);
});

const atMin = computed(() => Number.isFinite(numMin.value) && numValue.value <= numMin.value);
const atMax = computed(() => Number.isFinite(numMax.value) && numValue.value >= numMax.value);

function stepAdd(base: number, direction: -1 | 1): number {
  const scale = stepScale.value;
  const scaledBase = Math.round(base * scale);
  const scaledStep = Math.round(numStep.value * scale);
  let result = (scaledBase + direction * scaledStep) / scale;
  if (Number.isFinite(numMin.value)) result = Math.max(result, numMin.value);
  if (Number.isFinite(numMax.value)) result = Math.min(result, numMax.value);
  return result;
}

function increment() {
  if (props.disabled) return;
  const base = Number.isFinite(numValue.value) ? numValue.value : 0;
  emit("update:modelValue", stepAdd(base, 1));
}

function decrement() {
  if (props.disabled) return;
  const base = Number.isFinite(numValue.value) ? numValue.value : 0;
  emit("update:modelValue", stepAdd(base, -1));
}

function onInput(e: Event) {
  const raw = (e.target as HTMLInputElement).value;
  if (raw === "" || raw === "-") {
    emit("update:modelValue", NaN);
    return;
  }
  const v = Number(raw);
  if (Number.isFinite(v)) {
    emit("update:modelValue", v);
  }
}
</script>

<template>
  <div class="number-input-wrapper">
    <input
      :id="id"
      type="number"
      :value="Number.isFinite(numValue) ? numValue : ''"
      :placeholder="placeholder"
      :disabled="disabled"
      :min="min"
      :max="max"
      :step="step"
      :aria-invalid="error ? 'true' : undefined"
      class="field number-input-field"
      :class="{ 'border-[var(--border-error)]': error }"
      @input="onInput"
    />
    <div class="number-input-spinners">
      <button
        type="button"
        class="number-input-btn"
        :disabled="disabled || atMax"
        @click="increment"
      >
        <ChevronUp class="h-3 w-3" />
      </button>
      <div class="number-input-divider" />
      <button
        type="button"
        class="number-input-btn"
        :disabled="disabled || atMin"
        @click="decrement"
      >
        <ChevronDown class="h-3 w-3" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.number-input-wrapper {
  position: relative;
  display: inline-flex;
  width: 100%;
}

.number-input-field {
  width: 100%;
  padding-right: 2rem;
  -moz-appearance: textfield;
}

.number-input-field::-webkit-inner-spin-button,
.number-input-field::-webkit-outer-spin-button {
  -webkit-appearance: none;
  margin: 0;
}

.number-input-field[type="number"]::-moz-inner-spin-button,
.number-input-field[type="number"]::-moz-outer-spin-button {
  -moz-appearance: none;
}

.number-input-spinners {
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  display: flex;
  flex-direction: column;
  width: 1.75rem;
  border-left: 1px solid var(--border-primary);
  border-radius: 0 0.25rem 0.25rem 0;
  overflow: hidden;
}

.number-input-divider {
  height: 1px;
  background: var(--border-primary);
}

.number-input-btn {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
  border: none;
  color: var(--text-muted);
  cursor: pointer;
  transition:
    background 0.15s,
    color 0.15s;
  padding: 0;
}

.number-input-btn:hover:not(:disabled) {
  background: var(--hover-neutral);
  color: var(--text-primary);
}

.number-input-btn:active:not(:disabled) {
  background: var(--border-primary);
}

.number-input-btn:disabled {
  cursor: not-allowed;
  opacity: 0.35;
}

.number-input-field:focus ~ .number-input-spinners {
  border-left-color: var(--focus-ring);
}
</style>
