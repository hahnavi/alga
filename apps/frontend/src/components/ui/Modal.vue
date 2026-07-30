<script setup lang="ts">
import { computed, ref } from "vue";
import Button from "./Button.vue";
import DialogCloseButton from "./DialogCloseButton.vue";
import { useModalFocusTrap } from "@/lib/dialogFocus";
import { useEscapeKey } from "@/composables/useEscapeKey";

const props = withDefaults(
  defineProps<{
    open: boolean;
    title?: string;
    maxWidth?: string;
    loading?: boolean;
    confirmLabel?: string;
    cancelLabel?: string;
    destructive?: boolean;
    showFooter?: boolean;
    showHeader?: boolean;
    preventClose?: boolean;
    /**
     * "default" → standard modal with body + footer slots.
     * "confirm" → tight layout, max-w-md, optional `message` prop renders
     * a `<p>` body, and cancel/confirm buttons auto-close the modal on
     * click. This is the layout the legacy ConfirmDialog used; callers
     * can use `variant="confirm"` instead of importing ConfirmDialog.
     */
    variant?: "default" | "confirm";
    /** Confirm-only: simple message body. Has no effect on `default`. */
    message?: string;
  }>(),
  {
    title: "",
    maxWidth: "lg",
    loading: false,
    confirmLabel: "Save",
    cancelLabel: "Cancel",
    destructive: false,
    showFooter: true,
    showHeader: true,
    preventClose: false,
    variant: "default",
    message: "",
  },
);

const emit = defineEmits<{
  "update:open": [value: boolean];
  confirm: [];
  cancel: [];
  close: [];
}>();

const panelRef = ref<HTMLElement | null>(null);
const cancelButtonRef = ref<InstanceType<typeof Button> | null>(null);

const isOpen = computed(() => props.open);
const isConfirm = computed(() => props.variant === "confirm");

useModalFocusTrap(
  isOpen,
  () => panelRef.value,
  isConfirm.value
    ? {
        getInitialFocus: () => {
          const el = cancelButtonRef.value?.$el;
          return el instanceof HTMLElement ? el : null;
        },
      }
    : undefined,
);

const maxWidthClass = computed(() => {
  if (isConfirm.value) return "max-w-md";
  switch (props.maxWidth) {
    case "sm":
      return "max-w-sm";
    case "xl":
      return "max-w-xl";
    case "2xl":
      return "max-w-2xl";
    case "3xl":
      return "max-w-3xl";
    case "5xl":
      return "max-w-5xl";
    default:
      return "max-w-lg";
  }
});

function onBackdropClick(e: MouseEvent) {
  if (props.preventClose) return;
  if (e.target === e.currentTarget) close();
}

function close() {
  emit("update:open", false);
  emit("close");
}

function onConfirm() {
  emit("confirm");
  if (isConfirm.value) close();
}

function onCancel() {
  emit("cancel");
  if (isConfirm.value) close();
  else close();
}

function handleEscape() {
  if (props.preventClose) return;
  close();
}

useEscapeKey(handleEscape, () => props.open);
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="fixed inset-0 z-50 flex items-start justify-center bg-black/50 p-4 sm:items-center"
      role="presentation"
      @mousedown="onBackdropClick"
    >
      <div
        ref="panelRef"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="title ? 'modal-title' : undefined"
        :aria-describedby="isConfirm && message ? 'modal-message' : undefined"
        class="relative flex max-h-[calc(100vh-2rem)] w-full flex-col rounded-lg border border-[var(--border-primary)] bg-[var(--bg-card)] text-[var(--text-primary)] shadow-xl"
        :class="maxWidthClass"
        @mousedown.stop
      >
        <div
          v-if="showHeader || $slots.header"
          class="flex shrink-0 items-center justify-between border-b border-[var(--border-primary)] px-4 py-3"
        >
          <slot name="header">
            <h3
              v-if="title"
              id="modal-title"
              class="text-base font-semibold text-[var(--text-primary)]"
            >
              {{ title }}
            </h3>
            <DialogCloseButton :on-click="close" :disabled="preventClose" />
          </slot>
        </div>

        <div
          class="flex-1 overflow-y-auto overscroll-contain"
          :class="showHeader ? 'px-4 py-4' : 'p-4'"
        >
          <p
            v-if="isConfirm && message"
            id="modal-message"
            class="text-sm text-[var(--text-secondary)]"
          >
            {{ message }}
          </p>
          <slot v-else />
        </div>

        <div
          v-if="showFooter || $slots.footer"
          class="flex shrink-0 justify-end gap-2 border-t border-[var(--border-primary)] bg-[var(--bg-secondary)]/40 px-4 py-3"
        >
          <slot name="footer">
            <Button ref="cancelButtonRef" variant="outline" :disabled="loading" @click="onCancel">
              {{ cancelLabel }}
            </Button>
            <Button
              :variant="destructive ? 'destructive' : 'primary'"
              :loading="loading"
              @click="onConfirm"
            >
              {{ confirmLabel }}
            </Button>
          </slot>
        </div>
      </div>
    </div>
  </Teleport>
</template>
