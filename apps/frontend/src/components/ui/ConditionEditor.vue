<script setup lang="ts">
import type { RouteCondition } from "@/lib/api";
import Input from "@/components/ui/Input.vue";
import Select from "@/components/ui/Select.vue";
import Button from "@/components/ui/Button.vue";
import { Plus, X } from "@lucide/vue";

const props = withDefaults(
  defineProps<{
    modelValue: RouteCondition[];
    source?: "labels" | "annotations" | "alert";
    disabled?: boolean;
  }>(),
  {
    source: "labels",
    disabled: false,
  },
);

const emit = defineEmits<{
  "update:modelValue": [value: RouteCondition[]];
}>();

function updateCondition(index: number, patch: Partial<RouteCondition>) {
  const next = [...props.modelValue];
  next[index] = { ...next[index], ...patch };
  emit("update:modelValue", next);
}

function removeCondition(index: number) {
  const next = [...props.modelValue];
  next.splice(index, 1);
  emit("update:modelValue", next);
}

function addCondition() {
  emit("update:modelValue", [
    ...props.modelValue,
    { source: props.source, field: "", operator: "exact" as const, value: "" },
  ]);
}
</script>

<template>
  <div class="space-y-2">
    <div v-for="(cond, ci) in modelValue" :key="ci" class="flex items-start gap-2">
      <Input
        :model-value="cond.field"
        placeholder="Label key"
        class="w-1/3"
        :disabled="disabled"
        @update:model-value="(v: string) => updateCondition(ci, { field: v })"
      />
      <Select
        :model-value="cond.operator"
        class="w-1/4"
        :disabled="disabled"
        @update:model-value="
          (v: string) => updateCondition(ci, { operator: v as RouteCondition['operator'] })
        "
      >
        <option value="exact">equals</option>
        <option value="contains">contains</option>
        <option value="prefix">starts with</option>
        <option value="suffix">ends with</option>
        <option value="regex">regex</option>
        <option value="exists">exists</option>
        <option value="not_exists">not exists</option>
      </Select>
      <Input
        v-if="cond.operator !== 'exists' && cond.operator !== 'not_exists'"
        :model-value="cond.value"
        placeholder="Value"
        class="flex-1"
        :disabled="disabled"
        @update:model-value="(v: string) => updateCondition(ci, { value: v })"
      />
      <Button
        size="sm"
        variant="outline"
        class="mt-0.5 shrink-0"
        :disabled="disabled"
        @click="removeCondition(ci)"
      >
        <X class="h-3.5 w-3.5" />
      </Button>
    </div>
    <Button size="sm" variant="outline" :disabled="disabled" @click="addCondition">
      <Plus class="h-3.5 w-3.5" />Add condition
    </Button>
  </div>
</template>
