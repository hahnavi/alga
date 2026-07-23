<script setup lang="ts">
import { ref } from "vue";
import type { Component } from "vue";
import { MoreVertical } from "@lucide/vue";
import { useDropdownLifecycle } from "@/composables/useDropdownLifecycle";
import {
  HEADER_ICON_BTN_CLASS,
  POPOVER_MENU_DESTRUCTIVE_CLASS,
  POPOVER_MENU_ITEM_CLASS,
  POPOVER_MENU_ITEM_ICON_CLASS,
  POPOVER_MENU_PANEL_CLASS,
} from "@/lib/uiClasses";

type HeaderActionItem = {
  label: string;
  icon?: Component;
  onSelect: () => void;
  destructive?: boolean;
  disabled?: boolean;
  /** When set, render as `<a target="_blank">` instead of a `<button>`. */
  href?: string;
  target?: "_blank";
};

withDefaults(
  defineProps<{
    items: HeaderActionItem[];
    label?: string;
  }>(),
  {
    label: "Page actions",
  },
);

const open = ref(false);
const rootRef = ref<HTMLElement | null>(null);

function toggle() {
  open.value = !open.value;
}

function select(item: HeaderActionItem) {
  if (item.disabled) return;
  open.value = false;
  item.onSelect();
}

useDropdownLifecycle(open, rootRef);
</script>

<template>
  <div ref="rootRef" class="relative shrink-0" @click.stop>
    <button
      type="button"
      :class="HEADER_ICON_BTN_CLASS"
      :aria-label="label"
      :title="label"
      :aria-expanded="open"
      aria-haspopup="menu"
      @click="toggle"
    >
      <MoreVertical class="h-4 w-4" aria-hidden="true" />
    </button>
    <div v-if="open" :class="POPOVER_MENU_PANEL_CLASS" role="menu" :aria-label="label">
      <a
        v-for="(item, i) in items.filter((x) => x.href)"
        :key="`a-${i}`"
        role="menuitem"
        :href="item.href"
        :target="item.target"
        rel="noopener noreferrer"
        :class="item.destructive ? POPOVER_MENU_DESTRUCTIVE_CLASS : POPOVER_MENU_ITEM_CLASS"
        @click="select(item)"
      >
        <component
          v-if="item.icon"
          :is="item.icon"
          :class="POPOVER_MENU_ITEM_ICON_CLASS"
          aria-hidden="true"
        />
        {{ item.label }}
      </a>
      <button
        v-for="(item, i) in items.filter((x) => !x.href)"
        :key="`b-${i}`"
        type="button"
        role="menuitem"
        :class="item.destructive ? POPOVER_MENU_DESTRUCTIVE_CLASS : POPOVER_MENU_ITEM_CLASS"
        :disabled="item.disabled"
        @click="select(item)"
      >
        <component
          v-if="item.icon"
          :is="item.icon"
          :class="POPOVER_MENU_ITEM_ICON_CLASS"
          aria-hidden="true"
        />
        {{ item.label }}
      </button>
    </div>
  </div>
</template>
