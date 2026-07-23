<script setup lang="ts">
import { Save, Trash2 } from "@lucide/vue";
import HeaderActionsMenu from "./HeaderActionsMenu.vue";

const emit = defineEmits<{
  edit: [];
  delete: [];
}>();
</script>

<template>
  <HeaderActionsMenu
    :items="[
      { label: 'Edit', icon: Save, onSelect: () => emit('edit') },
      {
        label: 'Delete',
        icon: Trash2,
        onSelect: () => emit('delete'),
        destructive: true,
      },
    ]"
    label="Service actions"
  />
</template>
