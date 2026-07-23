<script setup lang="ts">
import { computed } from "vue";
import { ArrowUp, ArrowDown } from "@lucide/vue";
import Select from "./Select.vue";
import Button from "./Button.vue";

export type SortOption = {
  label: string;
  value: string;
};

const props = defineProps<{
  modelValue: string;
  options: SortOption[];
}>();

const emit = defineEmits<{
  "update:modelValue": [value: string];
}>();

function parseSort(raw: string) {
  if (raw.startsWith("-")) return { field: raw.slice(1), desc: true };
  if (raw.startsWith("+")) return { field: raw.slice(1), desc: false };
  return { field: raw, desc: false };
}

const currentField = computed(() => parseSort(props.modelValue).field);
const isDesc = computed(() => parseSort(props.modelValue).desc);

function onFieldChange(field: string) {
  emit("update:modelValue", isDesc.value ? `-${field}` : field);
}

function toggleDirection() {
  const { field, desc } = parseSort(props.modelValue);
  emit("update:modelValue", desc ? field : `-${field}`);
}
</script>

<template>
  <div class="flex items-end gap-1.5">
    <div class="min-w-0 flex-1">
      <Select
        :model-value="currentField"
        class="w-full sm:w-[min(100%,16rem)]"
        aria-label="Sort by"
        @update:model-value="onFieldChange"
      >
        <option v-for="opt in options" :key="opt.value" :value="opt.value">
          {{ opt.label }}
        </option>
      </Select>
    </div>
    <Button
      variant="outline"
      size="sm"
      type="button"
      class="mb-px h-[2.25rem] px-2"
      :aria-label="isDesc ? 'Sort ascending' : 'Sort descending'"
      :title="isDesc ? 'Ascending' : 'Descending'"
      @click="toggleDirection"
    >
      <ArrowDown v-if="isDesc" class="h-3.5 w-3.5" />
      <ArrowUp v-else class="h-3.5 w-3.5" />
    </Button>
  </div>
</template>
