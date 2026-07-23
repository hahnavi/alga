<script setup lang="ts">
import { ref, watch } from "vue";
import PhoneCountrySelect from "@/components/ui/PhoneCountrySelect.vue";
import { findCountryByCode, findCountryByDial } from "@/lib/phoneCountries";

const props = withDefaults(
  defineProps<{
    modelValue?: string;
    id?: string;
    disabled?: boolean;
    placeholder?: string;
    country?: string;
  }>(),
  {
    modelValue: "",
    id: undefined,
    disabled: false,
    placeholder: "415 555 1234",
    country: "",
  },
);

const emit = defineEmits<{
  "update:modelValue": [value: string];
  "update:country": [value: string];
}>();

const FALLBACK_CODE = "US";

function digitsOnly(s: string): string {
  return s.replace(/\D+/g, "");
}

const selectedCode = ref(FALLBACK_CODE);
const national = ref("");

function splitIncoming(value: string): { code: string; national: string } {
  const v = value.trim();
  if (!v.startsWith("+")) {
    return { code: FALLBACK_CODE, national: digitsOnly(v) };
  }
  const rest = digitsOnly(v.slice(1));
  if (!rest) {
    return { code: FALLBACK_CODE, national: "" };
  }
  let best = findCountryByDial(rest);
  if (!best) {
    return { code: FALLBACK_CODE, national: rest };
  }
  return { code: best.code, national: rest.slice(best.dial.length) };
}

function composeEmit(): string {
  const digits = digitsOnly(national.value);
  if (!digits) return "";
  const country = findCountryByCode(selectedCode.value) ?? findCountryByCode(FALLBACK_CODE);
  if (!country) return "";
  return "+" + country.dial + digits;
}

function emitValue() {
  emit("update:country", selectedCode.value);
  emit("update:modelValue", composeEmit());
}

watch(
  () => props.country,
  (val) => {
    if (!val) return;
    if (val === selectedCode.value) return;
    selectedCode.value = val;
  },
);

watch(
  () => props.modelValue,
  (val) => {
    const incoming = (val ?? "").trim();
    if (incoming === composeEmit()) return;
    if (!incoming) {
      national.value = "";
      if (props.country) selectedCode.value = props.country;
      return;
    }
    const { code, national: n } = splitIncoming(incoming);
    selectedCode.value = props.country || code;
    national.value = n;
  },
  { immediate: true },
);

function onCountryChange(code: string) {
  selectedCode.value = code;
  emitValue();
}

function onInput(e: Event) {
  national.value = (e.target as HTMLInputElement).value;
  emitValue();
}
</script>

<template>
  <div class="flex gap-2">
    <PhoneCountrySelect
      :model-value="selectedCode"
      :disabled="disabled"
      @update:model-value="onCountryChange"
    />
    <input
      :id="id"
      class="field min-w-0 flex-1"
      type="tel"
      inputmode="tel"
      :value="national"
      :disabled="disabled"
      :placeholder="placeholder"
      @input="onInput"
    />
  </div>
</template>
