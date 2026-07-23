<script setup lang="ts">
import { computed, nextTick, ref, watch, type Component } from "vue";
import { useDropdownLifecycle } from "@/composables/useDropdownLifecycle";
import {
  POPOVER_MENU_DESTRUCTIVE_CLASS,
  POPOVER_MENU_ITEM_CLASS,
  POPOVER_MENU_ITEM_ICON_CLASS,
  POPOVER_MENU_PANEL_CLASS,
  POPOVER_MENU_SEPARATOR_CLASS,
} from "@/lib/uiClasses";

export type MessageAction = {
  key: string;
  label: string;
  icon?: Component;
  destructive?: boolean;
  disabled?: boolean;
  onSelect: () => void;
};

const props = defineProps<{
  open: boolean;
  position: { clientX: number; clientY: number } | null;
  actions: MessageAction[];
  ariaLabel?: string;
}>();

const emit = defineEmits<{ close: [] }>();

const rootRef = ref<HTMLElement | null>(null);
const localOpen = ref(props.open);
const menuSize = ref({ width: 0, height: 0 });
const MENU_GAP = 8;
const VIEWPORT_PADDING = 8;

watch(
  () => props.open,
  (v) => {
    localOpen.value = v;
  },
);

watch(localOpen, (v) => {
  if (!v) emit("close");
});

useDropdownLifecycle(localOpen, rootRef);

watch(localOpen, async (open) => {
  if (open) {
    await nextTick();
    measureMenu();
  }
});

function measureMenu() {
  const el = rootRef.value;
  if (!el) return;
  const rect = el.getBoundingClientRect();
  menuSize.value = { width: rect.width, height: rect.height };
}

function onSelect(action: MessageAction) {
  if (action.disabled) return;
  localOpen.value = false;
  action.onSelect();
}

const placement = computed(() => {
  if (!props.position) return { top: 0, left: 0 };
  const vw = window.innerWidth;
  const vh = window.innerHeight;
  const { width, height } = menuSize.value;
  const estimatedWidth = width || 200;
  const estimatedHeight = height || Math.max(props.actions.length * 36 + 16, 60);
  let left = props.position.clientX;
  let top = props.position.clientY;
  if (left + estimatedWidth + VIEWPORT_PADDING > vw) {
    left = vw - estimatedWidth - VIEWPORT_PADDING;
  }
  if (top + estimatedHeight + VIEWPORT_PADDING > vh) {
    top = vh - estimatedHeight - VIEWPORT_PADDING;
  }
  left = Math.max(VIEWPORT_PADDING, left);
  top = Math.max(VIEWPORT_PADDING, top);
  return { top: top + MENU_GAP, left };
});
</script>

<template>
  <Teleport to="body">
    <div
      v-if="localOpen && position"
      ref="rootRef"
      :class="POPOVER_MENU_PANEL_CLASS"
      :style="{
        position: 'fixed',
        top: `${placement.top}px`,
        left: `${placement.left}px`,
        zIndex: 60,
      }"
      role="menu"
      :aria-label="ariaLabel ?? 'Message actions'"
      @contextmenu.prevent
      @pointerdown.stop
    >
      <template v-for="action in actions" :key="action.key">
        <div
          v-if="action.key === 'separator'"
          :class="POPOVER_MENU_SEPARATOR_CLASS"
          role="separator"
          aria-hidden="true"
        />
        <button
          v-else
          type="button"
          role="menuitem"
          :class="action.destructive ? POPOVER_MENU_DESTRUCTIVE_CLASS : POPOVER_MENU_ITEM_CLASS"
          :disabled="action.disabled"
          @click="onSelect(action)"
        >
          <component
            v-if="action.icon"
            :is="action.icon"
            :class="POPOVER_MENU_ITEM_ICON_CLASS"
            aria-hidden="true"
          />
          <span>{{ action.label }}</span>
        </button>
      </template>
    </div>
  </Teleport>
</template>
