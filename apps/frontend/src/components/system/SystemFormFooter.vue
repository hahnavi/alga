<script setup lang="ts">
import { RotateCcw, Save } from "@lucide/vue";
import Button from "@/components/ui/Button.vue";

defineProps<{
  dirty: boolean;
  saving: boolean;
}>();

const emit = defineEmits<{
  save: [];
  discard: [];
}>();
</script>

<template>
  <div class="flex items-center justify-end gap-2 pt-2">
    <Button v-if="dirty" variant="outline" :disabled="saving" @click="emit('discard')">
      <RotateCcw class="h-4 w-4" />
      Discard
    </Button>
    <Button :disabled="!dirty" :loading="saving" @click="emit('save')">
      <Save class="h-4 w-4" />
      Save
    </Button>
  </div>
</template>
