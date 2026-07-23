<script setup lang="ts">
import Modal from "./Modal.vue";

defineOptions({ name: "ConfirmDialog" });

withDefaults(
  defineProps<{
    open: boolean;
    title?: string;
    message: string;
    confirmLabel?: string;
    cancelLabel?: string;
    destructive?: boolean;
    loading?: boolean;
  }>(),
  {
    title: "Confirm",
    confirmLabel: "Confirm",
    cancelLabel: "Cancel",
    destructive: false,
    loading: false,
  },
);

defineEmits<{
  confirm: [];
  cancel: [];
  "update:open": [value: boolean];
}>();
</script>

<template>
  <Modal
    :open="open"
    :title="title"
    :message="message"
    :confirm-label="confirmLabel"
    :cancel-label="cancelLabel"
    :destructive="destructive"
    :loading="loading"
    variant="confirm"
    @confirm="$emit('confirm')"
    @cancel="$emit('cancel')"
    @update:open="$emit('update:open', $event)"
  />
</template>
