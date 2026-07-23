<script setup lang="ts">
import { ref } from "vue";
import { ChevronDown, Lock, Send } from "@lucide/vue";
import { useDropdownLifecycle } from "@/composables/useDropdownLifecycle";

defineProps<{
  disabled: boolean;
  enableInternalNote: boolean;
  isEditorEmpty: boolean;
}>();

const emit = defineEmits<{
  send: [];
  sendInternal: [];
}>();

const showSendMenu = ref(false);
const sendWrapperRef = ref<HTMLElement | null>(null);

useDropdownLifecycle(showSendMenu, sendWrapperRef);

function handleSend() {
  showSendMenu.value = false;
  emit("send");
}

function handleSendInternal() {
  showSendMenu.value = false;
  emit("sendInternal");
}

function toggleSendMenu() {
  showSendMenu.value = !showSendMenu.value;
}
</script>

<template>
  <div ref="sendWrapperRef" class="markdown-editor__send-wrapper">
    <button
      type="button"
      class="markdown-editor__send-btn"
      :disabled="isEditorEmpty || disabled"
      title="Send (Ctrl+Enter)"
      @click="emit('send')"
    >
      <Send aria-hidden="true" />
    </button>
    <button
      v-if="enableInternalNote"
      type="button"
      class="markdown-editor__send-dropdown-toggle"
      :disabled="isEditorEmpty || disabled"
      title="Send options"
      @click="toggleSendMenu"
    >
      <ChevronDown class="h-2.5 w-2.5" aria-hidden="true" />
    </button>
    <div v-if="showSendMenu && enableInternalNote" class="markdown-editor__send-menu">
      <button type="button" class="markdown-editor__send-menu-option" @click="handleSend">
        <Send class="h-3.5 w-3.5" aria-hidden="true" />
        <span>Send</span>
      </button>
      <button type="button" class="markdown-editor__send-menu-option" @click="handleSendInternal">
        <Lock class="h-3.5 w-3.5" aria-hidden="true" />
        <span>Send as internal note</span>
      </button>
    </div>
  </div>
</template>
